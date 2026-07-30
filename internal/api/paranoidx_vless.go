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
	vlessMu          sync.RWMutex
	vlessInitialized bool
	vlessDataDir     string
)

const (
	vlessPort    = 10813
	vlessDataSub = "vless"
)

// VLESSStatus holds the current VLESS server state.
type VLESSStatus struct {
	Running bool   `json:"running"`
	UUID    string `json:"uuid"`
	Port    int    `json:"port"`
	Since   string `json:"since,omitempty"`
}

func ensureVLESSDir(baseDir string) string {
	dir := filepath.Join(baseDir, vlessDataSub)
	os.MkdirAll(dir, 0700)
	return dir
}

func readVLESSMeta(baseDir string) (string, error) {
	metaPath := filepath.Join(baseDir, vlessDataSub, "meta.json")
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

func isVLESSRunning() bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", vlessPort), 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// VLESSStatusHandler returns the current VLESS server state.
func VLESSStatusHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		running := isVLESSRunning()
		uuid := ""
		if vlessDataDir != "" {
			if u, err := readVLESSMeta(vlessDataDir); err == nil {
				uuid = u
			}
		}
		status := VLESSStatus{
			Running: running,
			UUID:    uuid,
			Port:    vlessPort,
		}
		writeJSON(w, map[string]any{
			"ok":     true,
			"status": status,
		})
	}
}

// VLESSInitHandler generates VLESS config and starts the server via setup-vless.sh.
func VLESSInitHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		vlessMu.Lock()
		defer vlessMu.Unlock()

		if vlessInitialized {
			writeJSON(w, map[string]any{
				"ok":    false,
				"error": "VLESS already initialized. Use /api/paranoidx/vless/rotate to rotate UUID.",
			})
			return
		}

		scriptPath := filepath.Join(os.Getenv("HOME"), "simplex-node", "scripts", "setup-vless.sh")
		if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
			http.Error(w, "setup-vless.sh not found", http.StatusInternalServerError)
			return
		}

		cmd := exec.Command("bash", scriptPath)
		output, err := cmd.CombinedOutput()
		if err != nil {
			writeJSON(w, map[string]any{
				"ok":     false,
				"error":  fmt.Sprintf("setup-vless.sh failed: %v", err),
				"output": string(output),
			})
			return
		}

		vlessInitialized = true

		uuid := ""
		if vlessDataDir != "" {
			if u, erru := readVLESSMeta(vlessDataDir); erru == nil {
				uuid = u
			}
		}

		writeJSON(w, map[string]any{
			"ok":      true,
			"uuid":    uuid,
			"port":    vlessPort,
			"output":  string(output),
			"message": "VLESS server initialized and started. Run /api/paranoidx/vless/rotate to change UUID.",
		})
	}
}

// VLESSRotateHandler generates a new UUID and restarts the VLESS server.
func VLESSRotateHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}

		scriptPath := filepath.Join(os.Getenv("HOME"), "simplex-node", "scripts", "setup-vless.sh")
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

		// Restart VLESS server to pick up new config
		_ = exec.Command("systemctl", "--user", "restart", "vless-server.service").Run()

		// Also restart main xray to pick up updated client outbound
		pgrep := exec.Command("pgrep", "-x", "xray")
		if xrayPID, err := pgrep.Output(); err == nil {
			pid := string(xrayPID)
			_ = exec.Command("kill", "-HUP", "xray").Run()
			if !isVLESSRunning() {
				_ = exec.Command("kill", strings.TrimSpace(pid)).Run()
				time.Sleep(500 * time.Millisecond)
				_ = exec.Command("bash", "-c",
					"nohup /home/tomas/bin/v2ray/xray run -c /home/tomas/bin/v2ray/config.json &>/dev/null &").Run()
			}
		}

		writeJSON(w, map[string]any{
			"ok":      true,
			"uuid":    newUUID,
			"port":    vlessPort,
			"output":  string(output),
			"message": "UUID rotated, VLESS server restarted with new UUID.",
		})
	}
}

// VLESSConfigHandler returns the current VLESS client config (for external clients).
func VLESSConfigHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		running := isVLESSRunning()
		uuid := ""
		if vlessDataDir != "" {
			if u, err := readVLESSMeta(vlessDataDir); err == nil {
				uuid = u
			}
		}

		clientConfig := map[string]any{
			"protocol": "vless",
			"settings": map[string]any{
				"vnext": []map[string]any{
					{
						"address": "127.0.0.1",
						"port":    vlessPort,
						"users": []map[string]any{
							{
								"id":         uuid,
								"flow":       "xtls-rprx-vision",
								"encryption": "none",
							},
						},
					},
				},
			},
			"streamSettings": map[string]any{
				"network":  "tcp",
				"security": "reality",
				"realitySettings": map[string]any{
					"show":       false,
					"fingerprint": "chrome",
					"serverName":  "www.microsoft.com",
					"publicKey":   "REALITY_PUBLIC_KEY_PLACEHOLDER",
					"shortId":     "REALITY_SHORT_ID_PLACEHOLDER",
				},
			},
		}

		writeJSON(w, map[string]any{
			"ok":           true,
			"running":      running,
			"uuid":         uuid,
			"port":         vlessPort,
			"clientConfig": clientConfig,
			"serverConfig": map[string]any{
				"port":     vlessPort,
				"protocol": "vless",
			},
		})
	}
}

// InitVLESS initialises the VLESS data directory and runs auto-init if not yet set up.
func InitVLESS(dataDir string) {
	vlessDataDir = dataDir
	_ = ensureVLESSDir(dataDir)

	// Check if already initialized
	if _, err := readVLESSMeta(dataDir); err == nil {
		vlessInitialized = true
	}

	// Auto-init if not yet initialized
	if !vlessInitialized {
		slog.Info("vless: not initialized, running setup-vless.sh")
		scriptPath := filepath.Join(os.Getenv("HOME"), "simplex-node", "scripts", "setup-vless.sh")
		if _, err := os.Stat(scriptPath); err == nil {
			cmd := exec.Command("bash", scriptPath)
			if out, err := cmd.CombinedOutput(); err != nil {
				slog.Warn("vless: auto-init failed", "error", err, "output", string(out))
			} else {
				vlessInitialized = true
				slog.Info("vless: auto-init complete", "output", string(out))
			}
		} else {
			slog.Warn("vless: setup-vless.sh not found, skipping auto-init", "path", scriptPath)
		}
	}
}

// randHex is defined in paranoidx_vmess.go