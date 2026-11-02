// Package registry provides internal service registry
package registry

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func newNodeInfo(id string) *NodeInfo {
	return &NodeInfo{
		ID:           id,
		Name:         "test-" + id,
		Region:       "us-east",
		OnionAddr:    "http://" + id + ".onion",
		DirectAddr:   "10.0.0.1:8080",
		Capabilities: []NodeCap{CapRelaySMP, CapRelayXFTP},
		FeeBps:       100,
		PublicKey:    "pk-" + id,
		Version:      "1.0",
		LastSeen:     time.Now(),
		Status:       "online",
		StakeNg:      1000,
	}
}


// TestNewRegistry handles the TestNewRegistry HTTP request.
func TestNewRegistry(t *testing.T) {
	t.Run("creates empty registry", func(t *testing.T) {
		dir := t.TempDir()
		r := NewRegistry(dir)

		all := r.All()
		if len(all) != 0 {
			t.Fatalf("expected 0 nodes, got %d", len(all))
		}
	})

	t.Run("loads existing data", func(t *testing.T) {
		dir := t.TempDir()

		r1 := NewRegistry(dir)
		n1 := newNodeInfo("node-a")
		r1.Announce(n1)
		time.Sleep(50 * time.Millisecond)

		r2 := NewRegistry(dir)
		all := r2.All()
		if len(all) != 1 {
			t.Fatalf("expected 1 node on reload, got %d", len(all))
		}
		if all[0].ID != "node-a" {
			t.Fatalf("expected node-a, got %s", all[0].ID)
		}
	})
}


// TestAnnounce handles the TestAnnounce HTTP request.
func TestAnnounce(t *testing.T) {
	dir := t.TempDir()
	r := NewRegistry(dir)

	n := newNodeInfo("node-1")
	nearby := r.Announce(n)

	all := r.All()
	if len(all) != 1 {
		t.Fatalf("expected 1 node, got %d", len(all))
	}
	if all[0].ID != "node-1" {
		t.Fatalf("expected node-1, got %s", all[0].ID)
	}
	if all[0].Status != "online" {
		t.Fatalf("expected status online, got %s", all[0].Status)
	}

	if len(nearby) != 0 {
		t.Fatalf("expected 0 nearby nodes, got %d", len(nearby))
	}
}


// TestDuplicatePeerHandling handles the TestDuplicatePeerHandling HTTP request.
func TestDuplicatePeerHandling(t *testing.T) {
	dir := t.TempDir()
	r := NewRegistry(dir)

	n1 := newNodeInfo("node-dup")
	n1.Region = "us-east"
	n1.FeeBps = 100

	r.Announce(n1)

	n2 := newNodeInfo("node-dup")
	n2.Region = "eu-west"
	n2.FeeBps = 200
	r.Announce(n2)

	all := r.All()
	if len(all) != 1 {
		t.Fatalf("expected 1 node after duplicate announce, got %d", len(all))
	}

	if all[0].FeeBps != 200 {
		t.Fatalf("expected FeeBps 200 (updated), got %d", all[0].FeeBps)
	}
	if all[0].Region != "eu-west" {
		t.Fatalf("expected Region eu-west (updated), got %s", all[0].Region)
	}
}


// TestFindNearby handles the TestFindNearby HTTP request.
func TestFindNearby(t *testing.T) {
	dir := t.TempDir()
	r := NewRegistry(dir)

	// Announce nodes (Announce overrides Status to "online" and LastSeen to time.Now())
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		r.Announce(&NodeInfo{
			ID: id, Region: func() string {
				switch id {
				case "a", "b", "e":
					return "us-east"
				default:
					return "eu-west"
				}
			}(), Status: "online", LastSeen: time.Now(),
		})
	}

	// Manually set node 'e' status to offline (Announce always sets online)
	r.mu.Lock()
	r.nodes["e"].Status = "offline"
	r.mu.Unlock()

	t.Run("prefers same region first", func(t *testing.T) {
		nearby := r.FindNearby("us-east", "a", 5)
		// Should have at least 1 node and the first must be us-east
		if len(nearby) == 0 {
			t.Fatal("expected at least 1 nearby node")
		}
		if nearby[0].Region != "us-east" {
			t.Fatalf("expected first result to be us-east, got %s", nearby[0].Region)
		}
	})

	t.Run("excludes self", func(t *testing.T) {
		nearby := r.FindNearby("us-east", "a", 5)
		for _, n := range nearby {
			if n.ID == "a" {
				t.Fatalf("should exclude id a")
			}
		}
	})

	t.Run("excludes offline nodes", func(t *testing.T) {
		nearby := r.FindNearby("us-east", "", 5)
		for _, n := range nearby {
			if n.ID == "e" {
				t.Fatalf("should exclude offline node e")
			}
		}
	})

	t.Run("returns limited results", func(t *testing.T) {
		nearby := r.FindNearby("us-east", "a", 1)
		if len(nearby) > 1 {
			t.Fatalf("expected at most 1 result, got %d", len(nearby))
		}
	})
}


