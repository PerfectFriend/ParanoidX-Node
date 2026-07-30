// Package status provides server status tracking
package status

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"ParanoidX/internal/dockerutil"
)


// Collect handles the Collect HTTP request.
func Collect(dataDir, vaultPath string, startTime time.Time) map[string]any {
	uptime := time.Now().Sub(startTime).Seconds()

	info := map[string]any{
		"status":         "running",
		"started_at":     startTime.Format(time.RFC3339),
		"uptime_seconds": int64(uptime),
	}

	smpStatus, xftpStatus := dockerutil.ServiceStatus()
	info["smp"] = map[string]any{
		"fingerprint": readTrim(filepath.Join("/home/tomas/ParanoidX/docker/smp_configs/fingerprint")),
		"status":      smpStatus,
	}
	info["xftp"] = map[string]any{
		"fingerprint": readTrim(filepath.Join("/home/tomas/ParanoidX/docker/xftp_configs/fingerprint")),
		"status":      xftpStatus,
	}

	info["storage"] = map[string]any{
		"smp_state":  dirSizeMBFast(filepath.Join("/home/tomas/ParanoidX/docker/smp_state")),
		"xftp_state": dirSizeMBFast(filepath.Join("/home/tomas/ParanoidX/docker/xftp_state")),
	}

	info["vault"] = map[string]any{
		"used_mb":    getVaultSizeMB(vaultPath),
		"quota_mb":   2048,
		"file_count": getVaultFileCount(vaultPath),
	}

	info["disk"] = getDiskMetrics()

	info["is_royal"] = isRoyalNode(dataDir)

	if b, err := os.ReadFile(filepath.Join(dataDir, "island_contact_link.txt")); err == nil {
		info["island_services_contact"] = strings.TrimSpace(string(b))
	}

	reputation := calculateReputationStub(dataDir)
	info["reputation"] = reputation
	info["reputation_tier"] = getTier(reputation)

	return info
}

func readTrim(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func dirSizeMBFast(path string) float64 {
	out, err := exec.Command("du", "-sb", path).Output()
	if err != nil {
		return 0
	}
	parts := strings.Fields(string(out))
	if len(parts) == 0 {
		return 0
	}
	bytes, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0
	}
	return bytes / (1024 * 1024)
}

func getVaultSizeMB(vaultPath string) float64 {
	var total int64
	filepath.Walk(vaultPath, func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return nil
		}
		total += fi.Size()
		return nil
	})
	return float64(total) / (1024 * 1024)
}

func getVaultFileCount(vaultPath string) int {
	count := 0
	ents, _ := os.ReadDir(vaultPath)
	for _, e := range ents {
		if !e.IsDir() {
			count++
		}
	}
	return count
}

func getDiskMetrics() map[string]any {
	res := map[string]any{}
	df := func(path, label string) {
		out, err := exec.Command("df", "-B1", path).Output()
		if err != nil {
			return
		}
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(lines) < 2 {
			return
		}
		fields := strings.Fields(lines[1])
		if len(fields) < 4 {
			return
		}
		total, _ := strconv.ParseInt(fields[1], 10, 64)
		used, _ := strconv.ParseInt(fields[2], 10, 64)
		avail, _ := strconv.ParseInt(fields[3], 10, 64)
		pct := 0.0
		if total > 0 {
			pct = float64(used) / float64(total) * 100.0
		}
		res[label] = map[string]any{
			"total_gb": float64(total) / (1024 * 1024 * 1024),
			"used_gb":  float64(used) / (1024 * 1024 * 1024),
			"avail_gb": float64(avail) / (1024 * 1024 * 1024),
			"used_pct": fmt.Sprintf("%.1f%%", pct),
		}
		if pct > 90 {
			res[label].(map[string]any)["alert"] = "CRITICAL: >90% used"
		} else if pct > 80 {
			res[label].(map[string]any)["warn"] = "WARNING: >80% used"
		}
	}
	df("/", "root")
	df("/home/tomas/.local/share/simplex-node", "data")
	df("/home/tomas/ParanoidX/docker/smp_state", "smp_state")
	df("/home/tomas/ParanoidX/docker/xftp_state", "xftp_state")

	if out, err := exec.Command("docker", "system", "df", "--format", "{{.Type}}\t{{.Size}}\t{{.Reclaimable}}").Output(); err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			parts := strings.Split(line, "\t")
			if len(parts) >= 2 {
				res["docker_"+strings.ToLower(parts[0])] = parts[1]
			}
		}
	}

	return res
}

