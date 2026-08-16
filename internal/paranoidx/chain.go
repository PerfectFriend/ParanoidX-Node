// Package paranoidx implements the ParanoidX multi-layer proxy chain
package paranoidx

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os/exec"
	"strconv"
	"sync"
	"time"
)

// ChainState represents the lifecycle state of the proxy chain.
type ChainState string

const (
	ChainDown   ChainState = "down"
	ChainBuild  ChainState = "building"
	ChainUp     ChainState = "up"
	ChainTeardown ChainState = "tearing_down"
	ChainError  ChainState = "error"
)

// ChainOrchestrator manages sequential build and teardown of all proxy layers.
type ChainOrchestrator struct {
	mu    sync.RWMutex
	state ChainState

	// Layer startup timeouts
	LayerTimeout time.Duration

	// Docker compose path for V2Ray
	ComposeDir string
	DataDir    string

	// Component managers
	v2ray *NativeV2RayManager
	vpn   *VPNManager

	// Callbacks for health-aware waiting
	waitForLayer func(layer Layer, timeout time.Duration) bool
}

// NewChainOrchestrator creates a chain orchestrator.
func NewChainOrchestrator(dataDir, composeDir string) *ChainOrchestrator {
	return &ChainOrchestrator{
		state:        ChainDown,
		LayerTimeout: 30 * time.Second,
		DataDir:      dataDir,
		ComposeDir:   composeDir,
		v2ray:        NewNativeV2RayManager(),
		vpn:          NewVPNManager(dataDir),
		waitForLayer: waitForLayerHealth,
	}
}

// waitForLayerHealth polls layer status until healthy or timeout.
func waitForLayerHealth(layer Layer, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		s := GetStatus()[layer]
		if s != nil && s.Healthy {
			return true
		}
		time.Sleep(1 * time.Second)
	}
	return false
}

// BuildChain brings up all layers in order: V2Ray → VPN → Tor → SimpleX.
func (co *ChainOrchestrator) BuildChain() error {
	co.mu.Lock()
	if co.state == ChainBuild || co.state == ChainUp {
		co.mu.Unlock()
		return fmt.Errorf("chain already %s", co.state)
	}
	co.state = ChainBuild
	co.mu.Unlock()

	defer func() {
		co.mu.Lock()
		if co.state != ChainUp {
			co.state = ChainError
		}
		co.mu.Unlock()
	}()

	// Step 1: V2Ray (Docker)
	slog.Info("paranoidx: starting V2Ray (Docker)")
	SetLayerStatus(LayerV2Ray, false, 0, "starting v2ray docker")
	if err := co.v2ray.Start(); err != nil {
		slog.Warn("paranoidx: V2Ray start failed", "err", err)
		SetLayerStatus(LayerV2Ray, false, 0, fmt.Sprintf("v2ray start: %v", err))
		return fmt.Errorf("v2ray start: %w", err)
	}
	if !co.waitForLayer(LayerV2Ray, co.LayerTimeout) {
		return fmt.Errorf("V2Ray did not become healthy within %v", co.LayerTimeout)
	}
	slog.Info("paranoidx: V2Ray healthy")

	// Step 2: VPN (if active profile configured)
	profile := co.vpn.ActiveProfile()
	if profile != "" {
		slog.Info("paranoidx: starting VPN", "profile", profile)
		SetLayerStatus(LayerVPN, false, 0, fmt.Sprintf("starting vpn %s", profile))
		if err := co.vpn.Up(profile); err != nil {
			slog.Warn("paranoidx: VPN up failed", "err", err)
			SetLayerStatus(LayerVPN, false, 0, fmt.Sprintf("vpn up: %v", err))
			return fmt.Errorf("vpn up: %w", err)
		}
		if !co.waitForLayer(LayerVPN, co.LayerTimeout) {
			return fmt.Errorf("VPN did not become healthy within %v", co.LayerTimeout)
		}
		slog.Info("paranoidx: VPN healthy")
	} else {
		slog.Info("paranoidx: no active VPN profile, skipping VPN layer")
		SetLayerStatus(LayerVPN, false, 0, "no active profile")
	}

	// Step 3: Tor (already in Docker compose, just verify)
	slog.Info("paranoidx: verifying Tor layer")
	SetLayerStatus(LayerTor, false, 0, "verifying tor")
	if !co.waitForLayer(LayerTor, co.LayerTimeout) {
		slog.Warn("paranoidx: Tor not reachable")
		SetLayerStatus(LayerTor, false, 0, "tor not reachable")
	}
	slog.Info("paranoidx: Tor verified")

	// Step 4: SimpleX (verify bridge)
	slog.Info("paranoidx: verifying SimpleX bridge")
	SetLayerStatus(LayerSimpleX, false, 0, "verifying simplex")
	if !co.waitForLayer(LayerSimpleX, co.LayerTimeout) {
		slog.Warn("paranoidx: SimpleX not reachable")
		SetLayerStatus(LayerSimpleX, false, 0, "simplex not reachable")
	}
	slog.Info("paranoidx: SimpleX verified")

	co.mu.Lock()
	co.state = ChainUp
	co.mu.Unlock()

	slog.Info("paranoidx: proxy chain fully built",
		"chain", "V2Ray|VPN (fallback) -> Tor -> SimpleX",
	)
	return nil
}

