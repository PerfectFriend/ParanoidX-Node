// Package paranoidx implements the ParanoidX multi-layer proxy chain
package paranoidx

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Bridge is the top-level ParanoidX multi-layer proxy coordinator.
// It manages health checks across all four layers, orchestrates the
// proxy chain build/teardown, and provides a unified status API.
type Bridge struct {
	mu           sync.RWMutex
	ctx          context.Context
	cancel       context.CancelFunc
	chain        *ChainOrchestrator
	builder      *BridgeBuilder
	torSocksPort int
	vpnIface     string
	v2rayPort    int
	httpClient   *http.Client

	// Connection chain config
	V2RayEnabled bool
	VPNEnabled   bool   // disabled by default; fallback when V2Ray not available
	TorEnabled   bool
	SimplexPort  int
}

// NewBridge creates a ParanoidX bridge with default settings.
// composeDir should point to the docker/ directory containing docker-compose.yml.
func NewBridge(dataDir, composeDir string, torSocksPort int, vpnIface string, simplexPort int) *Bridge {
	ctx, cancel := context.WithCancel(context.Background())
	chain := NewChainOrchestrator(dataDir, composeDir)
	return &Bridge{
		ctx:          ctx,
		cancel:       cancel,
		chain:        chain,
		builder:      NewBridgeBuilder(dataDir, chain),
		torSocksPort: torSocksPort,
		vpnIface:     vpnIface,
		v2rayPort:    10810,
		SimplexPort:  simplexPort,
		V2RayEnabled: true,
		VPNEnabled:   false, // disabled by default; enable when V2Ray unavailable
		TorEnabled:   true,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout: 5 * time.Second,
				}).DialContext,
			},
		},
	}
}

// Start begins ParanoidX health monitoring and optionally builds the
// proxy chain. Non-fatal: if V2Ray is not available, the health loop
// still monitors remaining layers and reports V2Ray as unhealthy.
func (b *Bridge) Start() error {
	SetLayerStatus(LayerV2Ray, false, 0, "starting")
	if b.VPNEnabled {
		SetLayerStatus(LayerVPN, false, 0, "checking")
	}
	SetLayerStatus(LayerTor, false, 0, "checking")
	SetLayerStatus(LayerSimpleX, false, 0, "checking")

	// Run initial health check and begin monitoring loop
	go b.healthLoop()
	return nil
}

// Stop gracefully shuts down all layers and tears down the chain.
func (b *Bridge) Stop() {
	b.cancel()
	b.chain.TeardownChain()
}

// healthLoop runs periodic health checks on all layers.
func (b *Bridge) healthLoop() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	b.checkAll()

	for {
		select {
		case <-b.ctx.Done():
			return
		case <-ticker.C:
			b.checkAll()
		}
	}
}

// checkAll probes every layer and updates global status.
func (b *Bridge) checkAll() {
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		if b.V2RayEnabled {
			b.checkV2Ray()
		}
	}()

	if b.VPNEnabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b.checkVPN()
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		b.checkTor()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		b.checkSimplex()
	}()

	wg.Wait()
}

func (b *Bridge) checkV2Ray() {
	b.chain.V2Ray().CheckHealth()
}

func (b *Bridge) checkVPN() {
	b.mu.RLock()
	iface := b.vpnIface
	b.mu.RUnlock()
	// Check the configured interface directly, ignoring VPNManager profile logic
	start := time.Now()
	cmd := exec.Command("ip", "link", "show", iface)
	if err := cmd.Run(); err != nil {
		SetLayerStatus(LayerVPN, false, 0, fmt.Sprintf("interface %s not found", iface))
		return
	}
	latencyMs := time.Since(start).Milliseconds()
	SetLayerStatus(LayerVPN, true, latencyMs, fmt.Sprintf("%s up", iface))
}

