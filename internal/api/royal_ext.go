// Package api provides HTTP handlers and API endpoints for the simplex-node server
package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"ParanoidX/internal/dc"
	"ParanoidX/internal/economy"
	"ParanoidX/internal/fileutil"
	"ParanoidX/internal/middleware"
)

var GlobalDCCloud *dc.Cloud

var (
	cachedTargets   map[string]CachedNodeInfo
	cachedTargetsMu sync.RWMutex
)

type CachedNodeInfo struct {
	Addr   string `json:"addr"`
	Pubkey string `json:"pubkey"`
	Alias  string `json:"alias"`
}

func lazyInitCachedTargets(dataDir string) {
	cachedTargetsMu.Lock()
	defer cachedTargetsMu.Unlock()
	if cachedTargets != nil {
		return
	}
	cachedTargets = map[string]CachedNodeInfo{}
	nodesFile := filepath.Join(dataDir, "tracker_nodes.json")
	if b, err := os.ReadFile(nodesFile); err == nil {
		var nodes []map[string]any
		if json.Unmarshal(b, &nodes) == nil {
			for _, n := range nodes {
				if id, ok := n["id"].(string); ok {
					cachedTargets[id] = CachedNodeInfo{
						Addr:   toString(n["addr"]),
						Pubkey: toString(n["pubkey"]),
						Alias:  toString(n["alias"]),
					}
				}
			}
		}
	}
}

func toString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// ── Royal DC Cloud ──────────────────────────────────────────────────────────

func RoyalDCStatusHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) { return }
		if !isRoyalNodePath(dataDir) { http.Error(w, "forbidden", 403); return }

		if GlobalDCCloud == nil {
			writeJSON(w, map[string]any{"ok": false, "error": "dc cloud not initialized"})
			return
		}
		containers := GlobalDCCloud.ListContainers()
		seeding := GlobalDCCloud.SeedingStatus()
		totalMB := int64(0)
		for _, c := range containers {
			totalMB += c.Size / (1024 * 1024)
		}
		writeJSON(w, map[string]any{
			"ok":             true,
			"containers":     len(containers),
			"seeding":        len(seeding),
			"total_size_mb":  totalMB,
			"container_list": containers,
			"seeding_list":   seeding,
		})
	}
}


// RoyalDCSwarmHandler handles the RoyalDCSwarmHandler HTTP request.
func RoyalDCSwarmHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) { return }
		if !isRoyalNodePath(dataDir) { http.Error(w, "forbidden", 403); return }
		if GlobalDCCloud == nil { http.Error(w, "dc cloud not initialized", 500); return }

		infohash := r.URL.Query().Get("infohash")
		containers := GlobalDCCloud.ListContainers()
		if infohash != "" {
			for _, c := range containers {
				if c.Infohash == infohash || strings.HasPrefix(c.Infohash, infohash) {
					writeJSON(w, map[string]any{"ok": true, "container": c})
					return
				}
			}
			writeJSON(w, map[string]any{"ok": false, "error": "not found"})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "containers": containers, "count": len(containers)})
	}
}


// RoyalDCSeedHandler handles the RoyalDCSeedHandler HTTP request.
func RoyalDCSeedHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) { return }
		if !isRoyalNodePath(dataDir) { http.Error(w, "forbidden", 403); return }
		if r.Method != "POST" { http.Error(w, "POST required", 405); return }
		if GlobalDCCloud == nil { http.Error(w, "dc cloud not initialized", 500); return }

		var req struct {
			ContainerID string `json:"container_id"`
			Path        string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
			return
		}
		if req.ContainerID == "" {
			writeJSON(w, map[string]any{"ok": false, "error": "container_id required"})
			return
		}
		containerPath := req.Path
		if containerPath == "" {
			containerPath = filepath.Join(dataDir, "containers", req.ContainerID)
		}
		manifest, err := GlobalDCCloud.SeedContainer(containerPath, req.ContainerID)
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		appendRoyalAudit(dataDir, "dc.seed", map[string]any{
			"container_id": req.ContainerID, "infohash": manifest.Infohash,
		})
		writeJSON(w, map[string]any{"ok": true, "manifest": manifest})
	}
}


