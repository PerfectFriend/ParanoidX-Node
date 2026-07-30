// Package api provides HTTP handlers and API endpoints for the simplex-node server
package api

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"ParanoidX/internal/economy"
	"ParanoidX/internal/fileutil"
	"ParanoidX/internal/middleware"
)

// ── C24: Multi-sig ─────────────────────────────────────────────────────────

func RoyalMultiSigHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) { return }
		if !isRoyalNodePath(dataDir) { http.Error(w, "forbidden", 403); return }
		signersFile := filepath.Join(dataDir, "royal_allowed_signers.txt")
		switch r.Method {
		case "GET":
			b, _ := os.ReadFile(signersFile)
			var signers []string
			if b != nil {
				for _, s := range strings.Split(strings.TrimSpace(string(b)), "\n") {
					if s = strings.TrimSpace(s); s != "" {
						signers = append(signers, s)
					}
				}
			}
			writeJSON(w, map[string]any{"ok": true, "signers": signers, "count": len(signers)})
		case "POST":
			if checkEmergencyStop(w, dataDir) { return }
			var req struct {
				Action string `json:"action"`
				Pubkey string `json:"pubkey"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, map[string]any{"ok": false, "error": "bad json"}); return
			}
			switch req.Action {
			case "add":
				f, _ := os.OpenFile(signersFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
				if f != nil { fmt.Fprintf(f, "%s\n", req.Pubkey); f.Close() }
				appendRoyalAudit(dataDir, "multisig.add", nil)
				writeJSON(w, map[string]any{"ok": true})
			case "remove":
				b, _ := os.ReadFile(signersFile)
				if b != nil {
					var lines []string
					for _, s := range strings.Split(string(b), "\n") {
						if strings.TrimSpace(s) != req.Pubkey && s != "" {
							lines = append(lines, s)
						}
					}
					os.WriteFile(signersFile, []byte(strings.Join(lines, "\n")+"\n"), 0600)
				}
				appendRoyalAudit(dataDir, "multisig.remove", nil)
				writeJSON(w, map[string]any{"ok": true})
			default:
				writeJSON(w, map[string]any{"ok": false, "error": "action must be add/remove"})
			}
		default:
			http.Error(w, "GET/POST only", 405)
		}
	}
}

// ── C25: Auto-Cron ─────────────────────────────────────────────────────────

var GlobalCronScheduler *CronScheduler

type CronRule struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Action   string         `json:"action"`
	Interval string         `json:"interval"`
	Params   map[string]any `json:"params,omitempty"`
	Enabled  bool           `json:"enabled"`
	LastRun  string         `json:"last_run,omitempty"`
}

type CronScheduler struct {
	mu    sync.Mutex
	rules []CronRule
	path  string
}


// LoadCronRules handles the LoadCronRules HTTP request.
func LoadCronRules(dataDir string) *CronScheduler {
	c := &CronScheduler{path: filepath.Join(dataDir, "royal_cron_rules.json")}
	fileutil.ReadJSON(c.path, &c.rules)
	if c.rules == nil { c.rules = []CronRule{} }
	return c
}


// List handles the List HTTP request.
func (c *CronScheduler) List() []CronRule {
	c.mu.Lock(); defer c.mu.Unlock()
	out := make([]CronRule, len(c.rules)); copy(out, c.rules); return out
}


// Add handles the Add HTTP request.
func (c *CronScheduler) Add(rule CronRule) {
	c.mu.Lock(); defer c.mu.Unlock()
	rule.ID = fmt.Sprintf("cron-%d", time.Now().UnixNano())
	rule.Enabled = true
	c.rules = append(c.rules, rule)
	fileutil.WriteJSON(c.path, c.rules)
}


// Delete handles the Delete HTTP request.
func (c *CronScheduler) Delete(id string) bool {
	c.mu.Lock(); defer c.mu.Unlock()
	for i, r := range c.rules {
		if r.ID == id {
			c.rules = append(c.rules[:i], c.rules[i+1:]...)
			fileutil.WriteJSON(c.path, c.rules)
			return true
		}
	}
	return false
}


// Tick handles the Tick HTTP request.
func (c *CronScheduler) Tick(dataDir string) []string {
	c.mu.Lock(); defer c.mu.Unlock()
	var executed []string
	now := time.Now().UTC()
	for i, rule := range c.rules {
		if !rule.Enabled { continue }
		var shouldRun bool
		lastHrs := 9999.0
		if rule.LastRun != "" {
			if t, err := time.Parse(time.RFC3339, rule.LastRun); err == nil {
				lastHrs = time.Since(t).Hours()
			}
		}
		switch rule.Interval {
		case "hourly": shouldRun = lastHrs >= 1
		case "daily": shouldRun = lastHrs >= 24
		case "weekly": shouldRun = lastHrs >= 168
		case "monthly": shouldRun = lastHrs >= 720
		}
		if shouldRun {
			c.rules[i].LastRun = now.Format(time.RFC3339)
			executed = append(executed, rule.Name)
			slog.Info("cron executed", "rule", rule.Name, "action", rule.Action)
		}
	}
	if len(executed) > 0 { fileutil.WriteJSON(c.path, c.rules) }
	return executed
}


// RoyalCronListHandler handles the RoyalCronListHandler HTTP request.
func RoyalCronListHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) { return }
		if !isRoyalNodePath(dataDir) { http.Error(w, "forbidden", 403); return }
		writeJSON(w, map[string]any{"ok": true, "rules": GlobalCronScheduler.List()})
	}
}


// RoyalCronAddHandler handles the RoyalCronAddHandler HTTP request.
func RoyalCronAddHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) { return }
		if !isRoyalNodePath(dataDir) { http.Error(w, "forbidden", 403); return }
		if r.Method != "POST" { http.Error(w, "POST required", 405); return }
		if checkEmergencyStop(w, dataDir) { return }
		var rule CronRule
		if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "bad json"}); return
		}
		if rule.Name == "" || rule.Action == "" || rule.Interval == "" {
			writeJSON(w, map[string]any{"ok": false, "error": "name, action, interval required"}); return
		}
		GlobalCronScheduler.Add(rule)
		appendRoyalAudit(dataDir, "cron.add", map[string]any{"name": rule.Name})
		writeJSON(w, map[string]any{"ok": true, "rule": rule})
	}
}


// RoyalCronDeleteHandler handles the RoyalCronDeleteHandler HTTP request.
func RoyalCronDeleteHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) { return }
		if !isRoyalNodePath(dataDir) { http.Error(w, "forbidden", 403); return }
		if r.Method != "POST" { http.Error(w, "POST required", 405); return }
		id := r.URL.Query().Get("id")
		if id == "" { writeJSON(w, map[string]any{"ok": false, "error": "id required"}); return }
		if GlobalCronScheduler.Delete(id) {
			appendRoyalAudit(dataDir, "cron.delete", nil)
			writeJSON(w, map[string]any{"ok": true})
		} else {
			writeJSON(w, map[string]any{"ok": false, "error": "not found"})
		}
	}
}

// ── C26: Inter-Node Sync ────────────────────────────────────────────────────

func RoyalSyncHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) { return }
		if !isRoyalNodePath(dataDir) { http.Error(w, "forbidden", 403); return }
		switch r.Method {
		case "GET":
			reserveNg := int64(0)
			if b, _ := os.ReadFile(filepath.Join(dataDir, "silver_reserve_ng.txt")); b != nil {
				fmt.Sscanf(string(b), "%d", &reserveNg)
			}
			ledger := economy.LoadLedger(dataDir)
			writeJSON(w, map[string]any{"ok": true, "reserve_ng": reserveNg, "supply_ng": ledger.TotalSupply, "node": "royal", "timestamp": time.Now().UTC().Format(time.RFC3339)})
		case "POST":
			var req struct { ReserveNg int64 `json:"reserve_ng"`; SupplyNg int64 `json:"supply_ng"`; Node string `json:"node"` }
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, map[string]any{"ok": false, "error": "bad json"}); return
			}
			appendRoyalAudit(dataDir, "sync.received", map[string]any{"from": req.Node})
			slog.Info("royal sync", "from", req.Node, "reserve", req.ReserveNg)
			writeJSON(w, map[string]any{"ok": true})
		default:
			http.Error(w, "GET/POST only", 405)
		}
	}
}

// ── C28: Scheduled Actions ──────────────────────────────────────────────────

type ScheduledAction struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Action    string         `json:"action"`
	Params    map[string]any `json:"params,omitempty"`
	RunAt     string         `json:"run_at"`
	Executed  bool           `json:"executed"`
	CreatedAt string         `json:"created_at"`
}

var (
	schedActions   []ScheduledAction
	schedActionsMu sync.Mutex
)


// RoyalScheduleListHandler handles the RoyalScheduleListHandler HTTP request.
func RoyalScheduleListHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) { return }
		if !isRoyalNodePath(dataDir) { http.Error(w, "forbidden", 403); return }
		schedActionsMu.Lock()
		out := make([]ScheduledAction, len(schedActions)); copy(out, schedActions)
		schedActionsMu.Unlock()
		writeJSON(w, map[string]any{"ok": true, "scheduled": out})
	}
}


// RoyalScheduleCreateHandler handles the RoyalScheduleCreateHandler HTTP request.
func RoyalScheduleCreateHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) { return }
		if !isRoyalNodePath(dataDir) { http.Error(w, "forbidden", 403); return }
		if r.Method != "POST" { http.Error(w, "POST required", 405); return }
		if checkEmergencyStop(w, dataDir) { return }
		var req struct {
			Name string `json:"name"`; Action string `json:"action"`; RunAt string `json:"run_at"`
			Params map[string]any `json:"params,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "bad json"}); return
		}
		sa := ScheduledAction{
			ID: fmt.Sprintf("sched-%d", time.Now().UnixNano()), Name: req.Name, Action: req.Action,
			Params: req.Params, RunAt: req.RunAt, CreatedAt: time.Now().UTC().Format(time.RFC3339),
		}
		schedActionsMu.Lock()
		schedActions = append(schedActions, sa)
		schedActionsMu.Unlock()
		appendRoyalAudit(dataDir, "schedule.create", map[string]any{"name": req.Name})
		writeJSON(w, map[string]any{"ok": true, "scheduled": sa})
	}
}

