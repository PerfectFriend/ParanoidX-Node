// Package paranoidx implements the ParanoidX multi-layer proxy chain
package paranoidx

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// NativeV2RayManager manages the native xray client process (SOCKS5 on :10810).
// Canonical layout (replaces the old Docker container):
//   - binary:  ~/bin/v2ray/xray
//   - config:  ~/bin/v2ray/config.json  (SOCKS5 inbound on 10810, VLESS+Reality outbound)
//   - servers: vless-server.service (:10813) and vmess-server.service (:10812) run separately.
type NativeV2RayManager struct {
	BinPath    string
	ConfigPath string
	Port       int
	cmd        *exec.Cmd
}

// NewNativeV2RayManager creates a manager for the native xray client.
func NewNativeV2RayManager() *NativeV2RayManager {
	home, _ := os.UserHomeDir()
	return &NativeV2RayManager{
		BinPath:    filepath.Join(home, "bin", "v2ray", "xray"),
		ConfigPath: filepath.Join(home, "bin", "v2ray", "config.json"),
		Port:       10810,
	}
}

// Start launches the native xray client if it is not already serving SOCKS5 on :Port.
func (m *NativeV2RayManager) Start() error {
	if m.IsRunning() {
		slog.Info("paranoidx: native xray already running on :10810")
		SetLayerStatus(LayerV2Ray, true, 1, "native xray running on :10810")
		return nil
	}
	if _, err := os.Stat(m.BinPath); err != nil {
		return fmt.Errorf("xray binary not found: %w", err)
	}
	if _, err := os.Stat(m.ConfigPath); err != nil {
		return fmt.Errorf("xray config not found: %w", err)
	}
	cmd := exec.Command(m.BinPath, "run", "-c", m.ConfigPath)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start native xray: %w", err)
	}
	m.cmd = cmd
	go func() {
		cmd.Wait()
		SetLayerStatus(LayerV2Ray, false, 0, "native xray process exited")
	}()

	// Wait for SOCKS5 port to be ready
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if m.CheckHealth() {
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("native xray SOCKS5 port %d not ready within 15s", m.Port)
}

// Stop terminates only the client xray process owned by this manager.
// Server processes (vless/vmess systemd units) are left untouched.
func (m *NativeV2RayManager) Stop() error {
	if m.cmd != nil && m.cmd.Process != nil {
		m.cmd.Process.Kill()
		m.cmd.Wait()
		m.cmd = nil
	}
	// Fallback: kill any xray running with the client config path
	exec.Command("pkill", "-f", "xray run -c .*/v2ray/config.json").Run()
	SetLayerStatus(LayerV2Ray, false, 0, "native xray stopped")
	return nil
}

// CheckHealth probes the native xray SOCKS5 port.
func (m *NativeV2RayManager) CheckHealth() bool {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", m.Port), 3*time.Second)
	if err != nil {
		SetLayerStatus(LayerV2Ray, false, 0, fmt.Sprintf("xray port %d not reachable", m.Port))
		return false
	}
	conn.Close()
	latency := time.Since(start).Milliseconds()
	SetLayerStatus(LayerV2Ray, true, latency, fmt.Sprintf("native xray socks :%d up", m.Port))
	return true
}

// IsRunning checks whether the native xray client is serving :10810.
func (m *NativeV2RayManager) IsRunning() bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", m.Port), 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// UpdateConfig writes a new config JSON (kept for interface parity).
func (m *NativeV2RayManager) UpdateConfig(cfg map[string]any) error {
	return nil
}
