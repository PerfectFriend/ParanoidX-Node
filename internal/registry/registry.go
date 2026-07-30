// Package registry manages node discovery, regional routing, and health tracking
// for the relay mesh network.
package registry

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"ParanoidX/internal/fileutil"
)

// NodeCap describes what services a node provides.
type NodeCap string

const (
	CapRelaySMP  NodeCap = "relay-smp"
	CapRelayXFTP NodeCap = "relay-xftp"
	CapRadioSeed NodeCap = "radio-seed"
	CapVaultPeer NodeCap = "vault-peer"
	CapDCSeed    NodeCap = "dc-seed"
)

// NodeInfo is what a node announces about itself.
type NodeInfo struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Region       string    `json:"region"`
	OnionAddr    string    `json:"onion_addr"`
	DirectAddr   string    `json:"direct_addr,omitempty"` // ip:port for P2P
	Capabilities []NodeCap `json:"capabilities"`
	FeeBps       int       `json:"fee_bps,omitempty"` // commission basis points
	PublicKey    string    `json:"public_key"`
	Version      string    `json:"version"`
	LastSeen     time.Time `json:"last_seen"`
	Status       string    `json:"status"` // online, offline, overloaded
	StakeNg      int64     `json:"stake_ng"`

	// Runtime stats
	LoadAvg    float64 `json:"load_avg,omitempty"`
	UptimeHrs  float64 `json:"uptime_hrs,omitempty"`
	ClientDist int     `json:"client_dist,omitempty"` // number of clients served
}

// Registry tracks all known nodes and provides regional lookup.
type Registry struct {
	mu       sync.RWMutex
	nodes    map[string]*NodeInfo
	dataDir  string
	httpCli  *http.Client
}


// NewRegistry handles the NewRegistry HTTP request.
func NewRegistry(dataDir string) *Registry {
	r := &Registry{
		nodes:   make(map[string]*NodeInfo),
		dataDir: dataDir,
		httpCli: &http.Client{Timeout: 5 * time.Second},
	}
	r.load()
	go r.healthLoop()
	return r
}

func (r *Registry) load() {
	var nodes []*NodeInfo
	b, err := os.ReadFile(filepath.Join(r.dataDir, "registry_nodes.json"))
	if err == nil {
		json.Unmarshal(b, &nodes)
	}
	r.mu.Lock()
	for _, n := range nodes {
		r.nodes[n.ID] = n
	}
	r.mu.Unlock()
}

func (r *Registry) save() {
	r.mu.RLock()
	nodes := make([]*NodeInfo, 0, len(r.nodes))
	for _, n := range r.nodes {
		nodes = append(nodes, n)
	}
	r.mu.RUnlock()
	fileutil.WriteJSON(filepath.Join(r.dataDir, "registry_nodes.json"), nodes)
}

// Announce registers or updates a node. Returns the list of nearby nodes.
func (r *Registry) Announce(n *NodeInfo) []*NodeInfo {
	r.mu.Lock()
	n.LastSeen = time.Now()
	n.Status = "online"
	r.nodes[n.ID] = n
	r.mu.Unlock()
	r.save()
	slog.Info("registry node announced", "id", n.ID, "region", n.Region)
	return r.FindNearby(n.Region, n.ID, 5)
}

// Heartbeat keeps a node alive.
func (r *Registry) Heartbeat(id string, load float64) {
	r.mu.Lock()
	if n, ok := r.nodes[id]; ok {
		n.LastSeen = time.Now()
		n.LoadAvg = load
		n.Status = "online"
	}
	r.mu.Unlock()
}

// FindNearby returns the closest nodes by region, up to n.
func (r *Registry) FindNearby(region string, excludeID string, n int) []*NodeInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var candidates []*NodeInfo
	for _, node := range r.nodes {
		if node.ID == excludeID || node.Status != "online" {
			continue
		}
		if time.Since(node.LastSeen) > 30*time.Second {
			continue
		}
		candidates = append(candidates, node)
	}

	// Prefer same region, then sort by client load
	sort.Slice(candidates, func(i, j int) bool {
		ri := candidates[i].Region == region
		rj := candidates[j].Region == region
		if ri != rj {
			return ri
		}
		return candidates[i].ClientDist < candidates[j].ClientDist
	})

	if len(candidates) > n {
		candidates = candidates[:n]
	}
	return candidates
}