// ── C29: Audit Export ───────────────────────────────────────────────────────

func RoyalAuditExportHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) { return }
		if !isRoyalNodePath(dataDir) { http.Error(w, "forbidden", 403); return }
		format := r.URL.Query().Get("format")
		if format == "" { format = "json" }
		auditFile := filepath.Join(dataDir, "royal_audit.jsonl")
		b, err := os.ReadFile(auditFile)
		if err != nil { writeJSON(w, map[string]any{"ok": true, "entries": []any{}}); return }
		lines := strings.Split(strings.TrimSpace(string(b)), "\n")
		var entries []map[string]any
		for _, line := range lines {
			var entry map[string]any
			if json.Unmarshal([]byte(line), &entry) == nil { entries = append(entries, entry) }
		}
		if format == "csv" {
			w.Header().Set("Content-Type", "text/csv; charset=utf-8")
			w.Header().Set("Content-Disposition", "attachment; filename=royal_audit.csv")
			writer := csv.NewWriter(w)
			writer.Write([]string{"timestamp", "action", "details"})
			for _, e := range entries {
				ts, _ := e["timestamp"].(string)
				act, _ := e["action"].(string)
				det, _ := json.Marshal(e["details"])
				writer.Write([]string{ts, act, string(det)})
			}
			writer.Flush()
		} else {
			writeJSON(w, map[string]any{"ok": true, "entries": entries, "count": len(entries)})
		}
	}
}