// TestFindNearbyExcludesStale handles the TestFindNearbyExcludesStale HTTP request.
func TestFindNearbyExcludesStale(t *testing.T) {
	dir := t.TempDir()
	r := NewRegistry(dir)

	n := &NodeInfo{
		ID: "stale", Region: "us-east", Status: "online",
		LastSeen: time.Now(),
	}
	r.Announce(n)

	// Manually set LastSeen to 60s ago (Announce overrides to time.Now())
	r.mu.Lock()
	r.nodes["stale"].LastSeen = time.Now().Add(-60 * time.Second)
	r.mu.Unlock()

	nearby := r.FindNearby("us-east", "", 5)
	for _, m := range nearby {
		if m.ID == "stale" {
			t.Fatal("should exclude stale node")
		}
	}
}


// TestHeartbeat handles the TestHeartbeat HTTP request.
func TestHeartbeat(t *testing.T) {
	dir := t.TempDir()
	r := NewRegistry(dir)

	n := newNodeInfo("node-hb")
	r.Announce(n)

	// Manually set LastSeen to old so we can verify Heartbeat updates it
	r.mu.Lock()
	r.nodes["node-hb"].LastSeen = time.Now().Add(-5 * time.Minute)
	r.nodes["node-hb"].LoadAvg = 0.0
	r.mu.Unlock()

	r.Heartbeat("node-hb", 0.75)

	all := r.All()
	if len(all) != 1 {
		t.Fatalf("expected 1 node, got %d", len(all))
	}
	if all[0].LoadAvg != 0.75 {
		t.Fatalf("expected LoadAvg 0.75, got %f", all[0].LoadAvg)
	}
	if all[0].Status != "online" {
		t.Fatalf("expected status online, got %s", all[0].Status)
	}
	if time.Since(all[0].LastSeen) > 2*time.Second {
		t.Fatal("expected LastSeen to be updated to near now")
	}
}


// TestHeartbeatUnknownNode handles the TestHeartbeatUnknownNode HTTP request.
func TestHeartbeatUnknownNode(t *testing.T) {
	dir := t.TempDir()
	r := NewRegistry(dir)

	r.Heartbeat("nonexistent", 0.5)
	if len(r.All()) != 0 {
		t.Fatal("heartbeat should not create unknown nodes")
	}
}


// TestStats handles the TestStats HTTP request.
func TestStats(t *testing.T) {
	dir := t.TempDir()
	r := NewRegistry(dir)

	// Initially empty
	s := r.Stats()
	if s["total"].(int) != 0 {
		t.Fatalf("expected total 0, got %d", s["total"])
	}
	if s["online"].(int) != 0 {
		t.Fatalf("expected online 0, got %d", s["online"])
	}

	// Add nodes via direct map manipulation to control Status
	r.mu.Lock()
	r.nodes["a"] = &NodeInfo{ID: "a", Region: "us-east", Status: "online", LastSeen: time.Now()}
	r.nodes["b"] = &NodeInfo{ID: "b", Region: "us-east", Status: "online", LastSeen: time.Now()}
	r.nodes["c"] = &NodeInfo{ID: "c", Region: "eu-west", Status: "offline", LastSeen: time.Now()}
	r.mu.Unlock()

	s = r.Stats()
	if s["total"].(int) != 3 {
		t.Fatalf("expected total 3, got %d", s["total"])
	}
	if s["online"].(int) != 2 {
		t.Fatalf("expected online 2, got %d", s["online"])
	}
	if s["offline"].(int) != 1 {
		t.Fatalf("expected offline 1, got %d", s["offline"])
	}

	regions := s["regions"].([]string)
	if len(regions) != 2 {
		t.Fatalf("expected 2 regions, got %d", len(regions))
	}
}