func (b *Bridge) checkTor() {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", b.torSocksPort), 3*time.Second)
	if err != nil {
		SetLayerStatus(LayerTor, false, 0, "tor socks not reachable")
		return
	}
	conn.Close()
	latencyMs := time.Since(start).Milliseconds()
	SetLayerStatus(LayerTor, true, latencyMs, fmt.Sprintf("tor socks :%d up", b.torSocksPort))
}

func (b *Bridge) checkSimplex() {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", b.SimplexPort), 3*time.Second)
	if err != nil {
		SetLayerStatus(LayerSimpleX, false, 0, "simplex bridge not reachable")
		return
	}
	conn.Close()
	latencyMs := time.Since(start).Milliseconds()
	SetLayerStatus(LayerSimpleX, true, latencyMs, fmt.Sprintf("simplex :%d up", b.SimplexPort))
}

func (b *Bridge) checkVMess() {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", "127.0.0.1:10812", 3*time.Second)
	if err != nil {
		SetLayerStatus(LayerVMess, false, 0, "vmess server not reachable on :10812")
		return
	}
	conn.Close()
	latencyMs := time.Since(start).Milliseconds()
	SetLayerStatus(LayerVMess, true, latencyMs, "vmess server on :10812")
}

// GetProxyChain returns the current proxy chain description.
func (b *Bridge) GetProxyChain() []string {
	chain := []string{}
	if b.V2RayEnabled {
		chain = append(chain, "v2ray (socks5://127.0.0.1:10810)")
	}
	if b.VPNEnabled {
		chain = append(chain, fmt.Sprintf("vpn (%s)", b.vpnIface))
	}
	if b.TorEnabled {
		chain = append(chain, fmt.Sprintf("tor (socks5://127.0.0.1:%d)", b.torSocksPort))
	}
	chain = append(chain, "simplex")
	return chain
}

// HTTP handlers for ParanoidX API endpoints.

// StatusHandler returns JSON with all layer statuses.
func (b *Bridge) StatusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(GetOverallStatus())
}

// ConfigHandler returns the current ParanoidX chain configuration.
func (b *Bridge) ConfigHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"v2ray_enabled":  b.V2RayEnabled,
		"vpn_enabled":    b.VPNEnabled,
		"tor_enabled":    b.TorEnabled,
		"tor_socks_port": b.torSocksPort,
		"vpn_interface":  b.vpnIface,
		"simplex_port":   b.SimplexPort,
		"proxy_chain":    b.GetProxyChain(),
	})
}

// ConfigUpdateHandler updates ParanoidX configuration.
func (b *Bridge) ConfigUpdateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var cfg struct {
		V2RayEnabled *bool  `json:"v2ray_enabled"`
		VPNEnabled   *bool  `json:"vpn_enabled"`
		TorEnabled   *bool  `json:"tor_enabled"`
		VpnInterface string `json:"vpn_interface"`
	}
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, fmt.Sprintf("bad request: %v", err), http.StatusBadRequest)
		return
	}
	b.mu.Lock()
	if cfg.V2RayEnabled != nil {
		b.V2RayEnabled = *cfg.V2RayEnabled
	}
	if cfg.VPNEnabled != nil {
		b.VPNEnabled = *cfg.VPNEnabled
	}
	if cfg.TorEnabled != nil {
		b.TorEnabled = *cfg.TorEnabled
	}
	if cfg.VpnInterface != "" {
		b.vpnIface = cfg.VpnInterface
	}
	b.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// ResolveProxyChain returns the dial function for the full proxy chain.
// For now, returns the Tor SOCKS5 address as the final hop.
func (b *Bridge) ResolveProxyChain() string {
	parts := []string{}
	if b.V2RayEnabled {
		parts = append(parts, "v2ray")
	}
	if b.VPNEnabled {
		parts = append(parts, "vpn")
	}
	if b.TorEnabled {
		parts = append(parts, "tor")
	}
	parts = append(parts, "simplex")
	return strings.Join(parts, " -> ")
}