// ── C30: Node Groups ────────────────────────────────────────────────────────

type NodeGroup struct {
	Name  string   `json:"name"`
	Nodes []string `json:"nodes"`
}

var (
	nodeGroups   []NodeGroup
	nodeGroupsMu sync.Mutex
)


// RoyalNodeGroupsHandler handles the RoyalNodeGroupsHandler HTTP request.
func RoyalNodeGroupsHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) { return }
		if !isRoyalNodePath(dataDir) { http.Error(w, "forbidden", 403); return }
		switch r.Method {
		case "GET":
			nodeGroupsMu.Lock()
			out := make([]NodeGroup, len(nodeGroups)); copy(out, nodeGroups)
			nodeGroupsMu.Unlock()
			writeJSON(w, map[string]any{"ok": true, "groups": out})
		case "POST":
			if checkEmergencyStop(w, dataDir) { return }
			var req struct { Action string `json:"action"`; Name string `json:"name"`; Nodes []string `json:"nodes"` }
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, map[string]any{"ok": false, "error": "bad json"}); return
			}
			switch req.Action {
			case "create":
				ng := NodeGroup{Name: req.Name, Nodes: req.Nodes}
				nodeGroupsMu.Lock()
				nodeGroups = append(nodeGroups, ng)
				nodeGroupsMu.Unlock()
				writeJSON(w, map[string]any{"ok": true, "group": ng})
			case "delete":
				nodeGroupsMu.Lock()
				for i, g := range nodeGroups {
					if g.Name == req.Name { nodeGroups = append(nodeGroups[:i], nodeGroups[i+1:]...); break }
				}
				nodeGroupsMu.Unlock()
				writeJSON(w, map[string]any{"ok": true})
			default:
				writeJSON(w, map[string]any{"ok": false, "error": "action must be create/delete"})
			}
		default:
			http.Error(w, "GET/POST only", 405)
		}
	}
}