// TestPersistence handles the TestPersistence HTTP request.
func TestPersistence(t *testing.T) {
	dir := t.TempDir()

	r1 := NewRegistry(dir)
	r1.Announce(newNodeInfo("p1"))
	r1.Announce(newNodeInfo("p2"))
	time.Sleep(50 * time.Millisecond)

	path := filepath.Join(dir, "registry_nodes.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("persistence file not created: %v", err)
	}

	r2 := NewRegistry(dir)

	all := r2.All()
	if len(all) != 2 {
		t.Fatalf("expected 2 persisted nodes, got %d", len(all))
	}

	ids := map[string]bool{}
	for _, n := range all {
		ids[n.ID] = true
	}
	if !ids["p1"] || !ids["p2"] {
		t.Fatal("persisted nodes have wrong IDs")
	}
}


// TestGenerateNodeID handles the TestGenerateNodeID HTTP request.
func TestGenerateNodeID(t *testing.T) {
	id1 := GenerateNodeID("pk1", nil)
	id2 := GenerateNodeID("pk2", nil)
	id3 := GenerateNodeID("pk1", nil) // same pubkey, should differ due to randomness

	// Check prefix
	if !strings.HasPrefix(id1, "node-") {
		t.Fatalf("expected node- prefix, got %s", id1)
	}

	// Check length: "node-" (5) + 32 hex chars (128 bits) = 37 total
	const expectedLen = 5 + 32
	if len(id1) != expectedLen {
		t.Fatalf("expected ID length %d (node- + 32 hex), got %d: %s", expectedLen, len(id1), id1)
	}

	// Check all chars after prefix are valid hex
	hexPart := id1[5:]
	for _, c := range hexPart {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Fatalf("ID contains non-hex chars: %s", id1)
		}
	}

	// Different pubkeys must produce different IDs (pubkey binding)
	if id1 == id2 {
		t.Fatal("expected different IDs for different pubkeys")
	}

	// Same pubkey must produce different IDs on each call (randomness)
	if id1 == id3 {
		t.Fatal("expected different IDs for same pubkey on repeated calls (randomness)")
	}

	// IDs for same pubkey should share no more than birthday-bound prefix
	// (they're XOR of pubkeyHash + random, so should differ in ~half bits)
	samePrefix := 0
	for i := 0; i < len(hexPart) && i < len(id3[5:]); i++ {
		if hexPart[i] == id3[5:][i] {
			samePrefix++
		} else {
			break
		}
	}
	if samePrefix > 4 { // >4 matching hex chars = >16 bits same prefix is very unlikely
		t.Logf("warning: same pubkey IDs share %d hex prefix chars (unlikely but possible)", samePrefix)
	}
}

// TestGenerateNodeIDFallback tests the fallback path when randomness source fails.
func TestGenerateNodeIDFallback(t *testing.T) {
	// Reader that always returns an error
	errReader := &errorReader{}

	id1 := GenerateNodeID("pk1", errReader)
	id2 := GenerateNodeID("pk1", errReader)

	// Should still produce valid node IDs with prefix and length
	if !strings.HasPrefix(id1, "node-") {
		t.Fatalf("expected node- prefix, got %s", id1)
	}
	const expectedLen = 5 + 32
	if len(id1) != expectedLen {
		t.Fatalf("expected ID length %d, got %d: %s", expectedLen, len(id1), id1)
	}

	// Fallback uses timestamp, so two calls in quick succession should produce
	// different IDs (nanosecond precision). If error check is removed (mutation),
	// randomBytes stays all zeros and both IDs would be identical.
	if id1 == id2 {
		t.Fatalf("fallback should produce different IDs on each call (timestamp entropy), got same: %s", id1)
	}

	// Verify hex chars
	hexPart := id1[5:]
	for _, c := range hexPart {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Fatalf("fallback ID contains non-hex chars: %s", id1)
		}
	}
}

// errorReader is an io.Reader that always returns an error.
type errorReader struct{}

func (e *errorReader) Read(p []byte) (n int, err error) {
	return 0, fmt.Errorf("intentional read error for testing")
}