// RoyalDCUnseedHandler handles the RoyalDCUnseedHandler HTTP request.
func RoyalDCUnseedHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) { return }
		if !isRoyalNodePath(dataDir) { http.Error(w, "forbidden", 403); return }
		if r.Method != "POST" { http.Error(w, "POST required", 405); return }
		if GlobalDCCloud == nil { http.Error(w, "dc cloud not initialized", 500); return }

		var req struct {
			Infohash string `json:"infohash"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
			return
		}
		if req.Infohash == "" {
			writeJSON(w, map[string]any{"ok": false, "error": "infohash required"})
			return
		}
		GlobalDCCloud.StopSeeding(req.Infohash)
		appendRoyalAudit(dataDir, "dc.unseed", map[string]any{"infohash": req.Infohash})
		writeJSON(w, map[string]any{"ok": true, "infohash": req.Infohash})
	}
}


// RoyalDCHealHandler handles the RoyalDCHealHandler HTTP request.
func RoyalDCHealHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) { return }
		if !isRoyalNodePath(dataDir) { http.Error(w, "forbidden", 403); return }
		if r.Method != "POST" { http.Error(w, "POST required", 405); return }
		if GlobalDCCloud == nil { http.Error(w, "dc cloud not initialized", 500); return }

		GlobalDCCloud.ForceHeal()
		appendRoyalAudit(dataDir, "dc.heal", map[string]any{"triggered": true})
		writeJSON(w, map[string]any{"ok": true, "note": "healing loop triggered"})
	}
}

// ── Royal Governance ────────────────────────────────────────────────────────

func RoyalConstitutionHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) { return }
		if !isRoyalNodePath(dataDir) { http.Error(w, "forbidden", 403); return }

		constitutionFile := filepath.Join(dataDir, "royal_constitution.json")

		switch r.Method {
		case "GET":
			var constitution map[string]any
			if b, _ := os.ReadFile(constitutionFile); b != nil {
				json.Unmarshal(b, &constitution)
			}
			if constitution == nil {
				constitution = map[string]any{
					"version": "1.0",
					"title":   "Constitution of Saint Mary Liberty Island",
					"articles": []map[string]any{
						{"number": 1, "title": "Sovereignty", "content": "The Island is a sovereign digital jurisdiction."},
						{"number": 2, "title": "Rights", "content": "Every resident has the right to privacy, free speech, and property."},
						{"number": 3, "title": "Governance", "content": "Governance is by DAO proposals and Steward AI oversight."},
					},
				}
			}
			writeJSON(w, map[string]any{"ok": true, "constitution": constitution})

		case "POST":
			var req struct {
				ArticleNumber int    `json:"article_number"`
				Title         string `json:"title"`
				Content       string `json:"content"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
				return
			}
			var constitution map[string]any
			if b, _ := os.ReadFile(constitutionFile); b != nil {
				json.Unmarshal(b, &constitution)
			}
			if constitution == nil {
				constitution = map[string]any{"version": "1.0", "title": "Constitution of Saint Mary Liberty Island", "articles": []map[string]any{}}
			}
			articles, _ := constitution["articles"].([]any)
			article := map[string]any{"number": req.ArticleNumber, "title": req.Title, "content": req.Content}
			articles = append(articles, article)
			constitution["articles"] = articles
			fileutil.WriteJSON(constitutionFile, constitution)
			appendRoyalAudit(dataDir, "governance.amend", map[string]any{"article": req.ArticleNumber})
			writeJSON(w, map[string]any{"ok": true, "article": article})

		default:
			http.Error(w, "GET/POST only", 405)
		}
	}
}


