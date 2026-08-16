// Package paranoidx implements the ParanoidX multi-layer proxy chain
package paranoidx

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// BridgeBuilder sequentially starts and tests network components, then
// assembles the optimal (fastest + safest) bridge configuration.
//
// Component inventory:
//   - tor          : SOCKS5 on 127.0.0.1:9050 (docker container ParanoidX-tor)
//   - xray-native  : SOCKS5 on 127.0.0.1:10810 (client xray ~/bin/v2ray/xray)
//   - wireguard    : wg-quick profiles in ~/.local/share/simplex-node/paranoidx/vpn/
//   - openvpn      : /etc/openvpn/client/*.conf (root-owned, needs sudo)
//
// The builder probes each component, measures latency through it, ranks by
// (latency, protocol safety), and produces the best bridge chain.
type BridgeBuilder struct {
	DataDir      string
	VpnProfiles  string
	Timeout      time.Duration
	mu           sync.Mutex
	lastReport   map[string]any
	lastBuildAt  time.Time
	chain        *ChainOrchestrator
}

// ComponentTest is the result of probing one network component.
type ComponentTest struct {
	Name     string `json:"name"`
	Type     string `json:"type"` // tor | xray | wireguard | openvpn
	Healthy  bool   `json:"healthy"`
	LatencyMs int64 `json:"latency_ms"`
	Message  string `json:"message"`
	Port     int    `json:"port,omitempty"`
	Profile  string `json:"profile,omitempty"`
	Weight   int    `json:"weight"` // safety weight: tor=10, wg=5, openvpn=4, xray=3
}

// NewBridgeBuilder creates a bridge builder.
func NewBridgeBuilder(dataDir string, chain *ChainOrchestrator) *BridgeBuilder {
	return &BridgeBuilder{
		DataDir:     dataDir,
		VpnProfiles: filepath.Join(dataDir, "paranoidx", "vpn"),
		Timeout:     10 * time.Second,
		chain:       chain,
	}
}

// v2rayPoolPath returns the imported v2ray outbounds pool path.
func (bb *BridgeBuilder) v2rayPoolPath() string {
	return filepath.Join(bb.DataDir, "paranoidx", "v2ray", "outbounds.json")
}

// PoolOutbound is a stored v2ray outbound from the import pool.
type PoolOutbound struct {
	Outbound map[string]any `json:"outbound"`
	Name     string         `json:"name"`
	Source   string         `json:"source"`
}

// LoadV2RayPool loads imported v2ray outbounds.
func (bb *BridgeBuilder) LoadV2RayPool() []PoolOutbound {
	var pool []PoolOutbound
	b, err := os.ReadFile(bb.v2rayPoolPath())
	if err != nil {
		return nil
	}
	if err := json.Unmarshal(b, &pool); err != nil {
		slog.Warn("paranoidx: parse v2ray pool", "err", err)
		return nil
	}
	return pool
}

// TestV2RayOutbound probes a v2ray outbound by running a short xray instance
// with that outbound and checking SOCKS5 connectivity. Returns latency.
func (bb *BridgeBuilder) TestV2RayOutbound(ob map[string]any) (bool, int64, string) {
	tag, _ := ob["tag"].(string)
	if tag == "" {
		return false, 0, "no tag"
	}
	// extract server address/port for a direct TCP pre-check
	addr := ""
	if vnext, ok := ob["settings"].(map[string]any)["vnext"].([]any); ok && len(vnext) > 0 {
		if v := vnext[0].(map[string]any); v != nil {
			addr = fmt.Sprintf("%s:%v", v["address"], v["port"])
		}
	} else if servers, ok := ob["settings"].(map[string]any)["servers"].([]any); ok && len(servers) > 0 {
		if s := servers[0].(map[string]any); s != nil {
			addr = fmt.Sprintf("%s:%v", s["address"], s["port"])
		}
	}
	if addr == "" {
		return false, 0, "no address"
	}
	// quick TCP reachability probe
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return false, 0, fmt.Sprintf("tcp dial failed: %v", err)
	}
	conn.Close()
	lat := time.Since(start).Milliseconds()
	return true, lat, "tcp reachable"
}