// TestCapConstants handles the TestCapConstants HTTP request.
func TestCapConstants(t *testing.T) {
	if CapRelaySMP != "relay-smp" {
		t.Fatalf("unexpected CapRelaySMP: %s", CapRelaySMP)
	}
	if CapRelayXFTP != "relay-xftp" {
		t.Fatalf("unexpected CapRelayXFTP: %s", CapRelayXFTP)
	}
	if CapRadioSeed != "radio-seed" {
		t.Fatalf("unexpected CapRadioSeed: %s", CapRadioSeed)
	}
	if CapVaultPeer != "vault-peer" {
		t.Fatalf("unexpected CapVaultPeer: %s", CapVaultPeer)
	}
}

// --- HTTP handler tests ---

func TestAnnounceHandler(t *testing.T) {
	dir := t.TempDir()
	r := NewRegistry(dir)

	ts := httptest.NewServer(http.HandlerFunc(r.AnnounceHandler))
	defer ts.Close()

	t.Run("rejects GET", func(t *testing.T) {
		resp, err := http.Get(ts.URL)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", resp.StatusCode)
		}
	})

	t.Run("rejects missing id", func(t *testing.T) {
		body := `{"name":"test"}`
		resp, err := http.Post(ts.URL, "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("rejects missing public_key", func(t *testing.T) {
		body := `{"id":"n1"}`
		resp, err := http.Post(ts.URL, "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("registers node successfully", func(t *testing.T) {
		body := `{"id":"n1","name":"test-node","region":"us-east","public_key":"pk-n1"}`
		resp, err := http.Post(ts.URL, "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		var result map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}
		if result["ok"] != true {
			t.Fatal("expected ok: true")
		}
	})
}


// TestListHandler handles the TestListHandler HTTP request.
func TestListHandler(t *testing.T) {
	dir := t.TempDir()
	r := NewRegistry(dir)

	r.Announce(newNodeInfo("node-x"))
	r.Announce(newNodeInfo("node-y"))

	ts := httptest.NewServer(http.HandlerFunc(r.ListHandler))
	defer ts.Close()

	resp, err := http.Get(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}

	count := int(result["count"].(float64))
	if count != 2 {
		t.Fatalf("expected count 2, got %d", count)
	}
	nodes := result["nodes"].([]any)
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodes))
	}
}


// TestStatusHandler handles the TestStatusHandler HTTP request.
func TestStatusHandler(t *testing.T) {
	dir := t.TempDir()
	r := NewRegistry(dir)

	// Direct map manipulation to control Status
	r.mu.Lock()
	r.nodes["s1"] = &NodeInfo{ID: "s1", Region: "us-east", Status: "online", LastSeen: time.Now()}
	r.nodes["s2"] = &NodeInfo{ID: "s2", Region: "eu-west", Status: "online", LastSeen: time.Now()}
	r.nodes["s3"] = &NodeInfo{ID: "s3", Region: "us-east", Status: "offline", LastSeen: time.Now()}
	r.mu.Unlock()

	ts := httptest.NewServer(http.HandlerFunc(r.StatusHandler))
	defer ts.Close()

	resp, err := http.Get(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result["total"].(float64) != 3 {
		t.Fatalf("expected total 3, got %v", result["total"])
	}
	if result["online"].(float64) != 2 {
		t.Fatalf("expected online 2, got %v", result["online"])
	}
	if result["offline"].(float64) != 1 {
		t.Fatalf("expected offline 1, got %v", result["offline"])
	}
}


// TestHeartbeatHandler handles the TestHeartbeatHandler HTTP request.
func TestHeartbeatHandler(t *testing.T) {
	dir := t.TempDir()
	r := NewRegistry(dir)

	r.Announce(newNodeInfo("hb-node"))

	ts := httptest.NewServer(http.HandlerFunc(r.HeartbeatHandler))
	defer ts.Close()

	t.Run("rejects GET", func(t *testing.T) {
		resp, err := http.Get(ts.URL)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", resp.StatusCode)
		}
	})

	t.Run("rejects missing id", func(t *testing.T) {
		body := `{"load":0.5}`
		resp, err := http.Post(ts.URL, "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("heartbeats successfully", func(t *testing.T) {
		body := `{"id":"hb-node","load":0.85}`
		resp, err := http.Post(ts.URL, "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		var result map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}
		if result["ok"] != true {
			t.Fatal("expected ok: true")
		}

		all := r.All()
		if len(all) == 1 && all[0].LoadAvg != 0.85 {
			t.Fatalf("expected LoadAvg 0.85, got %f", all[0].LoadAvg)
		}
	})
}


// TestDiscoverHandler handles the TestDiscoverHandler HTTP request.
func TestDiscoverHandler(t *testing.T) {
	dir := t.TempDir()
	r := NewRegistry(dir)

	r.Announce(newNodeInfo("d1"))
	r.Announce(newNodeInfo("d2"))

	ts := httptest.NewServer(http.HandlerFunc(r.DiscoverHandler))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "?region=us-east&exclude=d1")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}

	nodes := result["nodes"].([]any)
	if len(nodes) == 0 {
		t.Fatal("expected at least 1 discovered node")
	}
}