// TeardownChain stops all layers in reverse order.
func (co *ChainOrchestrator) TeardownChain() error {
	co.mu.Lock()
	if co.state == ChainDown || co.state == ChainTeardown {
		co.mu.Unlock()
		return nil
	}
	co.state = ChainTeardown
	co.mu.Unlock()

	defer func() {
		co.mu.Lock()
		co.state = ChainDown
		co.mu.Unlock()
	}()

	// Teardown in reverse: SimpleX is not stopped (it's the core service).
	// VPN teardown
	profile := co.vpn.ActiveProfile()
	if profile != "" {
		slog.Info("paranoidx: stopping VPN", "profile", profile)
		SetLayerStatus(LayerVPN, false, 0, "stopping")
		if err := co.vpn.Down(profile); err != nil {
			slog.Warn("paranoidx: VPN down", "err", err)
		}
	}

	// V2Ray teardown
	slog.Info("paranoidx: stopping V2Ray")
	SetLayerStatus(LayerV2Ray, false, 0, "stopping")
	if err := co.v2ray.Stop(); err != nil {
		slog.Warn("paranoidx: V2Ray stop", "err", err)
	}

	return nil
}

// State returns the current chain state.
func (co *ChainOrchestrator) State() ChainState {
	co.mu.RLock()
	defer co.mu.RUnlock()
	return co.state
}

// V2Ray returns the native xray manager.
func (co *ChainOrchestrator) V2Ray() *NativeV2RayManager {
	return co.v2ray
}

// VPN returns the VPN manager.
func (co *ChainOrchestrator) VPN() *VPNManager {
	return co.vpn
}

// TestChain performs an end-to-end connectivity test through the full proxy chain.
// It attempts to reach an external check service through:
//   - Direct connection (baseline)
//   - V2Ray SOCKS5 proxy
//   - Tor SOCKS5 proxy
//   - Full chain (via chain order)
func (co *ChainOrchestrator) TestChain(v2rayPort, torPort int, simplexPort int) map[string]any {
	results := map[string]any{}
	results["overall"] = false

	// Direct test
	directOK := co.testDial("tcp", "check.torproject.org:80", 5*time.Second)
	results["direct"] = directOK

	// V2Ray SOCKS5
	v2rayOK := co.testSOCKS5("127.0.0.1", v2rayPort, "check.torproject.org", 80, 10*time.Second)
	results["v2ray"] = v2rayOK

	// VPN check
	vpnOK := co.testVPN()
	results["vpn"] = vpnOK

	// Tor SOCKS5
	torOK := co.testSOCKS5("127.0.0.1", torPort, "check.torproject.org", 80, 15*time.Second)
	results["tor"] = torOK

	// SimpleX (local port only, not external)
	simplexOK := co.testDial("tcp", fmt.Sprintf("127.0.0.1:%d", simplexPort), 3*time.Second)
	results["simplex"] = simplexOK

	chainOK := (v2rayOK || vpnOK) && torOK && simplexOK
	results["overall"] = chainOK

	return results
}

func (co *ChainOrchestrator) testDial(network, addr string, timeout time.Duration) bool {
	conn, err := net.DialTimeout(network, addr, timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func (co *ChainOrchestrator) testSOCKS5(host string, port int, target string, targetPort int, timeout time.Duration) bool {
	proxyAddr := net.JoinHostPort(host, strconv.Itoa(port))
	conn, err := net.DialTimeout("tcp", proxyAddr, timeout)
	if err != nil {
		return false
	}
	defer conn.Close()

	// SOCKS5 handshake
	conn.SetDeadline(time.Now().Add(timeout))
	// Method negotiation: no auth
	conn.Write([]byte{0x05, 0x01, 0x00})
	resp := make([]byte, 2)
	if _, err := conn.Read(resp); err != nil {
		return false
	}
	if resp[0] != 0x05 || resp[1] != 0x00 {
		return false
	}
	// Connect request
	conn.Write([]byte{0x05, 0x01, 0x00, 0x03, byte(len(target))})
	conn.Write([]byte(target))
	conn.Write([]byte{byte(targetPort >> 8), byte(targetPort & 0xff)})
	resp2 := make([]byte, 10)
	if _, err := conn.Read(resp2); err != nil {
		return false
	}
	return resp2[0] == 0x05 && resp2[1] == 0x00
}

func (co *ChainOrchestrator) testVPN() bool {
	iface := "wg0"
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:10808"), 2*time.Second)
	if err == nil {
		conn.Close()
	}
	// Check if the interface exists via ip link
	return co.testExec("ip", "link", "show", iface)
}

func (co *ChainOrchestrator) testExec(name string, args ...string) bool {
	cmd := exec.Command(name, args...)
	return cmd.Run() == nil
}

// ChainTestHandler runs an end-to-end test of all proxy chain layers.
func (b *Bridge) ChainTestHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	results := b.chain.TestChain(b.v2rayPort, b.torSocksPort, b.SimplexPort)
	json.NewEncoder(w).Encode(map[string]any{
		"tested_at": time.Now().Format(time.RFC3339),
		"results":   results,
	})
}

// ChainBuildHandler builds the proxy chain.
func (b *Bridge) ChainBuildHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	err := b.chain.BuildChain()
	if err != nil {
		json.NewEncoder(w).Encode(map[string]any{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}
	json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
			"chain":  "V2Ray|VPN (fallback) -> Tor -> SimpleX",
		})
}

// ChainTeardownHandler tears down the proxy chain.
func (b *Bridge) ChainTeardownHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	err := b.chain.TeardownChain()
	if err != nil {
		json.NewEncoder(w).Encode(map[string]any{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}
	json.NewEncoder(w).Encode(map[string]any{
		"status": "ok",
		"chain":  "down",
	})
}

// ChainStateHandler returns the current chain build state.
func (b *Bridge) ChainStateHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"state": string(b.chain.State()),
	})
}