// All returns all known nodes.
func (r *Registry) All() []*NodeInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	nodes := make([]*NodeInfo, 0, len(r.nodes))
	for _, n := range r.nodes {
		nodes = append(nodes, n)
	}
	return nodes
}

// Stats returns registry health info.
func (r *Registry) Stats() map[string]any {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var online, offline int
	var regions []string
	seen := map[string]bool{}
	for _, n := range r.nodes {
		if n.Status == "online" {
			online++
		} else {
			offline++
		}
		if n.Region != "" && !seen[n.Region] {
			seen[n.Region] = true
			regions = append(regions, n.Region)
		}
	}
	return map[string]any{
		"total":   len(r.nodes),
		"online":  online,
		"offline": offline,
		"regions": regions,
	}
}

func (r *Registry) healthLoop() {
	ticker := time.NewTicker(15 * time.Second)
	for range ticker.C {
		r.mu.Lock()
		for id, n := range r.nodes {
			if time.Since(n.LastSeen) > 60*time.Second {
				n.Status = "offline"
				slog.Warn("registry node offline", "id", id, "last_seen", n.LastSeen)
			}
		}
		r.mu.Unlock()
	}
}

// --- HTTP handlers ---

func (r *Registry) AnnounceHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method != "POST" {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var n NodeInfo
	if err := json.NewDecoder(req.Body).Decode(&n); err != nil {
		http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
		return
	}
	if n.ID == "" || n.PublicKey == "" {
		http.Error(w, "id and public_key required", http.StatusBadRequest)
		return
	}
	nearby := r.Announce(&n)
	writeJSON(w, map[string]any{"ok": true, "nearby": nearby})
}


// DiscoverHandler handles the DiscoverHandler HTTP request.
func (r *Registry) DiscoverHandler(w http.ResponseWriter, req *http.Request) {
	region := req.URL.Query().Get("region")
	exclude := req.URL.Query().Get("exclude")
	limit := 5
	nearby := r.FindNearby(region, exclude, limit)
	writeJSON(w, map[string]any{"nodes": nearby, "count": len(nearby)})
}


// HeartbeatHandler handles the HeartbeatHandler HTTP request.
func (r *Registry) HeartbeatHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method != "POST" {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var hb struct {
		ID   string  `json:"id"`
		Load float64 `json:"load"`
	}
	if err := json.NewDecoder(req.Body).Decode(&hb); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}
	if hb.ID == "" {
		http.Error(w, "id required", http.StatusBadRequest)
		return
	}
	r.Heartbeat(hb.ID, hb.Load)
	writeJSON(w, map[string]any{"ok": true})
}


// StatusHandler handles the StatusHandler HTTP request.
func (r *Registry) StatusHandler(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, r.Stats())
}


// ListHandler handles the ListHandler HTTP request.
func (r *Registry) ListHandler(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{"nodes": r.All(), "count": len(r.All())})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// --- Helper for computing distance between two direct addresses ---

// PingLatency measures TCP handshake latency to addr.
func PingLatency(addr string) (time.Duration, error) {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return 0, err
	}
	conn.Close()
	return time.Since(start), nil
}

// SelectBestNodes picks the closest n nodes by measured latency.
func SelectBestNodes(candidates []*NodeInfo, n int) []*NodeInfo {
	type scored struct {
		node  *NodeInfo
		delay time.Duration
	}
	var scoredList []scored
	for _, c := range candidates {
		addr := c.DirectAddr
		if addr == "" {
			addr = strings.TrimPrefix(c.OnionAddr, "http://")
			addr = strings.TrimPrefix(addr, "https://")
		}
		d, err := PingLatency(addr)
		if err != nil {
			continue
		}
		scoredList = append(scoredList, scored{c, d})
	}
	sort.Slice(scoredList, func(i, j int) bool {
		return scoredList[i].delay < scoredList[j].delay
	})
	if len(scoredList) > n {
		scoredList = scoredList[:n]
	}
	out := make([]*NodeInfo, len(scoredList))
	for i, s := range scoredList {
		out[i] = s.node
	}
	return out
}

// GenerateNodeID creates a short unique node identifier.
func GenerateNodeID(pubkey string) string {
	h := fmt.Sprintf("%x", struct{ uint32 }{rand.Uint32()})
	return "node-" + h[:8]
}
