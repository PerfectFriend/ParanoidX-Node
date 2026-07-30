// Package api provides HTTP handlers and API endpoints for the simplex-node server
package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"ParanoidX/internal/economy"
	"ParanoidX/internal/fileutil"
	"ParanoidX/internal/middleware"
)

type AlertRule struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Metric     string  `json:"metric"`     // reserve_ng, supply_ng, coverage_pct, tier, oracle_price
	Operator   string  `json:"operator"`   // lt, gt, eq, changed
	Threshold  float64 `json:"threshold"`
	Severity   string  `json:"severity"`   // info, warning, critical
	Enabled    bool    `json:"enabled"`
	LastNotif  string  `json:"last_notified,omitempty"`
	CooldownM  int     `json:"cooldown_min"` // minutes between alerts
}

type AlertState struct {
	mu    sync.Mutex
	rules []AlertRule
	path  string
}


// LoadAlertRules handles the LoadAlertRules HTTP request.
func LoadAlertRules(dataDir string) *AlertState {
	a := &AlertState{path: filepath.Join(dataDir, "royal_alert_rules.json")}
	fileutil.ReadJSON(a.path, &a.rules)
	if a.rules == nil {
		a.rules = []AlertRule{}
	}
	return a
}


// Save handles the Save HTTP request.
func (a *AlertState) Save() {
	a.mu.Lock()
	defer a.mu.Unlock()
	fileutil.WriteJSON(a.path, a.rules)
}


// List handles the List HTTP request.
func (a *AlertState) List() []AlertRule {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]AlertRule, len(a.rules))
	copy(out, a.rules)
	return out
}


// Add handles the Add HTTP request.
func (a *AlertState) Add(rule AlertRule) {
	a.mu.Lock()
	defer a.mu.Unlock()
	rule.ID = fmt.Sprintf("alert-%d", time.Now().UnixNano())
	if rule.CooldownM <= 0 {
		rule.CooldownM = 60
	}
	rule.Enabled = true
	a.rules = append(a.rules, rule)
	fileutil.WriteJSON(a.path, a.rules)
}


// Update handles the Update HTTP request.
func (a *AlertState) Update(id string, rule AlertRule) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i, r := range a.rules {
		if r.ID == id {
			rule.ID = id
			a.rules[i] = rule
			fileutil.WriteJSON(a.path, a.rules)
			return true
		}
	}
	return false
}


// Delete handles the Delete HTTP request.
func (a *AlertState) Delete(id string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i, r := range a.rules {
		if r.ID == id {
			a.rules = append(a.rules[:i], a.rules[i+1:]...)
			fileutil.WriteJSON(a.path, a.rules)
			return true
		}
	}
	return false
}


// Evaluate handles the Evaluate HTTP request.
func (a *AlertState) Evaluate(dataDir string, oraclePrice float64) []string {
	a.mu.Lock()
	defer a.mu.Unlock()

	reserveNg := int64(0)
	if b, _ := os.ReadFile(filepath.Join(dataDir, "silver_reserve_ng.txt")); b != nil {
		fmt.Sscanf(string(b), "%d", &reserveNg)
	}
	ledger := economy.LoadLedger(dataDir)
	supplyNg := ledger.TotalSupply
	coveragePct := 0.0
	if supplyNg > 0 {
		coveragePct = float64(reserveNg) / float64(supplyNg) * 100
	}
	cfg := economy.DefaultTreasuryConfig()
	tier := economy.DetectTier(reserveNg, cfg)

	var triggered []string
	now := time.Now()

	for i, rule := range a.rules {
		if !rule.Enabled {
			continue
		}
		var current float64
		switch rule.Metric {
		case "reserve_ng":
			current = float64(reserveNg)
		case "supply_ng":
			current = float64(supplyNg)
		case "coverage_pct":
			current = coveragePct
		case "tier":
			current = float64(tier)
		case "oracle_price":
			current = oraclePrice
		default:
			continue
		}

		fired := false
		switch rule.Operator {
		case "lt":
			fired = current < rule.Threshold
		case "gt":
			fired = current > rule.Threshold
		case "eq":
			fired = current == rule.Threshold
		case "changed":
			continue
		}

		if fired {
			cooldown := time.Duration(rule.CooldownM) * time.Minute
			if rule.LastNotif != "" {
				last, err := time.Parse(time.RFC3339, rule.LastNotif)
				if err == nil && now.Sub(last) < cooldown {
					continue
				}
			}
			msg := fmt.Sprintf("ALERT %s: %s = %.2f (threshold %.2f)", rule.Severity, rule.Name, current, rule.Threshold)
			triggered = append(triggered, msg)
			a.rules[i].LastNotif = now.UTC().Format(time.RFC3339)
			slog.Warn("royal alert fired", "rule", rule.Name, "metric", rule.Metric, "current", current)
		}
	}

	if len(triggered) > 0 {
		fileutil.WriteJSON(a.path, a.rules)
	}
	return triggered
}

// ── Handlers ───────────────────────────────────────────────────────────────

var GlobalAlertState *AlertState


// RoyalAlertsListHandler handles the RoyalAlertsListHandler HTTP request.
func RoyalAlertsListHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) { return }
		if !isRoyalNodePath(dataDir) { http.Error(w, "forbidden", 403); return }
		writeJSON(w, map[string]any{"ok": true, "rules": GlobalAlertState.List()})
	}
}


// RoyalAlertsAddHandler handles the RoyalAlertsAddHandler HTTP request.
func RoyalAlertsAddHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) { return }
		if !isRoyalNodePath(dataDir) { http.Error(w, "forbidden", 403); return }
		if r.Method != "POST" { http.Error(w, "POST required", 405); return }

		var rule AlertRule
		if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
			return
		}
		if rule.Name == "" || rule.Metric == "" || rule.Operator == "" {
			writeJSON(w, map[string]any{"ok": false, "error": "name, metric, operator required"})
			return
		}
		GlobalAlertState.Add(rule)
		appendRoyalAudit(dataDir, "alert.add", map[string]any{"name": rule.Name, "metric": rule.Metric})
		writeJSON(w, map[string]any{"ok": true, "rule": rule})
	}
}


// RoyalAlertsDeleteHandler handles the RoyalAlertsDeleteHandler HTTP request.
func RoyalAlertsDeleteHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) { return }
		if !isRoyalNodePath(dataDir) { http.Error(w, "forbidden", 403); return }
		if r.Method != "POST" { http.Error(w, "POST required", 405); return }

		id := r.URL.Query().Get("id")
		if id == "" {
			writeJSON(w, map[string]any{"ok": false, "error": "id required"})
			return
		}
		if GlobalAlertState.Delete(id) {
			appendRoyalAudit(dataDir, "alert.delete", map[string]any{"id": id})
			writeJSON(w, map[string]any{"ok": true})
		} else {
			writeJSON(w, map[string]any{"ok": false, "error": "not found"})
		}
	}
}