// ── C31: Emergency Stop ─────────────────────────────────────────────────────

func RoyalEmergencyStopHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) { return }
		if !isRoyalNodePath(dataDir) { http.Error(w, "forbidden", 403); return }
		stopFile := filepath.Join(dataDir, "royal.emergency_stop")
		switch r.Method {
		case "GET":
			_, active := os.Stat(stopFile)
			writeJSON(w, map[string]any{"ok": true, "emergency_stop": active == nil})
		case "POST":
			if checkEmergencyStop(w, dataDir) { return }
			var req struct { Action string `json:"action"` }
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, map[string]any{"ok": false, "error": "bad json"}); return
			}
			switch req.Action {
			case "enable":
				os.WriteFile(stopFile, []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0600)
				slog.Warn("ROYAL EMERGENCY STOP ENABLED")
			case "disable":
				os.Remove(stopFile)
				slog.Warn("ROYAL EMERGENCY STOP DISABLED")
			default:
				writeJSON(w, map[string]any{"ok": false, "error": "action must be enable/disable"}); return
			}
			appendRoyalAudit(dataDir, "emergency."+req.Action, nil)
			writeJSON(w, map[string]any{"ok": true})
		default:
			http.Error(w, "GET/POST only", 405)
		}
	}
}

// ── C32: Node Reputation ────────────────────────────────────────────────────

type NodeRep struct {
	Pubkey      string  `json:"pubkey"`
	UptimePct   float64 `json:"uptime_pct"`
	AvgLatMs    float64 `json:"avg_latency_ms"`
	Score       float64 `json:"score"`
	LastSeen    string  `json:"last_seen"`
}

var (
	nodeReps   []NodeRep
	nodeRepsMu sync.Mutex
)


// RoyalReputationHandler handles the RoyalReputationHandler HTTP request.
func RoyalReputationHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) { return }
		if !isRoyalNodePath(dataDir) { http.Error(w, "forbidden", 403); return }
		nodeRepsMu.Lock()
		out := make([]NodeRep, len(nodeReps)); copy(out, nodeReps)
		sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
		nodeRepsMu.Unlock()
		writeJSON(w, map[string]any{"ok": true, "reputations": out})
	}
}


// RoyalHeartbeatHandler handles the RoyalHeartbeatHandler HTTP request.
func RoyalHeartbeatHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) { return }
		if !isRoyalNodePath(dataDir) { http.Error(w, "forbidden", 403); return }
		pubkey := r.URL.Query().Get("pubkey")
		if pubkey == "" { writeJSON(w, map[string]any{"ok": false, "error": "pubkey required"}); return }
		latStr := r.URL.Query().Get("latency_ms")
		lat := float64(0)
		if latStr != "" { fmt.Sscanf(latStr, "%f", &lat) }
		nodeRepsMu.Lock()
		found := false
		for i, nr := range nodeReps {
			if nr.Pubkey == pubkey {
				nodeReps[i].AvgLatMs = nr.AvgLatMs*0.7 + lat*0.3
				nodeReps[i].UptimePct = nr.UptimePct*0.95 + 5.0
				nodeReps[i].Score = nodeReps[i].UptimePct * (1.0 - nodeReps[i].AvgLatMs/1000.0)
				nodeReps[i].LastSeen = time.Now().UTC().Format(time.RFC3339)
				found = true; break
			}
		}
		if !found {
			nodeReps = append(nodeReps, NodeRep{
				Pubkey: pubkey, UptimePct: 100, AvgLatMs: lat, Score: 100,
				LastSeen: time.Now().UTC().Format(time.RFC3339),
			})
		}
		nodeRepsMu.Unlock()
		writeJSON(w, map[string]any{"ok": true})
	}
}

// ── C33: Treasury Analytics ─────────────────────────────────────────────────

type RoyalSnap struct {
	Timestamp   string  `json:"timestamp"`
	ReserveNg   int64   `json:"reserve_ng"`
	SupplyNg    int64   `json:"supply_ng"`
	OraclePrice float64 `json:"oracle_price"`
	Tier        int     `json:"tier"`
}

var (
	royalSnaps   []RoyalSnap
	royalSnapsMu sync.Mutex
)