func isRoyalNode(dataDir string) bool {
	p := filepath.Join(dataDir, "royal.enabled")
	if _, err := os.Stat(p); err != nil {
		return false
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return false
	}
	content := strings.TrimSpace(string(b))
	if content == "" || content == "0" {
		return false
	}
	return true
}

func calculateReputationStub(dataDir string) map[string]any {
	score := 0.0
	reasons := []string{}

	if isRoyalNode(dataDir) {
		score += 100
		reasons = append(reasons, "royal_node")
	}

	if b, err := os.ReadFile(filepath.Join(dataDir, "banknotes_registry.json")); err == nil {
		var holders []map[string]any
		if json.Unmarshal(b, &holders) == nil {
			score += float64(len(holders)) * 10
			if len(holders) > 0 {
				reasons = append(reasons, "banknote_holders")
			}
		}
	}

	auditors := 0
	if b, err := os.ReadFile(filepath.Join(dataDir, "auditor_tokens.json")); err == nil {
		var tokens []map[string]any
		if json.Unmarshal(b, &tokens) == nil {
			auditors = len(tokens)
			score += float64(auditors) * 5
			if auditors > 0 {
				reasons = append(reasons, "auditors_present")
			}
		}
	}

	return map[string]any{
		"score":   score,
		"reasons": reasons,
	}
}

func getTier(scoreIf any) string {
	score := 0.0
	switch v := scoreIf.(type) {
	case map[string]any:
		if s, ok := v["score"].(float64); ok {
			score = s
		}
	case float64:
		score = v
	}
	switch {
	case score >= 200:
		return "diamond"
	case score >= 100:
		return "gold"
	case score >= 50:
		return "silver"
	case score >= 10:
		return "bronze"
	default:
		return "basic"
	}
}


// CheckDiskAndAlert handles the CheckDiskAndAlert HTTP request.
func CheckDiskAndAlert() map[string]any {
	metrics := getDiskMetrics()
	var alerts []string
	for k, v := range metrics {
		if m, ok := v.(map[string]any); ok {
			if a, ok := m["alert"]; ok {
				alerts = append(alerts, fmt.Sprintf("%s: %s", k, a))
			}
			if w, ok := m["warn"]; ok {
				alerts = append(alerts, fmt.Sprintf("%s: %s", k, w))
			}
		}
	}
	metrics["alerts"] = alerts

	// USB detection
	if out, err := exec.Command("findmnt", "-n", "-o", "SOURCE,TARGET,SIZE,AVAIL", "--target", "/mnt/simplex-backup").Output(); err == nil {
		parts := strings.Fields(string(out))
		if len(parts) >= 4 {
			metrics["usb"] = map[string]any{
				"device":   parts[0],
				"mount":    parts[1],
				"size":     parts[2],
				"avail":    parts[3],
				"avail_gb": parseSizeToGB(parts[3]),
			}
		}
	}

	return metrics
}

func parseSizeToGB(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", ".")
	if strings.HasSuffix(s, "G") {
		return strings.TrimSuffix(s, "G")
	}
	if strings.HasSuffix(s, "T") {
		t, _ := strconv.ParseFloat(strings.TrimSuffix(s, "T"), 64)
		return fmt.Sprintf("%.1f", t*1024)
	}
	if strings.HasSuffix(s, "M") {
		m, _ := strconv.ParseFloat(strings.TrimSuffix(s, "M"), 64)
		return fmt.Sprintf("%.2f", m/1024)
	}
	return s
}