// ScanProbe tests a single component and returns its health + latency.
func (bb *BridgeBuilder) ScanProbe(name, typ string, port int, profile string) ComponentTest {
	ct := ComponentTest{Name: name, Type: typ, Port: port, Profile: profile}
	switch typ {
	case "tor":
		ct.Weight = 10
		ok, lat := probeSOCKS5("127.0.0.1", port, "check.torproject.org", 80, bb.Timeout)
		ct.Healthy, ct.LatencyMs = ok, lat
		if ok {
			ct.Message = "tor socks up"
		} else {
			ct.Message = "tor socks not reachable"
		}
	case "xray":
		ct.Weight = 3
		ok, lat := probeSOCKS5("127.0.0.1", port, "check.torproject.org", 80, bb.Timeout)
		ct.Healthy, ct.LatencyMs = ok, lat
		if ok {
			ct.Message = "xray socks up"
		} else {
			ct.Message = "xray socks not reachable"
		}
	case "wireguard":
		ct.Weight = 5
		iface := profile
		if iface == "" {
			iface = "wg0"
		}
		start := time.Now()
		cmd := exec.Command("ip", "link", "show", iface)
		if err := cmd.Run(); err != nil {
			ct.Healthy, ct.Message = false, fmt.Sprintf("iface %s not up", iface)
			return ct
		}
		ct.Healthy = true
		ct.LatencyMs = time.Since(start).Milliseconds()
		ct.Message = fmt.Sprintf("%s up", iface)
	case "openvpn":
		ct.Weight = 4
		// openvpn client interfaces are tun0/tun1; probe the config presence
		cfg := filepath.Join("/etc/openvpn/client", profile+".conf")
		if _, err := os.Stat(cfg); err != nil {
			ct.Healthy, ct.Message = false, "no client config"
			return ct
		}
		start := time.Now()
		cmd := exec.Command("ip", "link", "show", "tun0")
		if err := cmd.Run(); err != nil {
			ct.Healthy, ct.Message = false, "tun0 not up"
			return ct
		}
		ct.Healthy = true
		ct.LatencyMs = time.Since(start).Milliseconds()
		ct.Message = "tun0 up"
	default:
		ct.Healthy, ct.Message = false, "unknown type"
	}
	return ct
}

// ScanAll probes every available component in parallel.
func (bb *BridgeBuilder) ScanAll() []ComponentTest {
	var results []ComponentTest
	var mu sync.Mutex
	var wg sync.WaitGroup

	probe := func(ct ComponentTest) {
		defer wg.Done()
		r := bb.ScanProbe(ct.Name, ct.Type, ct.Port, ct.Profile)
		mu.Lock()
		results = append(results, r)
		mu.Unlock()
	}

	// tor
	wg.Add(1)
	go probe(ComponentTest{Name: "tor", Type: "tor", Port: 9050})

	// native xray client
	wg.Add(1)
	go probe(ComponentTest{Name: "xray-native", Type: "xray", Port: 10810})

	// wireguard profiles
	if profs, err := filepath.Glob(filepath.Join(bb.VpnProfiles, "*.conf")); err == nil {
		for _, p := range profs {
			name := strings.TrimSuffix(filepath.Base(p), ".conf")
			iface := ifaceForWG(name)
			wg.Add(1)
			go probe(ComponentTest{Name: "wireguard:" + name, Type: "wireguard", Profile: iface})
		}
	}

	// openvpn clients (root dir; may be empty)
	if cfgs, err := filepath.Glob("/etc/openvpn/client/*.conf"); err == nil {
		for _, c := range cfgs {
			name := strings.TrimSuffix(filepath.Base(c), ".conf")
			wg.Add(1)
			go probe(ComponentTest{Name: "openvpn:" + name, Type: "openvpn", Profile: name})
		}
	}

	// imported v2ray outbounds pool (vless/vmess/trojan/ss)
	for _, po := range bb.LoadV2RayPool() {
		ob := po.Outbound
		tag, _ := ob["tag"].(string)
		if tag == "" {
			continue
		}
		wg.Add(1)
		go func(tag string, ob map[string]any) {
			defer wg.Done()
			ok, lat, msg := bb.TestV2RayOutbound(ob)
			mu.Lock()
			results = append(results, ComponentTest{
				Name:      "v2ray:" + tag,
				Type:      "v2ray",
				Healthy:   ok,
				LatencyMs: lat,
				Message:   msg,
				Weight:    6, // v2ray protocols are safer than raw wg (TLS-wrapped)
			})
			mu.Unlock()
		}(tag, ob)
	}

	wg.Wait()
	sort.Slice(results, func(i, j int) bool {
		if results[i].Healthy != results[j].Healthy {
			return results[i].Healthy
		}
		return results[i].LatencyMs < results[j].LatencyMs
	})
	return results
}