// RoyalGovernanceProposalsHandler handles the RoyalGovernanceProposalsHandler HTTP request.
func RoyalGovernanceProposalsHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) { return }
		if !isRoyalNodePath(dataDir) { http.Error(w, "forbidden", 403); return }

		proposalFile := filepath.Join(dataDir, "royal_proposals.json")

		switch r.Method {
		case "GET":
			var proposals []map[string]any
			if b, _ := os.ReadFile(proposalFile); b != nil {
				json.Unmarshal(b, &proposals)
			}
			if proposals == nil {
				proposals = []map[string]any{}
			}
			writeJSON(w, map[string]any{"ok": true, "proposals": proposals, "count": len(proposals)})

		case "POST":
			var req struct {
				Title       string   `json:"title"`
				Description string   `json:"description"`
				Options     []string `json:"options"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
				return
			}
			if req.Title == "" {
				writeJSON(w, map[string]any{"ok": false, "error": "title required"})
				return
			}
			var proposals []map[string]any
			if b, _ := os.ReadFile(proposalFile); b != nil {
				json.Unmarshal(b, &proposals)
			}
			if proposals == nil {
				proposals = []map[string]any{}
			}
			proposal := map[string]any{
				"id":          fmt.Sprintf("prop-%d", time.Now().UnixNano()),
				"title":       req.Title,
				"description": req.Description,
				"options":     req.Options,
				"votes":       map[string]int{},
				"status":      "active",
				"created_at":  time.Now().UTC().Format(time.RFC3339),
			}
			proposals = append(proposals, proposal)
			fileutil.WriteJSON(proposalFile, proposals)
			appendRoyalAudit(dataDir, "governance.proposal", map[string]any{"id": proposal["id"], "title": req.Title})
			writeJSON(w, map[string]any{"ok": true, "proposal": proposal})

		default:
			http.Error(w, "GET/POST only", 405)
		}
	}
}

// ── Royal Relay ──────────────────────────────────────────────────────────────

func RealRelayForward(target, command, data string, relayURL string) (map[string]any, error) {
	url := relayURL
	if url == "" {
		cachedTargetsMu.RLock()
		ni, ok := cachedTargets[target]
		cachedTargetsMu.RUnlock()
		if !ok {
			return nil, fmt.Errorf("unknown target: %s", target)
		}
		url = ni.Addr
	}
	if !strings.HasPrefix(url, "http") {
		url = "http://" + url
	}
	url = strings.TrimRight(url, "/") + "/api/relay/receive"

	body, _ := json.Marshal(map[string]string{
		"command": command,
		"data":    data,
	})
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("forward failed: %w", err)
	}
	defer resp.Body.Close()
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("bad response: %w", err)
	}
	result["_target"] = target
	result["_url"] = url
	return result, nil
}


// RoyalRelayHandler handles the RoyalRelayHandler HTTP request.
func RoyalRelayHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) { return }
		if !isRoyalNodePath(dataDir) { http.Error(w, "forbidden", 403); return }

		relayFile := filepath.Join(dataDir, "royal_relay.json")

		switch r.Method {
		case "GET":
			var messages []map[string]any
			if b, _ := os.ReadFile(relayFile); b != nil {
				json.Unmarshal(b, &messages)
			}
			if messages == nil {
				messages = []map[string]any{}
			}
			target := r.URL.Query().Get("target")
			if target != "" {
				filtered := []map[string]any{}
				for _, m := range messages {
					if m["target"] == target {
						filtered = append(filtered, m)
					}
				}
				writeJSON(w, map[string]any{"ok": true, "messages": filtered, "count": len(filtered)})
				return
			}
			writeJSON(w, map[string]any{"ok": true, "messages": messages, "count": len(messages)})

		case "POST":
			var req struct {
				Target   string `json:"target"`
				Command  string `json:"command"`
				Data     string `json:"data"`
				RelayURL string `json:"relay_url"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
				return
			}
			if req.Target == "" || req.Command == "" {
				writeJSON(w, map[string]any{"ok": false, "error": "target and command required"})
				return
			}

			lazyInitCachedTargets(dataDir)

			result, fwdErr := RealRelayForward(req.Target, req.Command, req.Data, req.RelayURL)
			if fwdErr == nil {
				appendRoyalAudit(dataDir, "relay.forward", map[string]any{
					"target": req.Target, "command": req.Command,
					"response": result,
				})
				writeJSON(w, map[string]any{"ok": true, "forwarded": true, "response": result})
				return
			}
			slog.Warn("relay forward failed, queuing", "target", req.Target, "err", fwdErr)

			var messages []map[string]any
			if b, _ := os.ReadFile(relayFile); b != nil {
				json.Unmarshal(b, &messages)
			}
			if messages == nil {
				messages = []map[string]any{}
			}
			msg := map[string]any{
				"id":         fmt.Sprintf("relay-%d", time.Now().UnixNano()),
				"target":     req.Target,
				"command":    req.Command,
				"data":       req.Data,
				"status":     "queued",
				"created_at": time.Now().UTC().Format(time.RFC3339),
			}
			messages = append(messages, msg)
			fileutil.WriteJSON(relayFile, messages)
			appendRoyalAudit(dataDir, "relay.queue", map[string]any{
				"target": req.Target, "command": req.Command,
			})
			writeJSON(w, map[string]any{"ok": true, "queued": true, "message": msg})

		default:
			http.Error(w, "GET/POST only", 405)
		}
	}
}

// ── Royal Chat Bridge ────────────────────────────────────────────────────────

func RoyalChatBroadcastHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) { return }
		if !isRoyalNodePath(dataDir) { http.Error(w, "forbidden", 403); return }
		if r.Method != "POST" { http.Error(w, "POST required", 405); return }
		if SimplexCmd == nil || !BridgeConnected {
			writeJSON(w, map[string]any{"ok": false, "error": "bridge not connected"})
			return
		}

		var req struct {
			Message string `json:"message"`
			Channel string `json:"channel"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
			return
		}
		if req.Message == "" {
			writeJSON(w, map[string]any{"ok": false, "error": "message required"})
			return
		}

		resp, err := SimplexCmd("/_contacts 1")
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		contacts, _ := resp["contacts"].([]any)
		sent := 0
		for _, c := range contacts {
			cm, ok := c.(map[string]any)
			if !ok { continue }
			id, _ := cm["contactId"].(float64)
			if id == 0 { continue }
			if req.Channel != "" {
				ch, _ := cm["group"].(string)
				if ch != req.Channel { continue }
			}
			SimplexCmd(fmt.Sprintf("/_send %d %s", int64(id), req.Message))
			sent++
			time.Sleep(100 * time.Millisecond)
		}
		appendRoyalAudit(dataDir, "chat.broadcast", map[string]any{
			"sent": sent, "channel": req.Channel,
		})
		writeJSON(w, map[string]any{"ok": true, "sent": sent})
	}
}


// RoyalTreasuryAlertHandler handles the RoyalTreasuryAlertHandler HTTP request.
func RoyalTreasuryAlertHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) { return }
		if !isRoyalNodePath(dataDir) { http.Error(w, "forbidden", 403); return }
		if r.Method != "POST" { http.Error(w, "POST required", 405); return }
		if SimplexCmd == nil || !BridgeConnected {
			writeJSON(w, map[string]any{"ok": false, "error": "bridge not connected"})
			return
		}

		var req struct {
			Message  string `json:"message"`
			Severity string `json:"severity"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
			return
		}
		if req.Message == "" {
			writeJSON(w, map[string]any{"ok": false, "error": "message required"})
			return
		}
		if req.Severity == "" { req.Severity = "info" }

		emoji := map[string]string{"info": "i", "warning": "!", "critical": "!!"}
		prefix := emoji[req.Severity]
		if prefix == "" { prefix = "i" }

		resp, err := SimplexCmd("/_contacts 1")
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		contacts, _ := resp["contacts"].([]any)
		sent := 0
		for _, c := range contacts {
			cm, ok := c.(map[string]any)
			if !ok { continue }
			id, _ := cm["contactId"].(float64)
			if id == 0 { continue }
			SimplexCmd(fmt.Sprintf("/_send %d [%s] Treasury Alert: %s", int64(id), req.Severity, req.Message))
			sent++
			time.Sleep(100 * time.Millisecond)
		}
		appendRoyalAudit(dataDir, "chat.treasury-alert", map[string]any{
			"severity": req.Severity, "sent": sent,
		})
		writeJSON(w, map[string]any{"ok": true, "sent": sent})
	}
}


// RoyalHealthHandler handles the RoyalHealthHandler HTTP request.
func RoyalHealthHandler(dataDir string, startTime time.Time) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) { return }
		if !isRoyalNodePath(dataDir) { http.Error(w, "forbidden", 403); return }

		uptime := time.Since(startTime).Round(time.Second).String()
		writeJSON(w, map[string]any{
			"ok":            true,
			"node":          "royal",
			"uptime":        uptime,
			"dc_cloud":      GlobalDCCloud != nil,
			"bridge_alive":  BridgeConnected,
		})
	}
}


// RoyalEconomyReportHandler handles the RoyalEconomyReportHandler HTTP request.
func RoyalEconomyReportHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) { return }
		if !isRoyalNodePath(dataDir) { http.Error(w, "forbidden", 403); return }

		ledger := economy.LoadLedger(dataDir)
		banknotes, _ := economy.LoadBanknotesV2(dataDir)
		dd := economy.LoadDividendRounds(dataDir)

		totalDist := int64(0)
		for _, r := range dd.Rounds { totalDist += r.TotalNg }

		report := map[string]any{
			"timestamp":           time.Now().UTC().Format(time.RFC3339),
			"node":                "royal",
			"supply_ng":           ledger.TotalSupply,
			"accounts":            len(ledger.Accounts),
			"banknotes":           len(banknotes),
			"dividend_rounds":     len(dd.Rounds),
			"total_distributed_ng": totalDist,
			"treasury_health":     "adequate",
			"recommendations":     []string{},
		}
		writeJSON(w, map[string]any{"ok": true, "report": report})
	}
}
