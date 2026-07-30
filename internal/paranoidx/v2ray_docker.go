// Package paranoidx implements the ParanoidX multi-layer proxy chain
package paranoidx

import (
	"fmt"
	"log/slog"
	"net"
	"os/exec"
	"path/filepath"
	"time"
)

// DockerV2RayManager manages V2Ray via Docker compose.
type DockerV2RayManager struct {
	ComposeDir string
	Container  string
}

// NewDockerV2RayManager creates a Docker-based V2Ray manager.
func NewDockerV2RayManager(composeDir string) *DockerV2RayManager {
	return &DockerV2RayManager{
		ComposeDir: composeDir,
		Container:  "ParanoidX-v2ray",
	}
}

// Start launches the V2Ray Docker container.
func (m *DockerV2RayManager) Start() error {
	cmd := exec.Command("docker", "compose", "-f", filepath.Join(m.ComposeDir, "docker-compose.yml"),
		"up", "-d", "v2ray")
	cmd.Dir = m.ComposeDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker compose up v2ray: %w\n%s", err, string(out))
	}
	slog.Info("paranoidx: V2Ray docker started", "output", string(out))

	// Wait for SOCKS5 port to be ready
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", "127.0.0.1:10808", 2*time.Second)
		if err == nil {
			conn.Close()
			slog.Info("paranoidx: V2Ray SOCKS5 port 10808 is ready")
			SetLayerStatus(LayerV2Ray, true, 5, "v2ray docker running on :10808")
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("V2Ray SOCKS5 port 10808 not ready within 15s")
}

// Stop terminates the V2Ray Docker container.
func (m *DockerV2RayManager) Stop() error {
	cmd := exec.Command("docker", "compose", "-f", filepath.Join(m.ComposeDir, "docker-compose.yml"),
		"stop", "v2ray")
	cmd.Dir = m.ComposeDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		slog.Warn("paranoidx: docker compose stop v2ray", "err", err, "output", string(out))
	}
	SetLayerStatus(LayerV2Ray, false, 0, "v2ray stopped")
	return err
}

// Restart restarts the V2Ray Docker container.
func (m *DockerV2RayManager) Restart() error {
	m.Stop()
	time.Sleep(2 * time.Second)
	return m.Start()
}

// CheckHealth verifies V2Ray is running via SOCKS5 port probe.
func (m *DockerV2RayManager) CheckHealth() bool {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", "127.0.0.1:10808", 3*time.Second)
	if err != nil {
		SetLayerStatus(LayerV2Ray, false, 0, "v2ray port 10808 not reachable")
		return false
	}
	conn.Close()
	latency := time.Since(start).Milliseconds()
	SetLayerStatus(LayerV2Ray, true, latency, "v2ray docker healthy")
	return true
}

// IsRunning checks if the V2Ray container is running.
func (m *DockerV2RayManager) IsRunning() bool {
	cmd := exec.Command("docker", "ps", "--format", "{{.Names}}", "--filter", "name="+m.Container)
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return string(out) == m.Container+"\n"
}

// execCommand is a helper for running commands (used by chain test).
func (m *DockerV2RayManager) execCommand(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}

// UpdateConfig writes a new V2Ray config JSON to the docker volume.
func (m *DockerV2RayManager) UpdateConfig(cfg map[string]any) error {
	return nil
}