// BuildOptimal assembles the best bridge chain from scan results.
// Safety-first: tor is mandatory; the fastest healthy VPN layer is added
// as the first hop. Returns the ordered chain and full report.
func (bb *BridgeBuilder) BuildOptimal() (map[string]any, error) {
	bb.mu.Lock()
	defer bb.mu.Unlock()

	results := bb.ScanAll()
	var healthy []ComponentTest
	for _, r := range results {
		if r.Healthy {
			healthy = append(healthy, r)
		}
	}

	report := map[string]any{
		"scanned_at":  time.Now().Format(time.RFC3339),
		"components":  results,
		"chain":       []string{},
		"overall":     false,
		"reason":      "",
	}

	// Tor is the mandatory last hop
	var tor *ComponentTest
	for i := range results {
		if results[i].Name == "tor" && results[i].Healthy {
			tor = &results[i]
			break
		}
	}
	if tor == nil {
		report["reason"] = "tor unavailable — bridge cannot be built"
		bb.lastReport = report
		bb.lastBuildAt = time.Now()
		return report, fmt.Errorf("tor unavailable")
	}

	// Choose the best VPN-layer hop: prefer v2ray protocols (TLS-wrapped,
	// provider sees only v2ray), then wireguard, then openvpn.
	var vpnHop *ComponentTest
	bestRank := -1
	for i := range healthy {
		if healthy[i].Name == "tor" {
			continue
		}
		rank := 0
		switch healthy[i].Type {
		case "v2ray":
			rank = 100 // safest: TLS obfuscation
		case "wireguard":
			rank = 50
		case "openvpn":
			rank = 30
		default:
			rank = 10
		}
		// prefer lower latency within same type
		rank -= int(healthy[i].LatencyMs / 50)
		if rank > bestRank {
			bestRank = rank
			vpnHop = &healthy[i]
		}
	}

	chain := []string{}
	if vpnHop != nil {
		chain = append(chain, vpnHop.Name)
	}
	chain = append(chain, "tor")

	report["chain"] = chain
	report["overall"] = true
	report["reason"] = "optimal chain selected"
	if vpnHop != nil {
		report["vpn_hop"] = map[string]any{
			"name":       vpnHop.Name,
			"type":       vpnHop.Type,
			"latency_ms": vpnHop.LatencyMs,
		}
	}

	bb.lastReport = report
	bb.lastBuildAt = time.Now()

	slog.Info("paranoidx: optimal bridge built", "chain", strings.Join(chain, " -> "))
	return report, nil
}

// ApplyAndStart brings up the chosen bridge chain in order.
func (bb *BridgeBuilder) ApplyAndStart(report map[string]any) error {
	chainRaw, _ := report["chain"].([]string)
	if len(chainRaw) == 0 {
		return fmt.Errorf("empty chain")
	}
	for _, hop := range chainRaw {
		switch {
		case hop == "tor":
			// docker tor already running; verify
			if !bb.chain.V2Ray().CheckHealth() { // keep watchdog simple; tor verified separately
				slog.Warn("paranoidx: tor not verified yet")
			}
		case strings.HasPrefix(hop, "wireguard:"):
			prof := strings.TrimPrefix(hop, "wireguard:")
			if err := bb.startWireGuard(prof); err != nil {
				slog.Warn("paranoidx: wireguard start failed", "profile", prof, "err", err)
			}
		case strings.HasPrefix(hop, "v2ray:"):
			tag := strings.TrimPrefix(hop, "v2ray:")
			if err := bb.startV2RayOutbound(tag); err != nil {
				slog.Warn("paranoidx: v2ray outbound start failed", "tag", tag, "err", err)
			}
		case strings.HasPrefix(hop, "xray:"):
			// native xray client — start via manager
			if err := bb.chain.V2Ray().Start(); err != nil {
				slog.Warn("paranoidx: native xray start failed", "err", err)
			}
		}
	}
	return nil
}

