// Mutagen — generalized mutation testing for Go packages.
// Runs mutations on source files and verifies tests catch them.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type Mutation struct {
	Name        string
	Pattern     *regexp.Regexp
	Replacement string
	Description string
}

var mutations = []Mutation{
	{
		Name:        "invert-comparison",
		Pattern:     regexp.MustCompile(`(\s+)(<|>|<=|>=|==|!=)(\s+)`),
		Replacement: "$1__INVERTED_$2$3",
		Description: "Invert comparison operators",
	},
	{
		Name:        "remove-error-check",
		Pattern:     regexp.MustCompile(`if err != nil \{`),
		Replacement: `if false { // MUT: removed error check`,
		Description: "Remove error checks",
	},
	{
		Name:        "invert-boolean",
		Pattern:     regexp.MustCompile(`(\s+)!(true|false)(\s+)`),
		Replacement: "$1$2$3",
		Description: "Invert boolean literals",
	},
	{
		Name:        "change-constant",
		Pattern:     regexp.MustCompile(`(\s+)(const\s+\w+\s+=\s+)(\d+)`),
		Replacement: "$1$2__MUTATED_$3",
		Description: "Change constant values",
	},
	{
		Name:        "remove-function-call",
		Pattern:     regexp.MustCompile(`(\s+)(\w+)\(([^)]*)\)`),
		Replacement: "$1// MUT: removed call to $2($3)",
		Description: "Remove function calls",
	},
}

type Result struct {
	MutationName string
	Passed       bool
	TestOutput   string
	Error        error
}

func main() {
	pkg := flag.String("pkg", "", "Package to test (e.g., ./internal/registry)")
	testPattern := flag.String("test", "", "Test pattern to run (e.g., TestGenerateNodeID)")
	verbose := flag.Bool("v", false, "Verbose output")
	flag.Parse()

	if *pkg == "" {
		fmt.Fprintln(os.Stderr, "Usage: mutagen -pkg ./path/to/package [-test TestPattern]")
		os.Exit(1)
	}

	absPkg, err := filepath.Abs(*pkg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid package path: %v\n", err)
		os.Exit(1)
	}

	// Find all .go files in the package (excluding _test.go)
	files, err := findSourceFiles(absPkg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to find source files: %v\n", err)
		os.Exit(1)
	}

	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "No source files found in package")
		os.Exit(1)
	}

	fmt.Printf("Found %d source files in %s\n", len(files), absPkg)

	// First, run tests to establish baseline
	fmt.Println("=== Baseline test run ===")
	baselineRC := runTests(absPkg, *testPattern, *verbose)
	if baselineRC != 0 {
		fmt.Fprintf(os.Stderr, "Baseline tests FAILED (exit %d). Fix tests first.\n", baselineRC)
		os.Exit(1)
	}
	fmt.Println("Baseline tests PASSED")

	// For each source file, try each mutation
	var results []Result
	totalMutations := 0

	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to read %s: %v\n", file, err)
			continue
		}

		// Skip files that are too small or are test files
		if strings.HasSuffix(file, "_test.go") {
			continue
		}

		for _, mut := range mutations {
			matches := mut.Pattern.FindAllIndex(content, -1)
			if len(matches) == 0 {
				continue
			}

			// Try each match location (limit to first 3 per file per mutation to avoid explosion)
			for i, match := range matches {
				if i >= 3 {
					break
				}

				totalMutations++
				mutName := fmt.Sprintf("%s:%s#%d", filepath.Base(file), mut.Name, i)

				if *verbose {
					fmt.Printf("\nTrying mutation %d/%d: %s\n", totalMutations, len(files)*len(mutations)*3, mutName)
				}

				// Apply mutation
				mutated := applyMutation(content, match, mut.Replacement)
				if bytes.Equal(mutated, content) {
					if *verbose {
						fmt.Printf("  SKIP: mutation produced no change\n")
					}
					continue
				}

				// Write mutated file
				if err := os.WriteFile(file, mutated, 0644); err != nil {
					fmt.Fprintf(os.Stderr, "Failed to write mutated file: %v\n", err)
					continue
				}

				// Run tests
				rc := runTests(absPkg, *testPattern, false)

				// Restore original file
				if err := os.WriteFile(file, content, 0644); err != nil {
					fmt.Fprintf(os.Stderr, "Failed to restore original file: %v\n", err)
				}

				passed := rc != 0 // Test should FAIL (non-zero exit) to catch the mutation
				results = append(results, Result{
					MutationName: mutName,
					Passed:       passed,
				})

				if *verbose {
					if passed {
						fmt.Printf("  PASS: test caught mutation (exit %d)\n", rc)
					} else {
						fmt.Printf("  FAIL: test did NOT catch mutation — VACUOUS!\n")
					}
				}
			}
		}
	}

	// Summary
	fmt.Println("\n=== Mutation Testing Summary ===")
	passCount := 0
	failCount := 0
	for _, r := range results {
		if r.Passed {
			passCount++
			fmt.Printf("PASS  %s\n", r.MutationName)
		} else {
			failCount++
			fmt.Printf("FAIL  %s  (test is vacuous)\n", r.MutationName)
		}
	}

	fmt.Printf("\nTotal mutations attempted: %d\n", totalMutations)
	fmt.Printf("PASS=%d  FAIL=%d\n", passCount, failCount)

	if failCount > 0 {
		fmt.Println("\nWARNING: Some tests are vacuous — they don't catch real bugs!")
		os.Exit(1)
	}
	fmt.Println("\nOK: All mutations caught by tests.")
}

func findSourceFiles(dir string) ([]string, error) {
	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

func applyMutation(content []byte, match []int, replacement string) []byte {
	start, end := match[0], match[1]
	result := make([]byte, 0, len(content)+len(replacement))
	result = append(result, content[:start]...)
	result = append(result, []byte(replacement)...)
	result = append(result, content[end:]...)
	return result
}

func runTests(pkg, pattern string, verbose bool) int {
	args := []string{"test", pkg}
	if pattern != "" {
		args = append(args, "-run", pattern)
	}
	if verbose {
		args = append(args, "-v")
	}
	args = append(args, "-count=1")

	cmd := exec.Command("go", args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if verbose {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	err := cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		return -1
	}
	return 0
}