// TestSelectBestNodesSkipsUnreachable handles the TestSelectBestNodesSkipsUnreachable HTTP request.
func TestSelectBestNodesSkipsUnreachable(t *testing.T) {
	candidates := []*NodeInfo{
		{ID: "a", DirectAddr: "127.0.0.1:1"},
		{ID: "b", DirectAddr: "127.0.0.1:2"},
	}
	result := SelectBestNodes(candidates, 5)
	if len(result) != 0 {
		t.Fatalf("expected 0 reachable nodes, got %d", len(result))
	}
}

// TestConcurrentAccess ensures the registry handles concurrent operations.
func TestConcurrentAccess(t *testing.T) {
	dir := t.TempDir()
	r := NewRegistry(dir)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("conc-%d", i)
			r.Announce(newNodeInfo(id))
		}(i)
	}
	wg.Wait()

	all := r.All()
	if len(all) != 10 {
		t.Fatalf("expected 10 nodes, got %d", len(all))
	}
}

// TestDiscoverNoMatch tests discover when no nodes match.
func TestDiscoverNoMatch(t *testing.T) {
	dir := t.TempDir()
	r := NewRegistry(dir)

	ts := httptest.NewServer(http.HandlerFunc(r.DiscoverHandler))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "?region=antarctica")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}

	count := int(result["count"].(float64))
	if count != 0 {
		t.Fatalf("expected 0 nodes, got %d", count)
	}
}

// TestAnnounceViaAnnounceThenList verifies the full announce flow via HTTP.
func TestAnnounceViaAnnounceThenList(t *testing.T) {
	dir := t.TempDir()
	r := NewRegistry(dir)

	mux := http.NewServeMux()
	mux.HandleFunc("/announce", r.AnnounceHandler)
	mux.HandleFunc("/list", r.ListHandler)

	ts := httptest.NewServer(mux)
	defer ts.Close()

	body := `{"id":"http-node","name":"http-test","region":"eu-west","public_key":"pk-http"}`
	resp, err := http.Post(ts.URL+"/announce", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	resp, err = http.Get(ts.URL + "/list")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}

	count := int(result["count"].(float64))
	if count != 1 {
		t.Fatalf("expected 1 node after HTTP announce, got %d", count)
	}
}

// TestHeartbeatUpdatesLastSeen verifies Heartbeat prevents node from going stale.
func TestHeartbeatUpdatesLastSeen(t *testing.T) {
	dir := t.TempDir()
	r := NewRegistry(dir)

	n := newNodeInfo("hb-seen")
	r.Announce(n)

	// Set LastSeen to old, heartbeat should refresh it
	r.mu.Lock()
	r.nodes["hb-seen"].LastSeen = time.Now().Add(-10 * time.Minute)
	r.mu.Unlock()

	r.Heartbeat("hb-seen", 0.5)

	all := r.All()
	if len(all) != 1 {
		t.Fatalf("expected 1 node, got %d", len(all))
	}
	if time.Since(all[0].LastSeen) > 2*time.Second {
		t.Fatal("Heartbeat should update LastSeen to recent")
	}
}

// TestSaveAndLoadWithEmptyDir tests that load handles missing file gracefully.
func TestSaveAndLoadWithEmptyDir(t *testing.T) {
	// This test verifies load() doesn't panic when no file exists
	dir := t.TempDir()
	r := NewRegistry(dir)

	if r.All() == nil {
		t.Fatal("All() should return empty slice, not nil")
	}
}