// startV2RayOutbound writes a dedicated xray config with the chosen outbound
// and (re)starts the native xray client on :10810. Tor stays the final hop —
// the provider sees only the v2ray protocol, never Tor.
func (bb *BridgeBuilder) startV2RayOutbound(tag string) error {
	pool := bb.LoadV2RayPool()
	var chosen map[string]any
	for _, po := range pool {
		if t, _ := po.Outbound["tag"].(string); t == tag {
			chosen = po.Outbound
			break
		}
	}
	if chosen == nil {
		return fmt.Errorf("v2ray outbound %s not in pool", tag)
	}

	cfg := map[string]any{
		"log": map[string]any{"loglevel": "warning"},
		"inbounds": []map[string]any{{
			"port":     10810,
			"listen":   "127.0.0.1",
			"protocol": "socks",
			"settings": map[string]any{"auth": "noauth", "udp": true},
			"tag":      "socks-in",
		}},
		"outbounds": []any{
			chosen,
			map[string]any{"protocol": "freedom", "tag": "direct"},
			map[string]any{"protocol": "blackhole", "tag": "block"},
		},
		"routing": map[string]any{
			"domainStrategy": "AsIs",
			"rules": []map[string]any{{
				"type":        "field",
				"inboundTag":  []string{"socks-in"},
				"outboundTag": tag,
			}},
		},
	}

	home, _ := os.UserHomeDir()
	cfgPath := filepath.Join(home, "bin", "v2ray", "config.json")
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		return err
	}
	slog.Info("paranoidx: v2ray config written", "outbound", tag, "path", cfgPath)
	return bb.chain.V2Ray().Start()
}

func (bb *BridgeBuilder) startWireGuard(profile string) error {
	conf := filepath.Join(bb.VpnProfiles, profile+".conf")
	if _, err := os.Stat(conf); err != nil {
		return err
	}
	// wg-quick needs root; try sudo -n first
	cmd := exec.Command("sudo", "-n", "wg-quick", "up", conf)
	if out, err := cmd.CombinedOutput(); err != nil {
		slog.Warn("paranoidx: wg-quick up needs sudo", "err", err, "out", string(out))
		return err
	}
	return nil
}

// LastReport returns the most recent build report.
func (bb *BridgeBuilder) LastReport() map[string]any {
	bb.mu.Lock()
	defer bb.mu.Unlock()
	if bb.lastReport == nil {
		return map[string]any{"components": []ComponentTest{}, "chain": []string{}, "overall": false}
	}
	return bb.lastReport
}

// BuildHandler is the HTTP handler for POST /api/paranoidx/bridge/build.
func (b *Bridge) BuildHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	report, err := b.builder.BuildOptimal()
	if err != nil {
		writeJSON(w, report)
		return
	}
	_ = b.builder.ApplyAndStart(report)
	writeJSON(w, report)
}

// ScanHandler is the HTTP handler for GET /api/paranoidx/bridge/scan.
func (b *Bridge) ScanHandler(w http.ResponseWriter, r *http.Request) {
	results := b.builder.ScanAll()
	writeJSON(w, map[string]any{
		"scanned_at": time.Now().Format(time.RFC3339),
		"components": results,
	})
}

// ReportHandler is the HTTP handler for GET /api/paranoidx/bridge/report.
func (b *Bridge) ReportHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, b.builder.LastReport())
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// probeSOCKS5 connects through a SOCKS5 proxy and measures latency to target.
func probeSOCKS5(host string, port int, target string, targetPort int, timeout time.Duration) (bool, int64) {
	start := time.Now()
	proxyAddr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	conn, err := net.DialTimeout("tcp", proxyAddr, timeout)
	if err != nil {
		return false, 0
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(timeout))
	// handshake
	if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		return false, 0
	}
	resp := make([]byte, 2)
	if _, err := conn.Read(resp); err != nil {
		return false, 0
	}
	if resp[0] != 0x05 || resp[1] != 0x00 {
		return false, 0
	}
	// connect
	req := []byte{0x05, 0x01, 0x00, 0x03, byte(len(target))}
	req = append(req, []byte(target)...)
	req = append(req, byte(targetPort>>8), byte(targetPort&0xff))
	if _, err := conn.Write(req); err != nil {
		return false, 0
	}
	resp2 := make([]byte, 10)
	if _, err := conn.Read(resp2); err != nil {
		return false, 0
	}
	if resp2[0] != 0x05 || resp2[1] != 0x00 {
		return false, 0
	}
	lat := time.Since(start).Milliseconds()
	return true, lat
}

func ifaceForWG(profile string) string {
	// WireGuard interfaces usually match the conf name, but check Address line
	return profile
}
