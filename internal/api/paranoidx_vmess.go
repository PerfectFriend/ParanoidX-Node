// Package api provides HTTP handlers and API endpoints for the simplex-node server
package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	vmessMu          sync.RWMutex
	vmessInitialized bool
	vmessDataDir     string
)

const (
	vmessPort    = 10812
	vmessDataSub = "vmess"
)

// VMessStatus holds the current VMess server state.
type VMessStatus struct {
	Running bool   `json:"running"`
	UUID    string `json:"uuid"`
	Port    int    `json:"port"`
	Since   string `json:"since,omitempty"`
}

func ensureVMessDir(baseDir string) string {
	dir := filepath.Join(baseDir, vmessDataSub)
	os.MkdirAll(dir, 0700)
	return dir
}

func readVMessMeta(baseDir string) (string, error) {
	metaPath := filepath.Join(baseDir, vmessDataSub, "meta.json")
	b, err := os.ReadFile(metaPath)
	if err != nil {
		return "", err
	}
	var meta struct {
		UUID    string `json:"uuid"`
		Port    int    `json:"port"`
		Created string `json:"created"`
	}
	if err := json.Unmarshal(b, &meta); err != nil {
		return "", err
	}
	return meta.UUID, nil
}

func isVMessRunning() bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", vmessPort), 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// VMessStatusHandler returns the current VMess server state.
func VMessStatusHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		running := isVMessRunning()
		uuid := ""
		if vmessDataDir != "" {
			if u, err := readVMessMeta(vmessDataDir); err == nil {
				uuid = u
			}
		}
		status := VMessStatus{
			Running: running,
			UUID:    uuid,
			Port:    vmessPort,
		}
		writeJSON(w, map[string]any{
			"ok":     true,
			"status": status,
		})
	}
}

// VMessInitHandler generates VMess config and starts the server.
// VMessInitHandler generates VMess config and starts the server via setup-vmess.sh.
func VMessInitHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		vmessMu.Lock()
		defer vmessMu.Unlock()

		if vmessInitialized {
			writeJSON(w, map[string]any{
				"ok":    false,
				"error": "VMess already initialized. Use /api/paranoidx/vmess/rotate to rotate UUID.",
			})
			return
		}

		scriptPath := filepath.Join(os.Getenv("HOME"), "ParanoidX", "scripts", "setup-vmess.sh")
		if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
			http.Error(w, "setup-vmess.sh not found", http.StatusInternalServerError)
			return
		}

		cmd := exec.Command("bash", scriptPath)
		output, err := cmd.CombinedOutput()
		if err != nil {
			writeJSON(w, map[string]any{
				"ok":     false,
				"error":  fmt.Sprintf("setup-vmess.sh failed: %v", err),
				"output": string(output),
			})
			return
		}

		vmessInitialized = true

		uuid := ""
		if vmessDataDir != "" {
			if u, erru := readVMessMeta(vmessDataDir); erru == nil {
				uuid = u
			}
		}

		writeJSON(w, map[string]any{
			"ok":      true,
			"uuid":    uuid,
			"port":    vmessPort,
			"output":  string(output),
			"message": "VMess server initialized and started. Run /api/paranoidx/vmess/rotate to change UUID.",
		})
	}
}

// VMessRotateHandler generates a new UUID and restarts the VMess server.
func VMessRotateHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}

		scriptPath := filepath.Join(os.Getenv("HOME"), "ParanoidX", "scripts", "setup-vmess.sh")
		newUUID := fmt.Sprintf("%s-%s-%s-%s-%s",
			randHex(8), randHex(4), randHex(4), randHex(4), randHex(12))

		cmd := exec.Command("bash", scriptPath, "--uuid", newUUID)
		output, err := cmd.CombinedOutput()
		if err != nil {
			writeJSON(w, map[string]any{
				"ok":     false,
				"error":  fmt.Sprintf("rotate failed: %v", err),
				"output": string(output),
			})
			return
		}

		// Restart VMess server to pick up new config
		_ = exec.Command("systemctl", "--user", "restart", "vmess-server.service").Run()

		// Also restart main xray to pick up updated client outbound
		pgrep := exec.Command("pgrep", "-x", "xray")
		if xrayPID, err := pgrep.Output(); err == nil {
			pid := string(xrayPID)
			_ = exec.Command("kill", "-HUP", "xray").Run()
			// Fallback: kill and restart via launch-node.sh
			if !isVMessRunning() {
				_ = exec.Command("kill", strings.TrimSpace(pid)).Run()
				time.Sleep(500 * time.Millisecond)
				_ = exec.Command("bash", "-c",
					"nohup /home/tomas/bin/v2ray/xray run -c /home/tomas/bin/v2ray/config.json &>/dev/null &").Run()
			}
		}

		writeJSON(w, map[string]any{
			"ok":      true,
			"uuid":    newUUID,
			"port":    vmessPort,
			"output":  string(output),
			"message": "UUID rotated, VMess server restarted with new UUID.",
		})
	}
}

// VMessConfigHandler returns the current VMess client config (for external clients).
func VMessConfigHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		running := isVMessRunning()
		uuid := ""
		if vmessDataDir != "" {
			if u, err := readVMessMeta(vmessDataDir); err == nil {
				uuid = u
			}
		}

		clientConfig := map[string]any{
			"protocol": "vmess",
			"settings": map[string]any{
				"vnext": []map[string]any{
					{
						"address": "127.0.0.1",
						"port":    vmessPort,
						"users": []map[string]any{
							{
								"id":       uuid,
								"alterId":  0,
								"security": "auto",
							},
						},
					},
				},
			},
			"streamSettings": map[string]any{
				"network":  "tcp",
				"security": "none",
			},
		}

		writeJSON(w, map[string]any{
			"ok":           true,
			"running":      running,
			"uuid":         uuid,
			"port":         vmessPort,
			"clientConfig": clientConfig,
			"serverConfig": map[string]any{
				"port": vmessPort,
				"protocol": "vmess",
			},
		})
	}
}

// InitVMess initialises the VMess data directory and runs auto-init if not yet set up.
func InitVMess(dataDir string) {
	vmessDataDir = dataDir
	_ = ensureVMessDir(dataDir)

	// Check if already initialized
	if _, err := readVMessMeta(dataDir); err == nil {
		vmessInitialized = true
	}

	// Auto-init if not yet initialized
	if !vmessInitialized {
		slog.Info("vmess: not initialized, running setup-vmess.sh")
		scriptPath := filepath.Join(os.Getenv("HOME"), "ParanoidX", "scripts", "setup-vmess.sh")
		if _, err := os.Stat(scriptPath); err == nil {
			cmd := exec.Command("bash", scriptPath)
			if out, err := cmd.CombinedOutput(); err != nil {
				slog.Warn("vmess: auto-init failed", "error", err, "output", string(out))
			} else {
				vmessInitialized = true
				slog.Info("vmess: auto-init complete", "output", string(out))
			}
		} else {
			slog.Warn("vmess: setup-vmess.sh not found, skipping auto-init", "path", scriptPath)
		}
	}
}

func randHex(n int) string {
	const hex = "0123456789abcdef"
	b := make([]byte, n)
	f, _ := os.Open("/dev/urandom")
	if f != nil {
		f.Read(b)
		f.Close()
	}
	for i := range b {
		b[i] = hex[int(b[i])%16]
	}
	return string(b)
}