// RecordRoyalSnap handles the RecordRoyalSnap HTTP request.
func RecordRoyalSnap(dataDir string) {
	op := economy.DefaultSilverSpotUSDperOZ
	if GlobalOracleRef != nil { op = GlobalOracleRef.GetPrice() }
	rn := int64(0)
	if b, _ := os.ReadFile(filepath.Join(dataDir, "silver_reserve_ng.txt")); b != nil {
		fmt.Sscanf(string(b), "%d", &rn)
	}
	l := economy.LoadLedger(dataDir)
	cfg := economy.DefaultTreasuryConfig()
	tier := economy.DetectTier(rn, cfg)
	snap := RoyalSnap{Timestamp: time.Now().UTC().Format(time.RFC3339), ReserveNg: rn, SupplyNg: l.TotalSupply, OraclePrice: op, Tier: int(tier)}
	royalSnapsMu.Lock()
	royalSnaps = append(royalSnaps, snap)
	if len(royalSnaps) > 1000 { royalSnaps = royalSnaps[len(royalSnaps)-1000:] }
	royalSnapsMu.Unlock()
}


// RoyalAnalyticsHandler handles the RoyalAnalyticsHandler HTTP request.
func RoyalAnalyticsHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) { return }
		if !isRoyalNodePath(dataDir) { http.Error(w, "forbidden", 403); return }
		days := 30
		if d := r.URL.Query().Get("days"); d != "" { fmt.Sscanf(d, "%d", &days) }
		cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
		royalSnapsMu.Lock()
		var filtered []RoyalSnap
		for _, s := range royalSnaps {
			t, err := time.Parse(time.RFC3339, s.Timestamp)
			if err == nil && t.After(cutoff) { filtered = append(filtered, s) }
		}
		royalSnapsMu.Unlock()
		trends := map[string]any{"days": days, "data_points": len(filtered), "history": filtered}
		if len(filtered) >= 2 {
			first, last := filtered[0], filtered[len(filtered)-1]
			trends["reserve_change"] = last.ReserveNg - first.ReserveNg
			trends["supply_change"] = last.SupplyNg - first.SupplyNg
			trends["price_change"] = last.OraclePrice - first.OraclePrice
		}
		writeJSON(w, map[string]any{"ok": true, "trends": trends})
	}
}

// ── C34: Multi-Currency Reserves ────────────────────────────────────────────

type CryptoRes struct {
	Currency string  `json:"currency"`
	Balance  float64 `json:"balance"`
	USDValue float64 `json:"usd_value"`
	Updated  string  `json:"updated"`
}


// RoyalCryptoHandler handles the RoyalCryptoHandler HTTP request.
func RoyalCryptoHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) { return }
		if !isRoyalNodePath(dataDir) { http.Error(w, "forbidden", 403); return }
		fp := filepath.Join(dataDir, "royal_crypto.json")
		switch r.Method {
		case "GET":
			var cr []CryptoRes
			fileutil.ReadJSON(fp, &cr)
			if cr == nil { cr = []CryptoRes{} }
			writeJSON(w, map[string]any{"ok": true, "reserves": cr})
		case "POST":
			if checkEmergencyStop(w, dataDir) { return }
			var req CryptoRes
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, map[string]any{"ok": false, "error": "bad json"}); return
			}
			if req.Currency == "" { writeJSON(w, map[string]any{"ok": false, "error": "currency required"}); return }
			req.Updated = time.Now().UTC().Format(time.RFC3339)
			var cr []CryptoRes
			fileutil.ReadJSON(fp, &cr)
			if cr == nil { cr = []CryptoRes{} }
			found := false
			for i, c := range cr {
				if c.Currency == req.Currency { cr[i] = req; found = true; break }
			}
			if !found { cr = append(cr, req) }
			fileutil.WriteJSON(fp, cr)
			appendRoyalAudit(dataDir, "crypto.update", map[string]any{"currency": req.Currency})
			writeJSON(w, map[string]any{"ok": true})
		default:
			http.Error(w, "GET/POST only", 405)
		}
	}
}

// ── C38: Rate Limit Stats ───────────────────────────────────────────────────

func RoyalRateLimitHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) { return }
		if !isRoyalNodePath(dataDir) { http.Error(w, "forbidden", 403); return }
		writeJSON(w, map[string]any{
			"ok": true, "endpoints": 30, "max_rpm": 100,
			"policy": "sliding window per IP",
		})
	}
}

// ── C40: Health Check ───────────────────────────────────────────────────────

func RoyalPingHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"ok": true, "pong": true, "royal": isRoyalNodePath(dataDir)})
	}
}
