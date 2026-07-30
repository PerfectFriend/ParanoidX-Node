// Package api provides HTTP handlers and API endpoints for the simplex-node server
package api

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"ParanoidX/internal/economy"
	"ParanoidX/internal/fileutil"
	"ParanoidX/internal/middleware"
)

// ── Audit Log ─────────────────────────────────────────────────────────────────

// AuditEntry records a single auditable action with actor and timestamp.
type AuditEntry struct {
	ID        string `json:"id"`
	Action    string `json:"action"`
	Actor     string `json:"actor"`
	Detail    string `json:"detail,omitempty"`
	Timestamp string `json:"timestamp"`
}

var (
	auditMu sync.RWMutex
	auditLog []AuditEntry
)

func init() {
	auditLog = make([]AuditEntry, 0, 1000)
}

func logAudit(action, actor, detail string) {
	auditMu.Lock()
	defer auditMu.Unlock()
	auditLog = append(auditLog, AuditEntry{
		ID:        fmt.Sprintf("aud-%d", time.Now().UnixNano()),
		Action:    action,
		Actor:     actor,
		Detail:    detail,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
	if len(auditLog) > 1000 {
		auditLog = auditLog[len(auditLog)-1000:]
	}
}

// AuditLogHandler returns the audit log with optional limit/offset pagination.
func AuditLogHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limitStr := r.URL.Query().Get("limit")
		offsetStr := r.URL.Query().Get("offset")
		limit := 50
		offset := 0
		if n, err := fmt.Sscanf(limitStr, "%d", &limit); err != nil || n != 1 {
			limit = 50
		}
		if n, err := fmt.Sscanf(offsetStr, "%d", &offset); err != nil || n != 1 {
			offset = 0
		}
		auditMu.RLock()
		total := len(auditLog)
		var out []AuditEntry
		if offset < total {
			end := offset + limit
			if end > total {
				end = total
			}
			out = auditLog[offset:end]
		}
		auditMu.RUnlock()
		writeJSON(w, map[string]any{"ok": true, "entries": out, "total": total, "limit": limit, "offset": offset})
	}
}

// ── Detailed System Metrics ───────────────────────────────────────────────────

// DetailedMetricsHandler returns CPU, RAM, disk, and data_dir size metrics.
func DetailedMetricsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		metrics := map[string]any{
			"memory": map[string]any{
				"alloc_mb":       m.Alloc / (1024 * 1024),
				"total_alloc_mb": m.TotalAlloc / (1024 * 1024),
				"sys_mb":         m.Sys / (1024 * 1024),
				"heap_mb":        m.HeapAlloc / (1024 * 1024),
				"stack_mb":       m.StackInuse / (1024 * 1024),
				"gc_cycles":      m.NumGC,
				"gc_pause_ns":    m.PauseTotalNs,
			},
			"goroutines": runtime.NumGoroutine(),
			"cgo_calls":  runtime.NumCgoCall(),
			"cpu_cores":  runtime.NumCPU(),
			"go_version": runtime.Version(),
		}
		if b, err := os.ReadFile("/proc/loadavg"); err == nil {
			parts := strings.Fields(string(b))
			if len(parts) >= 3 {
				metrics["load"] = map[string]any{"1m": parts[0], "5m": parts[1], "15m": parts[2]}
			}
		}
		writeJSON(w, metrics)
	}
}

// ── Chat Analytics ────────────────────────────────────────────────────────────

// ChatAnalytics holds message volume and engagement metrics for the chat system.
type ChatAnalytics struct {
	TotalMessages     int            `json:"total_messages"`
	MessagesToday     int            `json:"messages_today"`
	UniqueChats       int            `json:"unique_chats"`
	MessagesPerChat   map[string]int `json:"messages_per_chat"`
	TopSenders        map[string]int `json:"top_senders"`
	MessagesByHour    [24]int        `json:"messages_by_hour"`
	AvgMessageLength  float64        `json:"avg_message_length"`
	PinnedMessages    int            `json:"pinned_messages"`
	ReactionsCount    int            `json:"reactions_count"`
}

// ChatAnalyticsHandler returns message volume analytics grouped by day.
func ChatAnalyticsHandler(hub *ChatHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		msgs := hub.GetMessages()
		analytics := ChatAnalytics{
			TotalMessages:   len(msgs),
			MessagesPerChat: map[string]int{},
			TopSenders:      map[string]int{},
		}
		chats := map[string]int{}
		today := time.Now().UTC().Truncate(24 * time.Hour)
		totalLen := 0

		for _, m := range msgs {
			chats[m.ChatID]++
			analytics.TopSenders[m.From]++
			totalLen += len(m.Text)

			t, err := time.Parse(time.RFC3339, m.Timestamp)
			if err == nil {
				if t.After(today) {
					analytics.MessagesToday++
				}
				analytics.MessagesByHour[t.Hour()]++
			}
			if m.Pinned {
				analytics.PinnedMessages++
			}
			if m.Reactions != nil && len(m.Reactions) > 0 {
				analytics.ReactionsCount++
			}
		}

		analytics.UniqueChats = len(chats)
		for cid, count := range chats {
			analytics.MessagesPerChat[cid] = count
		}
		if len(msgs) > 0 {
			analytics.AvgMessageLength = float64(totalLen) / float64(len(msgs))
		}
		writeJSON(w, map[string]any{"ok": true, "analytics": analytics})
	}
}

// ── Message Templates ─────────────────────────────────────────────────────────

// MessageTemplate defines a reusable message template with variable placeholders.
type MessageTemplate struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Text    string   `json:"text"`
	Vars    []string `json:"vars,omitempty"`
}

var (
	templatesMu sync.RWMutex
	templates   []MessageTemplate
	templatesSeq int64
)

// ChatTemplatesHandler manages message templates (list/create/update/delete).
func ChatTemplatesHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			templatesMu.RLock()
			out := append([]MessageTemplate{}, templates...)
			templatesMu.RUnlock()
			writeJSON(w, map[string]any{"ok": true, "templates": out})
		case "POST":
			var req struct {
				Name string   `json:"name"`
				Text string   `json:"text"`
				Vars []string `json:"vars,omitempty"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
				return
			}
			templatesSeq++
			t := MessageTemplate{
				ID:   fmt.Sprintf("tmpl-%d", templatesSeq),
				Name: req.Name,
				Text: req.Text,
				Vars: req.Vars,
			}
			templatesMu.Lock()
			templates = append(templates, t)
			templatesMu.Unlock()
			writeJSON(w, map[string]any{"ok": true, "template": t})
		case "DELETE":
			id := r.URL.Query().Get("id")
			templatesMu.Lock()
			remaining := make([]MessageTemplate, 0, len(templates))
			for _, t := range templates {
				if t.ID != id {
					remaining = append(remaining, t)
				}
			}
			templates = remaining
			templatesMu.Unlock()
			writeJSON(w, map[string]any{"ok": true})
		case "PATCH":
			id := r.URL.Query().Get("id")
			var req struct {
				Name string   `json:"name"`
				Text string   `json:"text"`
				Vars []string `json:"vars,omitempty"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
				return
			}
			templatesMu.Lock()
			for i := range templates {
				if templates[i].ID == id {
					if req.Name != "" {
						templates[i].Name = req.Name
					}
					if req.Text != "" {
						templates[i].Text = req.Text
					}
					if req.Vars != nil {
						templates[i].Vars = req.Vars
					}
				}
			}
			templatesMu.Unlock()
			writeJSON(w, map[string]any{"ok": true})
		default:
			http.Error(w, "GET/POST/DELETE/PATCH", 405)
		}
	}
}

// ChatTemplateSendHandler looks up a template, replaces {var} placeholders, and sends.
func ChatTemplateSendHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		var req struct {
			TemplateID string            `json:"template_id"`
			ContactID  int64             `json:"contact_id"`
			Vars       map[string]string `json:"vars"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
			return
		}
		if req.TemplateID == "" {
			writeJSON(w, map[string]any{"ok": false, "error": "template_id required"})
			return
		}
		templatesMu.RLock()
		var tmpl *MessageTemplate
		for i, t := range templates {
			if t.ID == req.TemplateID {
				tmpl = &templates[i]
				break
			}
		}
		templatesMu.RUnlock()
		if tmpl == nil {
			writeJSON(w, map[string]any{"ok": false, "error": "template not found"})
			return
		}
		text := tmpl.Text
		for k, v := range req.Vars {
			text = strings.ReplaceAll(text, "{"+k+"}", v)
		}
		if SimplexCmd == nil {
			writeJSON(w, map[string]any{"ok": false, "error": "simplex not connected"})
			return
		}
		if req.ContactID <= 0 {
			writeJSON(w, map[string]any{"ok": false, "error": "contact_id required"})
			return
		}
		_, err := SimplexCmd(fmt.Sprintf("/_send %d %s", req.ContactID, text))
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "text": text})
	}
}

// ── Batch Forward ─────────────────────────────────────────────────────────────

// ChatBatchForwardHandler forwards multiple messages to contacts in batch.
func ChatBatchForwardHandler(hub *ChatHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		if SimplexCmd == nil || !BridgeConnected {
			writeJSON(w, map[string]any{"ok": false, "error": "bridge not connected"})
			return
		}
		var req struct {
			MessageIDs []string `json:"message_ids"`
			TargetIDs  []string `json:"target_ids"` // contact IDs
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
			return
		}
		msgs := hub.GetMessages()
		msgMap := map[string]ChatMessage{}
		for _, m := range msgs {
			msgMap[m.ID] = m
		}
		type result struct {
			MsgID    string `json:"msg_id"`
			TargetID string `json:"target_id"`
			OK       bool   `json:"ok"`
			Error    string `json:"error,omitempty"`
		}
		results := make([]result, 0, len(req.MessageIDs)*len(req.TargetIDs))
		for _, mid := range req.MessageIDs {
			m, ok := msgMap[mid]
			if !ok {
				continue
			}
			for _, tid := range req.TargetIDs {
				cid, err := strconv.ParseInt(tid, 10, 64)
				if err != nil {
					results = append(results, result{MsgID: mid, TargetID: tid, OK: false, Error: "invalid contact id"})
					continue
				}
				_, err = SimplexCmd(fmt.Sprintf("/_send %d %s", cid, m.Text))
				if err != nil {
					results = append(results, result{MsgID: mid, TargetID: tid, OK: false, Error: err.Error()})
				} else {
					results = append(results, result{MsgID: mid, TargetID: tid, OK: true})
				}
				time.Sleep(100 * time.Millisecond)
			}
		}
		writeJSON(w, map[string]any{"ok": true, "results": results})
	}
}

// ── System Diagnostics ────────────────────────────────────────────────────────

// SystemDiagnosticsHandler returns system diagnostics (goroutines, memory, disk).
func SystemDiagnosticsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		diag := map[string]any{
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"checks":    []map[string]any{},
		}
		checks := []map[string]any{}

		// Bridge check
		checks = append(checks, map[string]any{
			"name": "bridge", "status": BridgeConnected,
			"detail": map[string]any{"connected": BridgeConnected, "reconnects": BridgeReconnectCount},
		})

		// Disk check
		disk := statusCheckDisk()
		checks = append(checks, map[string]any{"name": "disk", "status": true, "detail": disk})

		// Memory check
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		checks = append(checks, map[string]any{
			"name": "memory", "status": m.Alloc < 500*1024*1024,
			"detail": map[string]any{"alloc_mb": m.Alloc / (1024 * 1024), "sys_mb": m.Sys / (1024 * 1024)},
		})

		// Load check
		if b, err := os.ReadFile("/proc/loadavg"); err == nil {
			parts := strings.Fields(string(b))
			if len(parts) >= 3 {
				checks = append(checks, map[string]any{"name": "load", "status": true, "detail": map[string]any{"1m": parts[0], "5m": parts[1], "15m": parts[2]}})
			}
		}

		// Docker check
		for _, name := range []string{"simplex-node-tor", "simplex-node-coturn"} {
			out, err := runCmd("docker", "ps", "--filter", "name="+name, "--format", "{{.Status}}")
			if err != nil || strings.TrimSpace(string(out)) == "" {
				checks = append(checks, map[string]any{"name": "docker_" + name, "status": false, "detail": "not running"})
			} else {
				checks = append(checks, map[string]any{"name": "docker_" + name, "status": true, "detail": strings.TrimSpace(string(out))})
			}
		}

		// CPU info
		if b, err := os.ReadFile("/proc/cpuinfo"); err == nil {
			coreCount := 0
			for _, line := range strings.Split(string(b), "\n") {
				if strings.HasPrefix(line, "processor") {
					coreCount++
				}
			}
			checks = append(checks, map[string]any{"name": "cpu", "status": true, "detail": map[string]any{"cores": coreCount}})
		}

		// Uptime
		if b, err := os.ReadFile("/proc/uptime"); err == nil {
			parts := strings.Fields(string(b))
			if len(parts) > 0 {
				if secs, err := strconv.ParseFloat(parts[0], 64); err == nil {
					checks = append(checks, map[string]any{"name": "uptime", "status": true, "detail": map[string]any{"hours": fmt.Sprintf("%.1f", secs/3600)}})
				}
			}
		}

		// Network interfaces
		if b, err := os.ReadFile("/proc/net/dev"); err == nil {
			ifaces := []map[string]any{}
			for _, line := range strings.Split(string(b), "\n") {
				line = strings.TrimSpace(line)
				if strings.Contains(line, ":") && !strings.HasPrefix(line, "Inter-") && !strings.HasPrefix(line, "face") {
					parts := strings.Fields(line)
					if len(parts) >= 10 {
						name := strings.TrimSuffix(parts[0], ":")
						rxBytes, _ := strconv.ParseInt(parts[1], 10, 64)
						txBytes, _ := strconv.ParseInt(parts[9], 10, 64)
						ifaces = append(ifaces, map[string]any{
							"name": name,
							"rx_mb": fmt.Sprintf("%.1f", float64(rxBytes)/1024/1024),
							"tx_mb": fmt.Sprintf("%.1f", float64(txBytes)/1024/1024),
						})
					}
				}
			}
			checks = append(checks, map[string]any{"name": "network", "status": true, "detail": map[string]any{"interfaces": ifaces}})
		}

		// Temperature (if available)
		if b, err := os.ReadFile("/sys/class/thermal/thermal_zone0/temp"); err == nil {
			temp := strings.TrimSpace(string(b))
			if t, err := strconv.ParseFloat(temp, 64); err == nil {
				checks = append(checks, map[string]any{"name": "temperature", "status": t < 80000, "detail": map[string]any{"celsius": fmt.Sprintf("%.1f", t/1000)}})
			}
		}

		diag["checks"] = checks
		allOK := true
		for _, c := range checks {
			if ok, _ := c["status"].(bool); !ok {
				allOK = false
			}
		}
		diag["healthy"] = allOK
		writeJSON(w, diag)
	}
}

func runCmd(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	return cmd.Output()
}

func statusCheckDisk() map[string]any {
	disk := map[string]any{}
	out, err := runCmd("df", "-h", "/")
	if err == nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(lines) >= 2 {
			fields := strings.Fields(lines[1])
			if len(fields) >= 4 {
				disk["total"] = fields[1]
				disk["used"] = fields[2]
				disk["avail"] = fields[3]
				disk["pct"] = fields[4]
			}
		}
	}
	return disk
}

// ── Status Page ───────────────────────────────────────────────────────────────

// ── Message Index (performance) ──────────────────────────────────────────────

// MessageIndex provides fast O(1) lookup of messages by ID, rebuilt periodically from the hub.
type MessageIndex struct {
	mu   sync.RWMutex
	byID map[string]int // messageID → index in slice
	hub  *ChatHub
}

var GlobalMsgIndex *MessageIndex

// NewMessageIndex creates a MessageIndex and starts a background rebuild loop every 30s.
func NewMessageIndex(hub *ChatHub) *MessageIndex {
	idx := &MessageIndex{
		byID: map[string]int{},
		hub:  hub,
	}
	idx.rebuild()
	go func() {
		for {
			time.Sleep(30 * time.Second)
			idx.rebuild()
		}
	}()
	return idx
}

func (idx *MessageIndex) rebuild() {
	msgs := idx.hub.GetMessages()
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.byID = make(map[string]int, len(msgs))
	for i, m := range msgs {
		idx.byID[m.ID] = i
	}
}

// GetByID returns a pointer to the message with the given ID, or nil if not found.
func (idx *MessageIndex) GetByID(id string) *ChatMessage {
	idx.mu.RLock()
	pos, ok := idx.byID[id]
	idx.mu.RUnlock()
	if !ok {
		return nil
	}
	msgs := idx.hub.GetMessages()
	if pos < 0 || pos >= len(msgs) {
		return nil
	}
	return &msgs[pos]
}

// Search performs a simple text search across message text, ID, and sender fields.
func (idx *MessageIndex) Search(q string, limit int) []ChatMessage {
	msgs := idx.hub.GetMessages()
	out := make([]ChatMessage, 0, limit)
	for i := len(msgs) - 1; i >= 0 && len(out) < limit; i-- {
		if contains(msgs[i].Text, q) || contains(msgs[i].ID, q) || contains(msgs[i].From, q) {
			out = append(out, msgs[i])
		}
	}
	return out
}

// SearchIndexHandler returns the current search index stats.
func SearchIndexHandler(idx *MessageIndex) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		limitStr := r.URL.Query().Get("limit")
		limit := 20
		if n, err := fmt.Sscanf(limitStr, "%d", &limit); err != nil || n != 1 {
			limit = 20
		}
		results := idx.Search(q, limit)
		writeJSON(w, map[string]any{"ok": true, "results": results, "count": len(results)})
	}
}

// BackupHandler triggers a USB backup on demand.
func BackupHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		script := filepath.Join(os.Getenv("HOME"), "simplex-node", "scripts", "backup-to-usb.sh")
		cmd := exec.Command("bash", script)
		cmd.Dir = filepath.Join(os.Getenv("HOME"), "simplex-node")
		go func() {
			cmd.Run()
			// Auto-verify the latest backup in A1-backups
			backupDir := filepath.Join(os.Getenv("HOME"), "A1-backups")
			entries, _ := os.ReadDir(backupDir)
			if len(entries) > 0 {
				sort.Slice(entries, func(i, j int) bool {
					ii, _ := entries[i].Info()
					ji, _ := entries[j].Info()
					return ii.ModTime().After(ji.ModTime())
				})
				autoVerifyBackup(backupDir, entries[0].Name())
			}
		}()
		writeJSON(w, map[string]any{"ok": true, "message": "backup started"})
	}
}

// ── Rate Limit Status (security) ──────────────────────────────────────────────

var (
	rateLimitHits = map[string]int{} // IP → count
	rateLimitMu   sync.RWMutex
)

// TrackRateLimit records a rate-limit hit for the given IP address.
func TrackRateLimit(ip string) {
	rateLimitMu.Lock()
	rateLimitHits[ip]++
	if len(rateLimitHits) > 1000 {
		for k := range rateLimitHits {
			if rateLimitHits[k] < 5 {
				delete(rateLimitHits, k)
			}
		}
	}
	rateLimitMu.Unlock()
}

// RateLimitStatusHandler returns rate limit hit counts per IP.
func RateLimitStatusHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rateLimitMu.RLock()
		top := map[string]int{}
		for ip, count := range rateLimitHits {
			top[ip] = count
		}
		rateLimitMu.RUnlock()
		writeJSON(w, map[string]any{"ok": true, "total_tracked": len(top), "ips": top})
	}
}

// ── Content Filter (security) ─────────────────────────────────────────────────


// ContentFilterHandler manages content filtering (blocked words add/remove/set).
func ContentFilterHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			if GlobalContentFilter != nil {
				writeJSON(w, map[string]any{"ok": true, "rules": GlobalContentFilter.GetRules()})
			} else {
				writeJSON(w, map[string]any{"ok": true, "rules": []any{}})
			}
			return
		}
		if r.Method == "POST" {
			var req struct {
				Word        string `json:"word"`
				Words       []string `json:"words"`
				Action      string   `json:"action"`
				Replacement string   `json:"replacement"`
				Remove      string   `json:"remove"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "bad json", 400)
				return
			}
			if req.Remove != "" && GlobalContentFilter != nil {
				GlobalContentFilter.RemoveRule(req.Remove)
			}
			if req.Word != "" && GlobalContentFilter != nil {
				GlobalContentFilter.AddRule(req.Word, req.Action, req.Replacement)
			}
			for _, w := range req.Words {
				if GlobalContentFilter != nil {
					GlobalContentFilter.AddRule(w, req.Action, req.Replacement)
				}
			}
			rules := []ContentFilterRule{}
			if GlobalContentFilter != nil {
				rules = GlobalContentFilter.GetRules()
			}
			writeJSON(w, map[string]any{"ok": true, "rules": rules})
		}
	}
}

// IsContentBlocked returns true if the given text is blocked by the content filter.
func IsContentBlocked(text string) bool {
	if GlobalContentFilter == nil {
		return false
	}
	_, _, blocked := GlobalContentFilter.Filter(text)
	return blocked
}

// StatusPageHandler returns an operational status page with service health.
func StatusPageHandler(hub *ChatHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page := map[string]any{
			"status":      "operational",
			"version":     "unknown",
			"uptime_hours": "?",
			"services": map[string]any{
				"bridge": BridgeConnected,
				"hub":    hub != nil,
				"bot":    SimplexCmd != nil,
			},
			"metrics": map[string]any{
				"messages":   hub.MessageCount(),
				"sse_clients": hub.SSEClientCount(),
			},
			"docker": dockerStatus(),
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		}
		writeJSON(w, page)
	}
}

// DockerStatusHandler returns docker container status.
func DockerStatusHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, dockerStatus())
	}
}

func dockerStatus() map[string]any {
	composeDir := os.Getenv("SIMPLEX_SRC")
	if composeDir == "" {
		composeDir = filepath.Join(os.Getenv("HOME"), "simplex-node")
	}
	composeDir = filepath.Join(composeDir, "docker")
	cmd := exec.Command("docker", "compose", "ps", "--format", "{{.Name}}\t{{.Status}}")
	cmd.Dir = composeDir
	out, err := cmd.Output()
	if err != nil {
		return map[string]any{"error": err.Error(), "healthy": false}
	}
	containers := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) == 2 {
			containers[parts[0]] = parts[1]
		}
	}
	healthy := true
	for _, st := range containers {
		if !strings.Contains(st, "Up") {
			healthy = false
			break
		}
	}
	return map[string]any{
		"containers": containers,
		"healthy":    healthy,
		"count":      len(containers),
	}
}

// SystemMetricsHandler returns CPU, RAM, disk usage.
func readUptime() int64 {
	d, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	fs := strings.Fields(string(d))
	if len(fs) < 1 {
		return 0
	}
	secs, _ := strconv.ParseFloat(fs[0], 64)
	return int64(secs)
}

func readMemInfo() (totalKB, availKB uint64) {
	d, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0
	}
	for _, line := range strings.Split(string(d), "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			fmt.Sscanf(line, "MemTotal: %d kB", &totalKB)
		}
		if strings.HasPrefix(line, "MemAvailable:") {
			fmt.Sscanf(line, "MemAvailable: %d kB", &availKB)
		}
	}
	return
}

func readLoadAvg() (float64, float64, float64) {
	d, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, 0, 0
	}
	fs := strings.Fields(string(d))
	if len(fs) < 3 {
		return 0, 0, 0
	}
	var l1, l5, l15 float64
	fmt.Sscanf(fs[0], "%f", &l1)
	fmt.Sscanf(fs[1], "%f", &l5)
	fmt.Sscanf(fs[2], "%f", &l15)
	return l1, l5, l15
}

// SystemMetricsHandler returns CPU, RAM, disk usage, goroutine count, and data directory size.
func SystemMetricsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m := runtime.MemStats{}
		runtime.ReadMemStats(&m)
		uptime := readUptime()
		ramTotal, ramAvail := readMemInfo()
		l1, _, _ := readLoadAvg()
		cpuPct := l1 * 100 / float64(runtime.NumCPU())
		if cpuPct > 100 {
			cpuPct = 100
		}
		metrics := map[string]any{
			"memory": map[string]any{
				"alloc_mb":       m.Alloc / 1024 / 1024,
				"total_alloc_mb": m.TotalAlloc / 1024 / 1024,
				"sys_mb":         m.Sys / 1024 / 1024,
				"num_gc":         m.NumGC,
			},
			"goroutines":     runtime.NumGoroutine(),
			"cpus":           runtime.NumCPU(),
			"cpu_percent":    cpuPct,
			"go_version":     runtime.Version(),
			"uptime_seconds": uptime,
		}
		if ramTotal > 0 {
			usedPct := float64(ramTotal-ramAvail) / float64(ramTotal) * 100
			metrics["ram"] = map[string]any{
				"total_mb":  ramTotal / 1024,
				"used_mb":   (ramTotal - ramAvail) / 1024,
				"avail_mb":  ramAvail / 1024,
				"used_pct":  fmt.Sprintf("%.1f%%", usedPct),
			}
		}
		// Disk usage
		dataDir := os.Getenv("DATA_DIR")
		if dataDir == "" {
			dataDir = filepath.Join(os.Getenv("HOME"), ".local/share/simplex-node")
		}
		if _, err := os.Stat(dataDir); err == nil {
			var dataSize int64
			filepath.Walk(dataDir, func(p string, fi os.FileInfo, err error) error {
				if err == nil && !fi.IsDir() {
					dataSize += fi.Size()
				}
				return nil
			})
			metrics["data_dir"] = map[string]any{
				"path":    dataDir,
				"size_mb": dataSize / 1024 / 1024,
			}
		}
		// Root disk
		var disk fs
		if err := diskUsage("/", &disk); err == nil {
			metrics["disk"] = map[string]any{
				"total_gb": disk.Total / 1024 / 1024 / 1024,
				"used_gb":  disk.Used / 1024 / 1024 / 1024,
				"avail_gb": disk.Avail / 1024 / 1024 / 1024,
				"used_pct": fmt.Sprintf("%.1f%%", float64(disk.Used)/float64(disk.Total)*100),
			}
		}
		writeJSON(w, metrics)
	}
}

type fs struct {
	Total, Used, Avail uint64
}

// ── Bandwidth Tracker ──────────────────────────────────────────────────────

type ifaceSnapshot struct {
	RX, TX uint64
}

type bandwidthSample struct {
	Time   time.Time
	RXbps  float64
	TXbps  float64
}

var (
	bwMu       sync.Mutex
	bwLast     map[string]ifaceSnapshot
	bwHistory  map[string][]bandwidthSample
	bwLastTime time.Time
)

func readNetDev() (map[string]ifaceSnapshot, error) {
	b, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return nil, err
	}
	ifaces := map[string]ifaceSnapshot{}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, ":") && !strings.HasPrefix(line, "Inter-") && !strings.HasPrefix(line, "face") {
			parts := strings.Fields(line)
			if len(parts) >= 10 {
				name := strings.TrimSuffix(parts[0], ":")
				rx, _ := strconv.ParseUint(parts[1], 10, 64)
				tx, _ := strconv.ParseUint(parts[9], 10, 64)
				ifaces[name] = ifaceSnapshot{RX: rx, TX: tx}
			}
		}
	}
	return ifaces, nil
}


// InitBandwidthTracker handles the InitBandwidthTracker HTTP request.
func InitBandwidthTracker() {
	bwLast, _ = readNetDev()
	bwLastTime = time.Now()
	bwHistory = map[string][]bandwidthSample{}
	// Start periodic sampling
	go func() {
		for {
			time.Sleep(60 * time.Second)
			bwMu.Lock()
			now := time.Now()
			delta := now.Sub(bwLastTime).Seconds()
			cur, err := readNetDev()
			if err == nil && bwLast != nil {
				for name, curIf := range cur {
					if prev, ok := bwLast[name]; ok && delta > 0 {
						rxBps := float64(curIf.RX-prev.RX) / delta
						txBps := float64(curIf.TX-prev.TX) / delta
						hist := bwHistory[name]
						hist = append(hist, bandwidthSample{Time: now, RXbps: rxBps, TXbps: txBps})
						if len(hist) > 60 { // keep 60 samples (1 hour)
							hist = hist[len(hist)-60:]
						}
						bwHistory[name] = hist
					}
				}
				bwLast = cur
				bwLastTime = now
			}
			bwMu.Unlock()
		}
	}()
}

// BandwidthHandler returns current bandwidth usage per interface.
// GET /api/admin/metrics/bandwidth
func BandwidthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bwMu.Lock()
		cur, err := readNetDev()
		now := time.Now()
		delta := now.Sub(bwLastTime).Seconds()
		rate := map[string]any{}
		if err == nil && bwLast != nil {
			for name, curIf := range cur {
				if prev, ok := bwLast[name]; ok && delta > 0 {
					rxBps := float64(curIf.RX-prev.RX) / delta
					txBps := float64(curIf.TX-prev.TX) / delta
					hist := []map[string]any{}
					for _, s := range bwHistory[name] {
						hist = append(hist, map[string]any{"time": s.Time.Format(time.RFC3339), "rx_bps": s.RXbps, "tx_bps": s.TXbps})
					}
					rate[name] = map[string]any{
						"rx_bps":        rxBps,
						"tx_bps":        txBps,
						"rx_kbps":       fmt.Sprintf("%.1f", rxBps/1024),
						"tx_kbps":       fmt.Sprintf("%.1f", txBps/1024),
						"rx_total_mb":   fmt.Sprintf("%.1f", float64(curIf.RX)/1024/1024),
						"tx_total_mb":   fmt.Sprintf("%.1f", float64(curIf.TX)/1024/1024),
						"history":       hist,
					}
				}
			}
		}
		bwMu.Unlock()
		writeJSON(w, map[string]any{"ok": true, "interfaces": rate, "sample_interval_s": 60})
	}
}

// ── Memory Trend Monitoring (I7) ──────────────────────────────────────────────

type memSample struct {
	Time    time.Time
	TotalMB float64
	UsedMB  float64
	FreeMB  float64
	Pct     float64
}

var (
	memMu      sync.RWMutex
	memHistory []memSample
)

func readMemDetail() (total, free, avail, cached float64, err error) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0, 0, 0, err
	}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := sc.Text()
		var val float64
		if strings.HasPrefix(line, "MemTotal:") {
			fmt.Sscanf(line, "MemTotal: %f kB", &val)
			total = val / 1024
		}
		if strings.HasPrefix(line, "MemFree:") {
			fmt.Sscanf(line, "MemFree: %f kB", &val)
			free = val / 1024
		}
		if strings.HasPrefix(line, "MemAvailable:") {
			fmt.Sscanf(line, "MemAvailable: %f kB", &val)
			avail = val / 1024
		}
		if strings.HasPrefix(line, "Cached:") {
			fmt.Sscanf(line, "Cached: %f kB", &val)
			cached = val / 1024
		}
	}
	return
}


// InitMemoryTracker handles the InitMemoryTracker HTTP request.
func InitMemoryTracker() {
	memHistory = []memSample{}
	total, _, _, _, _ := readMemDetail()
	if total == 0 {
		total = 16000 // fallback 16GB
	}
	// Immediate sample
	if t, _, f, _, err := readMemDetail(); err == nil {
		used := t - f
		memHistory = append(memHistory, memSample{
			Time:    time.Now(),
			TotalMB: t,
			UsedMB:  used,
			FreeMB:  f,
			Pct:     used / t * 100,
		})
	}
	go func() {
		for {
			time.Sleep(60 * time.Second)
			t, _, f, _, err := readMemDetail()
			if err != nil || t == 0 {
				continue
			}
			used := t - f
			memMu.Lock()
			memHistory = append(memHistory, memSample{
				Time:    time.Now(),
				TotalMB: t,
				UsedMB:  used,
				FreeMB:  f,
				Pct:     used / t * 100,
			})
			if len(memHistory) > 60 {
				memHistory = memHistory[len(memHistory)-60:]
			}
			memMu.Unlock()
		}
	}()
}

// MemoryTrendHandler returns memory usage history.
// GET /api/admin/metrics/memory-trend
func MemoryTrendHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		memMu.RLock()
		total := float64(0)
		hist := []map[string]any{}
		for _, s := range memHistory {
			if total == 0 && s.TotalMB > 0 {
				total = s.TotalMB
			}
			hist = append(hist, map[string]any{
				"time":     s.Time.Format(time.RFC3339),
				"used_mb":  fmt.Sprintf("%.0f", s.UsedMB),
				"free_mb":  fmt.Sprintf("%.0f", s.FreeMB),
				"total_mb": fmt.Sprintf("%.0f", s.TotalMB),
				"pct":      fmt.Sprintf("%.1f", s.Pct),
			})
		}
		current := float64(0)
		currentPct := float64(0)
		if len(memHistory) > 0 {
			last := memHistory[len(memHistory)-1]
			current = last.UsedMB
			currentPct = last.Pct
		}
		memMu.RUnlock()

		// Also read /proc/meminfo live for cached
		_, _, _, cached, _ := readMemDetail()

		writeJSON(w, map[string]any{
			"ok":            true,
			"total_mb":      fmt.Sprintf("%.0f", total),
			"current_used_mb": fmt.Sprintf("%.0f", current),
			"current_pct":   fmt.Sprintf("%.1f", currentPct),
			"cached_mb":     fmt.Sprintf("%.0f", cached),
			"history":       hist,
			"samples":       len(hist),
			"sample_interval_s": 60,
		})
	}
}

// ── Port Scan Detection (I9) ──────────────────────────────────────────────────

var allowedPorts = []int{
	22,    // SSH
	53,    // systemd-resolved DNS
	80,    // HTTP redirect
	443,   // HTTPS
	631,   // CUPS printing
	8080,  // simplex-node API
	8888,  // Dashboard
	17225, // SimpleX bridge
	17001, // DC P2P transport
	9050,  // Tor SOCKS
	10810, // V2Ray SOCKS
	10808, // V2Ray gRPC API
	10809, // V2Ray gRPC
	5349,  // coturn TLS
	3478,  // coturn
	8443,  // SMP relay
	4433,  // XFTP
	5223,  // SMP relay (alt)
	5224,  // SMP relay (alt)
	5225,  // SMP relay (alt)
	5226,  // SMP relay (alt)
	5230,  // SMP relay (alt)
	11434, // Ollama API
	2018,  // acestream HTTP
	33827, // ephemeral internal
}

// PortScanHandler checks for unexpected open ports (potential intrusion).
// GET /api/admin/port-scan
func PortScanHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		openPorts := map[int]string{}
		// Read TCP listening ports from /proc/net/tcp
		data, err := os.ReadFile("/proc/net/tcp")
		if err == nil {
			for _, line := range strings.Split(string(data), "\n")[1:] {
				fields := strings.Fields(line)
				if len(fields) < 4 {
					continue
				}
				// Field 3: local_address in hex "00000000:XXXX"
				parts := strings.Split(fields[1], ":")
				if len(parts) < 2 {
					continue
				}
				// Port is hex after colon
				var port int
				fmt.Sscanf(parts[1], "%X", &port)
				// State 0A = LISTEN
				if fields[3] == "0A" && port > 0 {
					openPorts[port] = "tcp"
				}
			}
		}
		// Also check TCP6
		data6, err := os.ReadFile("/proc/net/tcp6")
		if err == nil {
			for _, line := range strings.Split(string(data6), "\n")[1:] {
				fields := strings.Fields(line)
				if len(fields) < 4 {
					continue
				}
				parts := strings.Split(fields[1], ":")
				if len(parts) < 2 {
					continue
				}
				var port int
				fmt.Sscanf(parts[1], "%X", &port)
				if fields[3] == "0A" && port > 0 {
					openPorts[port] = "tcp6"
				}
			}
		}

		allowed := map[int]bool{}
		for _, p := range allowedPorts {
			allowed[p] = true
		}

		unexpected := []int{}
		expected := []int{}
		for port := range openPorts {
			if allowed[port] {
				expected = append(expected, port)
			} else {
				unexpected = append(unexpected, port)
			}
		}
		sort.Ints(unexpected)
		sort.Ints(expected)

		severity := "clear"
		if len(unexpected) > 0 {
			if len(unexpected) >= 3 {
				severity = "critical"
			} else {
				severity = "warning"
			}
		}

		writeJSON(w, map[string]any{
			"ok":          true,
			"severity":    severity,
			"total_open":  len(openPorts),
			"expected":    expected,
			"unexpected":  unexpected,
			"tip":         fmt.Sprintf("Found %d expected ports, %d unexpected", len(expected), len(unexpected)),
		})
	}
}

// ── Service Dependency Check (I11) ───────────────────────────────────────────

// ServiceDepsHandler checks that critical services are running and their
// dependencies are satisfied.
// GET /api/admin/service-deps
func ServiceDepsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		type dep struct {
			Name         string `json:"name"`
			Status       string `json:"status"`
			DependsOn    string `json:"depends_on,omitempty"`
			DepSatisfied bool   `json:"dep_satisfied"`
		}
		results := []dep{}
		serviceStatus := map[string]bool{}

		// Check each service
		checks := []struct {
			name      string
			addr      string
			dependsOn string
		}{
			{"tor", "127.0.0.1:9050", ""},
			{"xray", "127.0.0.1:10810", ""},
			{"simplex-bridge", "127.0.0.1:17225", "tor"},
			{"dc-transport", "127.0.0.1:17001", ""},
			{"ollama", "127.0.0.1:11434", ""},
		}

		for _, c := range checks {
			conn, err := net.DialTimeout("tcp", c.addr, 3*time.Second)
			status := "down"
			if err == nil {
				conn.Close()
				status = "up"
				serviceStatus[c.name] = true
			}
			depSat := true
			if c.dependsOn != "" {
				depSat = serviceStatus[c.dependsOn]
			}
			results = append(results, dep{
				Name:         c.name,
				Status:       status,
				DependsOn:    c.dependsOn,
				DepSatisfied: depSat,
			})
		}

		// Determine overall health
		criticalFailing := 0
		for _, r := range results {
			if r.Status == "down" {
				criticalFailing++
			}
		}

		overall := "healthy"
		if criticalFailing > 0 {
			overall = "degraded"
			if criticalFailing >= 2 {
				overall = "critical"
			}
		}

		writeJSON(w, map[string]any{
			"ok":             true,
			"overall":        overall,
			"services":       results,
			"healthy_count":  len(results) - criticalFailing,
			"failing_count":  criticalFailing,
			"total_services": len(results),
		})
	}
}

// ── DNS over Tor Check (I12) ─────────────────────────────────────────────────

// DNSCheckHandler checks DNS configuration and Tor DNS availability.
// GET /api/admin/dns-check
func DNSCheckHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		checks := []map[string]any{}
		// 1. Check systemd-resolved
		resolvedOk := false
		if data, err := os.ReadFile("/etc/resolv.conf"); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				if strings.Contains(line, "127.0.0.53") {
					resolvedOk = true
					break
				}
			}
		}
		checks = append(checks, map[string]any{
			"name":    "systemd-resolved",
			"status":  resolvedOk,
			"detail":  "127.0.0.53 stub resolver",
			"healthy": resolvedOk,
		})

		// 2. Check DNSSEC/DNSOverTLS config
		dnsOverTLS := false
		dnssec := false
		if data, err := os.ReadFile("/etc/systemd/resolved.conf"); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				trim := strings.TrimSpace(line)
				if strings.HasPrefix(trim, "DNSOverTLS=") && strings.Contains(trim, "yes") {
					dnsOverTLS = true
				}
				if strings.HasPrefix(trim, "DNSSEC=") && strings.Contains(trim, "yes") {
					dnssec = true
				}
			}
		}
		checks = append(checks, map[string]any{
			"name":    "dns_over_tls",
			"status":  dnsOverTLS,
			"detail":  "DNS-over-TLS encryption",
			"healthy": true, // not critical
		})
		checks = append(checks, map[string]any{
			"name":    "dnssec",
			"status":  dnssec,
			"detail":  "DNSSEC validation",
			"healthy": true,
		})

		// 3. Check Tor DNS (DNSPort 53 on localhost)
		torDNS := false
		conn, err := net.DialTimeout("tcp", "127.0.0.1:9050", 2*time.Second)
		if err == nil {
			conn.Close()
			torDNS = true
		}
		checks = append(checks, map[string]any{
			"name":    "tor_socks",
			"status":  torDNS,
			"detail":  "Tor SOCKS proxy (:9050) — can route DNS via Tor",
			"healthy": torDNS,
			"tip":     "Configure Tor DNSPort to enable DNS-over-Tor",
		})

		// 4. DNS resolution test
		resolveOk := false
		addrs, err := net.LookupHost("stmaria.org")
		if err == nil && len(addrs) > 0 {
			resolveOk = true
		}
		checks = append(checks, map[string]any{
			"name":    "dns_resolution",
			"status":  resolveOk,
			"detail":  fmt.Sprintf("stmaria.org resolves to %v", addrs),
			"healthy": resolveOk,
		})

		allHealthy := true
		for _, c := range checks {
			if h, ok := c["healthy"].(bool); ok && !h {
				allHealthy = false
			}
		}

		writeJSON(w, map[string]any{
			"ok":         true,
			"all_healthy": allHealthy,
			"checks":     checks,
			"tip":        "For full anonymity, configure DNSPort 5353 in torrc and point resolv.conf to Tor DNS",
		})
	}
}

// ── Infrastructure Audit (I13/I14/I15) ───────────────────────────────────────

// InfraAuditHandler returns firewall, NTP, and swap status.
// GET /api/admin/infra-audit
func InfraAuditHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result := map[string]any{}
		now := time.Now()

		// ── I13: Firewall ─────────────────────────────────────
		fw := map[string]any{}
		// Check iptables
		out, err := exec.Command("sudo", "-n", "iptables", "-L", "-n", "--line-numbers").CombinedOutput()
		if err == nil {
			lines := strings.Split(string(out), "\n")
			// Count rules per chain
			chains := map[string]int{}
			currentChain := ""
			for _, line := range lines {
				trim := strings.TrimSpace(line)
				if strings.HasPrefix(trim, "Chain ") {
					parts := strings.Fields(trim)
					if len(parts) > 1 {
						currentChain = parts[1]
					}
				} else if len(trim) > 0 && trim[0] >= '0' && trim[0] <= '9' {
					chains[currentChain]++
				}
			}
			fw["iptables"] = "active"
			fw["rules"] = chains
		} else {
			fw["iptables"] = "inactive or no sudo"
		}
		// Check UFW
		ufwOut, _ := exec.Command("ufw", "status").CombinedOutput()
		if strings.Contains(string(ufwOut), "active") || strings.Contains(string(ufwOut), "Status: active") {
			fw["ufw"] = "active"
		} else if strings.Contains(string(ufwOut), "Status: inactive") {
			fw["ufw"] = "inactive"
		} else {
			fw["ufw"] = "not installed"
		}
		result["firewall"] = fw

		// ── I14: NTP Sync ─────────────────────────────────────
		ntp := map[string]any{}
		// Check timedatectl for NTP sync
		tdOut, _ := exec.Command("timedatectl", "show", "--property=NTPSynchronized", "--value").CombinedOutput()
		ntpSynced := strings.TrimSpace(string(tdOut)) == "yes"
		ntp["ntp_synchronized"] = ntpSynced
		// Check systemd-timesyncd status
		tsOut, _ := exec.Command("timedatectl", "show", "--property=TimeUSec", "--value").CombinedOutput()
		ntp["system_time"] = strings.TrimSpace(string(tsOut))
		// Offset from /proc/driver/rtc or systemd-timesyncd
		if b, err := os.ReadFile("/sys/class/rtc/rtc0/time"); err == nil {
			rtcStr := strings.TrimSpace(string(b))
			if rt, err := time.Parse("15:04:05", rtcStr); err == nil {
				diff := now.Sub(time.Date(now.Year(), now.Month(), now.Day(), rt.Hour(), rt.Minute(), rt.Second(), 0, now.Location()))
				if diff < 0 {
					diff = -diff
				}
				ntp["rtc_offset_seconds"] = int(diff.Seconds())
			}
		}
		// Check if timesyncd is running
		syncdOut, _ := exec.Command("systemctl", "--user", "is-active", "systemd-timesyncd").CombinedOutput()
		_ = syncdOut
		// systemd-timesyncd is a system service, not user
		sysOut, _ := exec.Command("systemctl", "is-active", "systemd-timesyncd").CombinedOutput()
		ntp["timesyncd_active"] = strings.TrimSpace(string(sysOut)) == "active"
		result["ntp"] = ntp

		// ── I15: Swap & Cache ─────────────────────────────────
		swap := map[string]any{}
		if data, err := os.ReadFile("/proc/meminfo"); err == nil {
			swapTotal := float64(0)
			swapFree := float64(0)
			cached := float64(0)
			sReclaimable := float64(0)

			for _, line := range strings.Split(string(data), "\n") {
				var val float64
				if strings.HasPrefix(line, "SwapTotal:") {
					fmt.Sscanf(line, "SwapTotal: %f kB", &val)
					swapTotal = val / 1024
				}
				if strings.HasPrefix(line, "SwapFree:") {
					fmt.Sscanf(line, "SwapFree: %f kB", &val)
					swapFree = val / 1024
				}
				if strings.HasPrefix(line, "Cached:") {
					fmt.Sscanf(line, "Cached: %f kB", &val)
					cached = val / 1024
				}
				if strings.HasPrefix(line, "SReclaimable:") {
					fmt.Sscanf(line, "SReclaimable: %f kB", &val)
					sReclaimable = val / 1024
				}
			}
			swapUsed := swapTotal - swapFree
			swap["total_mb"] = fmt.Sprintf("%.0f", swapTotal)
			swap["used_mb"] = fmt.Sprintf("%.0f", swapUsed)
			swap["free_mb"] = fmt.Sprintf("%.0f", swapFree)
			swap["cached_mb"] = fmt.Sprintf("%.0f", cached)
			swap["sreclaimable_mb"] = fmt.Sprintf("%.0f", sReclaimable)
			if swapTotal > 0 {
				swap["pct"] = fmt.Sprintf("%.1f", swapUsed/swapTotal*100)
			} else {
				swap["pct"] = "0"
			}
			swap["tip"] = "High swap usage indicates memory pressure; cache + SReclaimable can be freed if needed"
		} else {
			swap["error"] = err.Error()
		}
		result["swap"] = swap

		result["ok"] = true
		result["timestamp"] = now.Format(time.RFC3339)
		writeJSON(w, result)
	}
}

// ── Full Infrastructure Audit (I16) ──────────────────────────────────────────

// FullAuditHandler runs all infrastructure checks in one response.
// GET /api/admin/full-audit
func FullAuditHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		results := map[string]any{}
		now := time.Now()

		// 1. System info
		hostname, _ := os.Hostname()
		uptime := readUptime()
		l1, l5, l15 := readLoadAvg()
		ramTotal, ramAvail := readMemInfo()
		ramPct := float64(0)
		if ramTotal > 0 {
			ramPct = float64(ramTotal-ramAvail) / float64(ramTotal) * 100
		}
		results["system"] = map[string]any{
			"hostname":    hostname,
			"uptime_sec":  uptime,
			"load_1m":     l1,
			"load_5m":     l5,
			"load_15m":    l15,
			"ram_total_mb": ramTotal / 1024,
			"ram_used_pct": fmt.Sprintf("%.1f", ramPct),
		}

		// 2. Disk
		disk := fs{}
		diskUsage("/", &disk)
		results["disk"] = map[string]any{
			"total_gb":  fmt.Sprintf("%.1f", float64(disk.Total)/1024/1024/1024),
			"used_gb":   fmt.Sprintf("%.1f", float64(disk.Used)/1024/1024/1024),
			"free_gb":   fmt.Sprintf("%.1f", float64(disk.Avail)/1024/1024/1024),
			"used_pct":  fmt.Sprintf("%.1f", float64(disk.Used)/float64(disk.Total)*100),
		}

		// 3. Services
		svcs := []map[string]any{}
		for _, s := range []struct{ name, addr string }{
			{"tor", "127.0.0.1:9050"},
			{"xray", "127.0.0.1:10810"},
			{"bridge", "127.0.0.1:17225"},
			{"dc_p2p", "127.0.0.1:17001"},
			{"ollama", "127.0.0.1:11434"},
		} {
			conn, err := net.DialTimeout("tcp", s.addr, 2*time.Second)
			status := "down"
			if err == nil {
				conn.Close()
				status = "up"
			}
			svcs = append(svcs, map[string]any{"name": s.name, "status": status, "address": s.addr})
		}
		results["services"] = svcs

		// 4. Open ports (summary)
		openPorts := map[int]string{}
		for _, procFile := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
			data, err := os.ReadFile(procFile)
			if err != nil {
				continue
			}
			for _, line := range strings.Split(string(data), "\n")[1:] {
				fields := strings.Fields(line)
				if len(fields) < 4 || fields[3] != "0A" {
					continue
				}
				parts := strings.Split(fields[1], ":")
				if len(parts) >= 2 {
					var port int
					fmt.Sscanf(parts[1], "%X", &port)
					if port > 0 {
						openPorts[port] = "tcp"
					}
				}
			}
		}
		results["open_ports_count"] = len(openPorts)

		// 5. DNS
		addrs, _ := net.LookupHost("stmaria.org")
		results["dns_resolves"] = len(addrs) > 0

		// 6. NTP
		tdOut, _ := exec.Command("timedatectl", "show", "--property=NTPSynchronized", "--value").CombinedOutput()
		results["ntp_synced"] = strings.TrimSpace(string(tdOut)) == "yes"

		results["ok"] = true
		results["timestamp"] = now.Format(time.RFC3339)
		writeJSON(w, results)
	}
}

// ── Snap→Native Check (I17) ─────────────────────────────────────────────────

// SnapMigrationHandler checks for snap packages that could be native.
// GET /api/admin/snap-check
func SnapMigrationHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		installed := []map[string]any{}
		out, err := exec.Command("snap", "list").CombinedOutput()
		if err != nil {
			writeJSON(w, map[string]any{"ok": true, "snap_installed": false, "total": 0, "packages": []string{}, "tip": "snap not available"})
			return
		}
		lines := strings.Split(string(out), "\n")
		for _, line := range lines[1:] {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				name := fields[0]
				ver := fields[1]
				replaceable := false
				nativeCmd := ""
				switch name {
				case "core", "core20", "core22", "core24", "snapd", "bare":
					continue // skip snap infrastructure
				case "firefox":
					replaceable = true
					nativeCmd = "apt install firefox"
				case "chromium":
					replaceable = true
					nativeCmd = "apt install chromium-browser"
				case "gnome-3-38-2004", "gnome-42-2204", "gtk-common-themes":
					continue // snap dependencies
				default:
					replaceable = false
				}
				installed = append(installed, map[string]any{
					"name":        name,
					"version":     ver,
					"replaceable": replaceable,
					"native_cmd":  nativeCmd,
				})
			}
		}
		total := len(installed)
		replaceable := 0
		for _, p := range installed {
			if p["replaceable"].(bool) {
				replaceable++
			}
		}
		writeJSON(w, map[string]any{
			"ok":          true,
			"snap_installed": true,
			"total":       total,
			"replaceable": replaceable,
			"packages":    installed,
			"tip":         fmt.Sprintf("%d/%d snap packages can be replaced with native versions", replaceable, total),
		})
	}
}

// ── Systemd Service Hardening (I18) ──────────────────────────────────────────

// ServiceHardeningHandler checks systemd service security settings.
// GET /api/admin/service-hardening
func ServiceHardeningHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		home := os.Getenv("HOME")
		serviceDir := filepath.Join(home, ".config", "systemd", "user")
		results := []map[string]any{}

		entries, err := os.ReadDir(serviceDir)
		if err != nil {
			writeJSON(w, map[string]any{"ok": true, "services": []map[string]any{}, "tip": "no user services found", "healthy": true})
			return
		}

		for _, e := range entries {
			if !strings.HasSuffix(e.Name(), ".service") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(serviceDir, e.Name()))
			if err != nil {
				continue
			}
			content := string(data)
			svc := map[string]any{
				"name":       e.Name(),
				"has_service_block": strings.Contains(content, "[Service]"),
			}
			checks := []map[string]any{}
			// Check security directives
			directives := []struct {
				name   string
				found  bool
				expect string
				reason string
			}{
				{"ProtectHome", strings.Contains(content, "ProtectHome="), "ProtectHome=yes", "prevents access to /home"},
				{"PrivateTmp", strings.Contains(content, "PrivateTmp="), "PrivateTmp=yes", "isolates /tmp"},
				{"NoNewPrivileges", strings.Contains(content, "NoNewPrivileges="), "NoNewPrivileges=yes", "prevents privilege escalation"},
				{"ProtectSystem", strings.Contains(content, "ProtectSystem="), "ProtectSystem=strict", "read-only /usr"},
				{"ProtectKernelModules", strings.Contains(content, "ProtectKernelModules="), "ProtectKernelModules=yes", "prevents kernel module loading"},
				{"MemoryDenyWriteExecute", strings.Contains(content, "MemoryDenyWriteExecute="), "MemoryDenyWriteExecute=yes", "prevents JIT spraying"},
			}
			for _, d := range directives {
				checks = append(checks, map[string]any{
					"directive": d.name,
					"present":   d.found,
					"expected":  d.expect,
					"reason":    d.reason,
					"secure":    d.found,
				})
			}
			svc["checks"] = checks
			svc["secure_count"] = countSecure(checks)
			svc["total_checks"] = len(checks)
			results = append(results, svc)
		}
		writeJSON(w, map[string]any{
			"ok":       true,
			"services": results,
			"tip":      "Add hardening directives to [Service] section of each unit file",
		})
	}
}

func countSecure(checks []map[string]any) int {
	n := 0
	for _, c := range checks {
		if s, ok := c["secure"].(bool); ok && s {
			n++
		}
	}
	return n
}

// ── Kernel Parameter Tuning (I19) ────────────────────────────────────────────

// KernelTuningHandler checks sysctl security/performance settings.
// GET /api/admin/kernel-tuning
func KernelTuningHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		checks := []map[string]any{}

		params := []struct {
			name     string
			key      string
			expected string
			reason   string
		}{
			{"IP Forwarding", "net.ipv4.ip_forward", "0", "should be 0 unless routing"},
			{"Reverse Path Filter", "net.ipv4.conf.all.rp_filter", "1", "prevents IP spoofing"},
			{"TCP SYN Cookies", "net.ipv4.tcp_syncookies", "1", "protects against SYN flood"},
			{"ICMP Redirects", "net.ipv4.conf.all.accept_redirects", "0", "prevents MITM via ICMP"},
			{"Secure ICMP Redirects", "net.ipv4.conf.all.secure_redirects", "0", "prevents MITM via secure ICMP"},
			{"Source Route Packets", "net.ipv4.conf.all.accept_source_route", "0", "prevents source routing attacks"},
			{"Kernel Exec Shield", "kernel.exec-shield", "1", "ASLR for executables"},
			{"Kernel Randomize", "kernel.randomize_va_space", "2", "full ASLR"},
		}

		for _, p := range params {
			out, err := exec.Command("sysctl", "-n", p.key).CombinedOutput()
			val := strings.TrimSpace(string(out))
			healthy := err == nil && val == p.expected
			checks = append(checks, map[string]any{
				"name":     p.name,
				"key":      p.key,
				"current":  val,
				"expected": p.expected,
				"healthy":  healthy,
				"reason":   p.reason,
			})
		}

		healthyCount := 0
		for _, c := range checks {
			if h, ok := c["healthy"].(bool); ok && h {
				healthyCount++
			}
		}

		writeJSON(w, map[string]any{
			"ok":            true,
			"checks":        checks,
			"healthy":       healthyCount,
			"total":         len(checks),
			"all_healthy":   healthyCount == len(checks),
			"tip":           "Add recommended values to /etc/sysctl.d/99-custom.conf",
		})
	}
}

// ── Backup Verify Cron (I20) ─────────────────────────────────────────────────

var backupVerifyMu sync.Mutex
var lastBackupVerify time.Time

// USBBackupVerifyHandler verifies USB backup integrity.
// GET /api/admin/backup-verify
func USBBackupVerifyHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		backupVerifyMu.Lock()
		defer backupVerifyMu.Unlock()

		usbDir := "/run/media/tomas/SIMPLEX-USB"
		entries, err := os.ReadDir(usbDir)
		if err != nil {
			writeJSON(w, map[string]any{"ok": true, "usb_mounted": false, "error": err.Error()})
			return
		}

		// Find latest .tar and .gitbundle
		latestTar := ""
		latestTarTime := time.Time{}
		latestBundle := ""
		latestBundleTime := time.Time{}

		for _, e := range entries {
			info, err := e.Info()
			if err != nil {
				continue
			}
			if strings.HasSuffix(e.Name(), ".tar") && info.ModTime().After(latestTarTime) {
				latestTar = e.Name()
				latestTarTime = info.ModTime()
			}
			if strings.HasSuffix(e.Name(), ".gitbundle") && info.ModTime().After(latestBundleTime) {
				latestBundle = e.Name()
				latestBundleTime = info.ModTime()
			}
		}

		tarOk := false
		bundleOk := false
		var tarSize int64
		var bundleSize int64
		var verifyErrors []string

		// Verify tar integrity
		if latestTar != "" {
			if fi, err := os.Stat(filepath.Join(usbDir, latestTar)); err == nil {
				tarSize = fi.Size()
			}
			out, err := exec.Command("tar", "-tzf", filepath.Join(usbDir, latestTar)).CombinedOutput()
			if err == nil && strings.Contains(string(out), "chat_history.json") {
				tarOk = true
			} else {
				verifyErrors = append(verifyErrors, fmt.Sprintf("tar integrity check failed: %s", latestTar))
			}
		}

		// Verify gitbundle integrity
		if latestBundle != "" {
			if fi, err := os.Stat(filepath.Join(usbDir, latestBundle)); err == nil {
				bundleSize = fi.Size()
			}
			out, err := exec.Command("git", "bundle", "verify", filepath.Join(usbDir, latestBundle)).CombinedOutput()
			if err == nil {
				bundleOk = true
			} else {
				verifyErrors = append(verifyErrors, fmt.Sprintf("gitbundle verify failed: %s - %s", latestBundle, string(out)))
			}
		}

		lastBackupVerify = time.Now()

		writeJSON(w, map[string]any{
			"ok":             true,
			"usb_mounted":    true,
			"latest_tar":     latestTar,
			"tar_size_mb":    fmt.Sprintf("%.1f", float64(tarSize)/1024/1024),
			"tar_valid":      tarOk,
			"latest_bundle":  latestBundle,
			"bundle_size_mb": fmt.Sprintf("%.1f", float64(bundleSize)/1024/1024),
			"bundle_valid":   bundleOk,
			"all_valid":      tarOk && bundleOk,
			"verify_errors":  verifyErrors,
			"last_verify":    lastBackupVerify.Format(time.RFC3339),
		})
	}
}

func diskUsage(path string, d *fs) error {
	cmd := exec.Command("df", "-B1", path)
	out, err := cmd.Output()
	if err != nil {
		return err
	}
	lines := strings.Split(string(out), "\n")
	if len(lines) < 2 {
		return fmt.Errorf("no data")
	}
	fields := strings.Fields(lines[1])
	if len(fields) < 4 {
		return fmt.Errorf("unexpected format")
	}
	d.Total = atou64(fields[1])
	d.Used = atou64(fields[2])
	d.Avail = atou64(fields[3])
	return nil
}

func atou64(s string) uint64 {
	v, _ := strconv.ParseUint(s, 10, 64)
	return v
}

// ── System Update Checker (Phase III-I5) ──────────────────────────────────

// UpdateCheckHandler checks for available system updates.
// GET /api/admin/updates
func UpdateCheckHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result := map[string]any{
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		}
		// Run commands without proxy
		cmd := func(name string, args ...string) ([]byte, error) {
			c := exec.Command(name, args...)
			c.Env = append(os.Environ(), "HTTP_PROXY=", "HTTPS_PROXY=", "ALL_PROXY=", "http_proxy=", "https_proxy=", "all_proxy=")
			return c.CombinedOutput()
		}
		// Check apt updates
		out, err := cmd("apt-get", "-s", "upgrade")
		if err == nil {
			output := string(out)
			upgradable := 0
			security := 0
			for _, line := range strings.Split(output, "\n") {
				if strings.Contains(line, "upgraded") && strings.Contains(line, "newly installed") {
					fmt.Sscanf(line, "%d upgraded", &upgradable)
				}
				if strings.Contains(line, "security") {
					security++
				}
			}
			result["apt_upgradable"] = upgradable
			result["apt_security"] = security
			if upgradable > 0 {
				result["apt_detail"] = fmt.Sprintf("%d packages can be upgraded (%d security)", upgradable, security)
			} else {
				result["apt_detail"] = "system is up to date"
			}
		} else {
			result["apt_error"] = err.Error()
		}
		// Check Go version
		goOut, _ := cmd("go", "version")
		result["go_version"] = strings.TrimSpace(string(goOut))
		// Check Docker version
		dockerOut, _ := cmd("docker", "version", "--format", "{{.Server.Version}}")
		if v := strings.TrimSpace(string(dockerOut)); v != "" {
			result["docker_version"] = v
		}
		// Kernel version
		kernelOut, _ := cmd("uname", "-r")
		result["kernel"] = strings.TrimSpace(string(kernelOut))
		// Uptime
		if b, err := os.ReadFile("/proc/uptime"); err == nil {
			parts := strings.Fields(string(b))
			if len(parts) > 0 {
				if secs, err := strconv.ParseFloat(parts[0], 64); err == nil {
					result["uptime_hours"] = fmt.Sprintf("%.1f", secs/3600)
				}
			}
		}
		writeJSON(w, result)
	}
}

// ── SSE Real-Time Events (Cycle 58) ───────────────────────────────────────────

// EventsHandler streams system metrics, Docker status, and bridge status via SSE.
func EventsHandler() http.HandlerFunc {
	startTime := time.Now()
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		ticker5s := time.NewTicker(5 * time.Second)
		ticker30s := time.NewTicker(30 * time.Second)
		defer ticker5s.Stop()
		defer ticker30s.Stop()

		sendEvent := func(event string, data map[string]any) {
			b, _ := json.Marshal(data)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, string(b))
			flusher.Flush()
		}

		// Send initial events immediately
		sendEvent("system", CollectSystemMetrics(startTime))
		sendEvent("docker", dockerStatus())
		sendEvent("bridge", map[string]any{"connected": BridgeConnected})

		for {
			select {
			case <-ticker5s.C:
				sendEvent("system", CollectSystemMetrics(startTime))
			case <-ticker30s.C:
				sendEvent("docker", dockerStatus())
				sendEvent("bridge", map[string]any{"connected": BridgeConnected})
			case <-r.Context().Done():
				return
			}
		}
	}
}

func CollectSystemMetrics(startTime time.Time) map[string]any {
	m := runtime.MemStats{}
	runtime.ReadMemStats(&m)

	uptime := time.Since(startTime).Seconds()
	ramTotal, ramAvail := readMemInfo()
	l1, _, _ := readLoadAvg()
	cpuPct := l1 * 100 / float64(runtime.NumCPU())
	if cpuPct > 100 {
		cpuPct = 100
	}

	metrics := map[string]any{
		"uptime_seconds":   int(uptime),
		"uptime_human":     fmt.Sprintf("%dh%dm", int(uptime)/3600, int(uptime)%3600/60),
		"cpu_percent":      cpuPct,
		"memory": map[string]any{
			"alloc_mb":       m.Alloc / 1024 / 1024,
			"total_alloc_mb": m.TotalAlloc / 1024 / 1024,
			"sys_mb":         m.Sys / 1024 / 1024,
			"heap_mb":        m.HeapAlloc / 1024 / 1024,
			"num_gc":         m.NumGC,
		},
		"goroutines": runtime.NumGoroutine(),
		"cpus":       runtime.NumCPU(),
		"go_version": runtime.Version(),
	}
	if ramTotal > 0 {
		usedPct := float64(ramTotal-ramAvail) / float64(ramTotal) * 100
		metrics["ram"] = map[string]any{
			"total_mb":  ramTotal / 1024,
			"used_mb":   (ramTotal - ramAvail) / 1024,
			"avail_mb":  ramAvail / 1024,
			"used_pct":  fmt.Sprintf("%.1f%%", usedPct),
		}
	}
	// Root disk
	var disk fs
	if err := diskUsage("/", &disk); err == nil {
		metrics["disk"] = map[string]any{
			"total_gb": disk.Total / 1024 / 1024 / 1024,
			"used_gb":  disk.Used / 1024 / 1024 / 1024,
			"avail_gb": disk.Avail / 1024 / 1024 / 1024,
			"used_pct": fmt.Sprintf("%.1f%%", float64(disk.Used)/float64(disk.Total)*100),
		}
	}
	return metrics
}

// MetricsStreamHandler pushes system metrics every 5s via SSE.
func MetricsStreamHandler() http.HandlerFunc {
	startTime := time.Now()
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		sendMetrics := func() {
			m := runtime.MemStats{}
			runtime.ReadMemStats(&m)
			var disk fs
			diskUsage("/", &disk)
			cpuPct := getCPUPercent()
			data := map[string]any{
				"cpu_percent":    cpuPct,
				"ram_percent":    getRAMPercent(),
				"ram_used_mb":    m.Alloc / 1024 / 1024,
				"ram_total_mb":   getRAMTotalMB(),
				"disk_percent":   fmt.Sprintf("%.1f", float64(disk.Used)/float64(disk.Total)*100),
				"disk_used_gb":   float64(disk.Used) / 1024 / 1024 / 1024,
				"disk_total_gb":  float64(disk.Total) / 1024 / 1024 / 1024,
				"uptime_seconds": int(time.Since(startTime).Seconds()),
				"goroutines":     runtime.NumGoroutine(),
			}
			b, _ := json.Marshal(data)
			fmt.Fprintf(w, "event: metrics\ndata: %s\n\n", string(b))
			flusher.Flush()
		}

		sendMetrics()
		for {
			select {
			case <-ticker.C:
				sendMetrics()
			case <-r.Context().Done():
				return
			}
		}
	}
}

func getCPUPercent() float64 {
	cmd := exec.Command("sh", "-c", "top -bn1 | grep 'Cpu(s)' | awk '{print $2}' | cut -d'%' -f1")
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	v, _ := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	return v
}

func getRAMPercent() float64 {
	cmd := exec.Command("sh", "-c", "free | grep Mem | awk '{print $3/$2 * 100.0}'")
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	v, _ := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	return v
}

func getRAMTotalMB() float64 {
	cmd := exec.Command("sh", "-c", "free -m | grep Mem | awk '{print $2}'")
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	v, _ := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	return v
}

// ── Real-Time Live Dashboard HTML (Cycle 58) ──────────────────────────────────

// LiveDashboardHandler serves a self-contained HTML page with SSE live metrics.
func LiveDashboardHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Live • simplex-node</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{background:#0d0d0f;color:#e2e8f0;font-family:system-ui,-apple-system,sans-serif;padding:20px;max-width:1200px;margin:0 auto}
h1{font-size:1.3rem;margin-bottom:20px;display:flex;align-items:center;gap:10px}
h1 span{font-size:0.75rem;color:#64748b}
.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(260px,1fr));gap:14px;margin-bottom:20px}
.card{background:#16161b;border-radius:12px;padding:16px;border:1px solid #1e1e24}
.card h3{font-size:0.8rem;color:#64748b;text-transform:uppercase;letter-spacing:0.05em;margin-bottom:8px}
.card .val{font-size:1.8rem;font-weight:700;font-variant-numeric:tabular-nums}
.card .sub{font-size:0.8rem;color:#94a3b8;margin-top:4px}
.ok{color:#22c55e}.warn{color:#eab308}.danger{color:#ef4444}
.containers{display:flex;flex-direction:column;gap:6px}
.ctn{display:flex;justify-content:space-between;align-items:center;padding:6px 10px;background:#1a1a20;border-radius:6px;font-size:0.85rem}
.ctn .name{font-family:ui-monospace,monospace;font-size:0.8rem}
.dot{width:8px;height:8px;border-radius:50%;display:inline-block;margin-right:6px}
.dot.up{background:#22c55e}.dot.down{background:#ef4444}.dot.unhealthy{background:#eab308}
.bridge{display:flex;align-items:center;gap:8px;padding:10px;border-radius:6px;font-size:0.9rem}
.bridge.on{background:#052e16;border:1px solid #166534}.bridge.off{background:#2e0505;border:1px solid #991b1b}
footer{text-align:center;margin-top:30px;font-size:0.75rem;color:#475569}
</style>
</head>
<body>
<h1>🔴 Live Dashboard <span id="uptime"></span></h1>
<div class="grid">
  <div class="card"><h3>CPU</h3><div class="val" id="cpu">—</div><div class="sub">goroutines: <span id="goroutines">—</span></div></div>
  <div class="card"><h3>Memory</h3><div class="val" id="mem">—</div><div class="sub">GC: <span id="gc">—</span></div></div>
  <div class="card"><h3>Disk</h3><div class="val" id="disk">—</div><div class="sub" id="disk_detail"></div></div>
  <div class="card"><h3>Bridge</h3><div class="bridge off" id="bridge">❌ Disconnected</div></div>
</div>
<div class="card">
  <h3>🐳 Containers</h3>
  <div class="containers" id="containers"><div style="color:#64748b;font-size:0.85rem">Waiting for data...</div></div>
</div>
<footer>simplex-node · live metrics update every 5s</footer>
<script>
const es=new EventSource("/api/admin/events");
es.addEventListener("system",e=>{
  const d=JSON.parse(e.data);
  document.getElementById("uptime").textContent="up "+Math.floor(d.uptime_seconds/60)+"m";
  document.getElementById("cpu").textContent=d.cpus+" cores";
  document.getElementById("goroutines").textContent=d.goroutines;
  document.getElementById("mem").textContent=(d.memory.alloc_mb||0)+" MB";
  document.getElementById("gc").textContent=d.memory.num_gc;
  if(d.disk){
    document.getElementById("disk").textContent=d.disk.used_gb+" GB / "+d.disk.total_gb+" GB";
    document.getElementById("disk").className="val "+(d.disk.used_pct>90?"danger":d.disk.used_pct>75?"warn":"ok");
    document.getElementById("disk_detail").textContent=d.disk.used_pct+" used";
  }
});
es.addEventListener("docker",e=>{
  const d=JSON.parse(e.data);
  const el=document.getElementById("containers");
  if(!d.containers){el.innerHTML='<div style="color:#64748b">No data</div>';return}
  let html="";
  for(const[name,st]of Object.entries(d.containers)){
    const cls=st.includes("healthy")?"up":st.includes("unhealthy")?"unhealthy":"down";
    html+='<div class="ctn"><span class="name"><span class="dot '+cls+'"></span>'+name+'</span><span>'+st+'</span></div>';
  }
  el.innerHTML=html;
});
es.addEventListener("bridge",e=>{
  const d=JSON.parse(e.data);
  const el=document.getElementById("bridge");
  if(d.connected){el.className="bridge on";el.innerHTML="🟢 Connected"}
  else{el.className="bridge off";el.innerHTML="🔴 Disconnected"}
});
es.onerror=()=>{document.querySelector("h1").innerHTML='🔴 Live Dashboard <span style="color:#ef4444">(reconnecting...)</span>'};
</script>
</body>
</html>`)
	}
}

// ── API Routes Listing (Cycle 59) ─────────────────────────────────────────────

// RoutesHandler returns a listing of all API routes with methods and descriptions.
func RoutesHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		routes := []map[string]string{
			{"path": "/api/version", "method": "GET", "desc": "Server version and build info"},
			{"path": "/api/health", "method": "GET", "desc": "Health check (bridge, uptime)"},
			// Chat
			{"path": "/api/chat/send", "method": "POST", "desc": "Send message to contact"},
			{"path": "/api/chat/history", "method": "GET", "desc": "Message history (chat_id=@N)"},
			{"path": "/api/chat/stream", "method": "GET", "desc": "SSE chat stream"},
			{"path": "/api/chat/contacts", "method": "GET", "desc": "Contact list"},
			{"path": "/api/chat/contact", "method": "GET", "desc": "Single contact (id=@N)"},
			{"path": "/api/chat/contact/alias", "method": "POST", "desc": "Set contact alias"},
			{"path": "/api/chat/contact/info", "method": "GET", "desc": "Contact info with stats"},
			{"path": "/api/chat/search", "method": "GET", "desc": "Search messages (q=, from=, to=)"},
			{"path": "/api/chat/edit", "method": "POST", "desc": "Edit message"},
			{"path": "/api/chat/delete", "method": "POST", "desc": "Delete message"},
			{"path": "/api/chat/clear", "method": "POST", "desc": "Clear all history"},
			{"path": "/api/chat/clear-old", "method": "POST", "desc": "Delete msgs older than N days"},
			{"path": "/api/chat/pin", "method": "POST", "desc": "Toggle message pin"},
			{"path": "/api/chat/react", "method": "POST", "desc": "Toggle emoji reaction"},
			{"path": "/api/chat/forward", "method": "POST", "desc": "Forward message"},
			{"path": "/api/chat/stats", "method": "GET", "desc": "Message statistics"},
			{"path": "/api/chat/status", "method": "GET", "desc": "Bridge health status"},
			{"path": "/api/chat/export", "method": "GET", "desc": "Export chat (format=json/html)"},
			{"path": "/api/chat/backup", "method": "GET/POST", "desc": "Backup/restore chat history"},
			{"path": "/api/chat/server-status", "method": "GET/POST", "desc": "Get/set server status message"},
			{"path": "/api/chat/broadcast", "method": "POST", "desc": "Broadcast to all contacts"},
			{"path": "/api/chat/last-message", "method": "GET", "desc": "Last message per contact"},
			{"path": "/api/chat/pay", "method": "POST", "desc": "Money transfer via chat"},
			{"path": "/api/chat/recall", "method": "POST", "desc": "Recall sent message"},
			{"path": "/api/chat/read-receipt", "method": "POST", "desc": "Set read receipt"},
			{"path": "/api/chat/ai", "method": "POST", "desc": "AI chat inline"},
			{"path": "/api/chat/voice", "method": "POST", "desc": "Send voice message"},
			{"path": "/api/chat/typing", "method": "POST", "desc": "Typing indicator"},
			{"path": "/api/chat/templates", "method": "GET/POST", "desc": "Message templates"},
			{"path": "/api/chat/auto-reply", "method": "GET/POST/DELETE", "desc": "Auto-reply rules"},
			{"path": "/api/chat/groups", "method": "GET/POST/DELETE", "desc": "Contact groups"},
			{"path": "/api/chat/labels", "method": "GET/POST", "desc": "Message labels"},
			{"path": "/api/chat/drafts", "method": "GET/POST", "desc": "Message drafts"},
			{"path": "/api/chat/webhook", "method": "GET/POST", "desc": "Webhook config"},
			{"path": "/api/chat/archive", "method": "POST", "desc": "Archive old messages to USB"},
			{"path": "/api/chat/schedule", "method": "POST", "desc": "Schedule message send"},
			{"path": "/api/chat/address/create", "method": "POST", "desc": "Create SimpleX address"},
			{"path": "/api/chat/qr", "method": "GET", "desc": "Get address QR code"},
			{"path": "/api/chat/connect", "method": "POST", "desc": "Connect to SimpleX contact"},
			{"path": "/api/chat/invoice/create", "method": "POST", "desc": "Create invoice"},
			{"path": "/api/chat/invoice/list", "method": "GET", "desc": "List invoices"},
			{"path": "/api/chat/invoice/pay", "method": "POST", "desc": "Pay invoice"},
			{"path": "/api/chat/invoice/stats", "method": "GET", "desc": "Invoice stats"},
			{"path": "/api/chat/invoice/export-csv", "method": "GET", "desc": "Export invoices CSV"},
			// Radio
			{"path": "/api/radio", "method": "GET", "desc": "Radio (action=stations/playlist/formula/m3u8)"},
			{"path": "/api/radio/acestep", "method": "GET/POST", "desc": "Acestep AI generator"},
			// Economy
			{"path": "/api/economy/oracle", "method": "GET", "desc": "Silver spot price oracle"},
			{"path": "/api/economy/rates", "method": "GET", "desc": "Multi-currency rates"},
			{"path": "/api/economy/dividend-admin", "method": "GET/POST", "desc": "Dividend history/trigger"},
			{"path": "/api/economy/treasury-forecast", "method": "GET", "desc": "Treasury health forecast"},
			{"path": "/api/economy/invoice-webhook-test", "method": "POST", "desc": "Test invoice webhook"},
			// Admin
			{"path": "/api/admin/info", "method": "GET", "desc": "Comprehensive node info"},
			{"path": "/api/admin/metrics", "method": "GET", "desc": "Detailed metrics"},
			{"path": "/api/admin/metrics/system", "method": "GET", "desc": "System metrics (CPU, RAM, disk)"},
			{"path": "/api/admin/events", "method": "GET", "desc": "SSE live events stream"},
			{"path": "/api/admin/live", "method": "GET", "desc": "Real-time dashboard HTML"},
			{"path": "/api/admin/docker", "method": "GET", "desc": "Docker container status"},
			{"path": "/api/admin/diagnostics", "method": "GET", "desc": "System diagnostics"},
			{"path": "/api/admin/backup", "method": "POST", "desc": "Trigger backup"},
			{"path": "/api/admin/audit-log", "method": "GET", "desc": "Audit log"},
			{"path": "/api/admin/status-page", "method": "GET", "desc": "Status page"},
			{"path": "/api/admin/rate-limit-status", "method": "GET", "desc": "Rate limiter status"},
			{"path": "/api/admin/rate-limit-config", "method": "GET/POST", "desc": "Rate limiter config"},
			{"path": "/api/admin/content-filter", "method": "GET", "desc": "Content filter config"},
			{"path": "/api/admin/webhook-queue", "method": "GET", "desc": "Webhook delivery queue"},
			{"path": "/api/admin/search-index", "method": "GET", "desc": "Search index status"},
			{"path": "/api/admin/routes", "method": "GET", "desc": "API routes listing"},
			{"path": "/api/admin/config", "method": "GET/POST", "desc": "Centralized threshold config"},
			{"path": "/api/admin/disk-cleanup", "method": "POST", "desc": "Run disk cleanup (docker prune, old logs, backups)"},
			{"path": "/api/admin/maintenance", "method": "GET/POST", "desc": "Maintenance mode toggle"},
			{"path": "/api/admin/ping", "method": "GET", "desc": "Watchdog-style health ping"},
			{"path": "/api/admin/backup/verify", "method": "POST", "desc": "Verify latest backup integrity"},
			{"path": "/api/admin/monitor-status", "method": "GET", "desc": "SSE monitor status stream"},
			{"path": "/api/health/checks", "method": "GET", "desc": "Detailed health check results"},
			// ParanoidX
			{"path": "/api/paranoidx/status", "method": "GET", "desc": "ParanoidX chain status"},
			{"path": "/api/paranoidx/config", "method": "GET", "desc": "Chain config"},
			{"path": "/api/paranoidx/chain/*", "method": "GET/POST", "desc": "Chain build/teardown/test"},
			{"path": "/api/paranoidx/vpn/*", "method": "GET/POST/DELETE", "desc": "VPN profile management"},
			// AI
			{"path": "/api/ai/chat", "method": "POST", "desc": "AI Steward chat"},
			{"path": "/api/ai/health", "method": "GET", "desc": "AI service health"},
			{"path": "/api/ai/economy-summary", "method": "POST", "desc": "Economy AI summary"},
			{"path": "/api/ai/moderation", "method": "POST", "desc": "Content moderation check"},
			{"path": "/api/ai/explain-silver", "method": "POST", "desc": "Silver standard explanation"},
			{"path": "/api/ai/suggest-treasury", "method": "POST", "desc": "Treasury action suggestions"},
			// Container
			{"path": "/api/container/*", "method": "GET/POST", "desc": "CryptoContainer lifecycle"},
			// Silver assets
			{"path": "/api/silver/mint", "method": "POST", "desc": "Mint silver-backed asset"},
			{"path": "/api/silver/burn", "method": "POST", "desc": "Burn silver-backed asset"},
			{"path": "/api/silver/list", "method": "GET", "desc": "List silver assets"},
			// Reserve
			{"path": "/api/reserve/proof", "method": "GET", "desc": "Proof of reserve"},
			// RWA
			{"path": "/api/rwa/register", "method": "POST", "desc": "Register RWA asset"},
			{"path": "/api/rwa/list", "method": "GET", "desc": "List RWA assets"},
			// DB
			{"path": "/api/db/*", "method": "GET/POST", "desc": "Database backup/restore"},
			// Other
			{"path": "/api/inquisitor/report", "method": "GET", "desc": "Inquisitor consolidator report"},
			{"path": "/api/metrics", "method": "GET", "desc": "Prometheus metrics"},
			{"path": "/api/panic", "method": "POST", "desc": "Emergency PANIC wipe"},
			{"path": "/api/relay/*", "method": "GET/POST", "desc": "Inter-node message relay"},
			{"path": "/api/did", "method": "GET", "desc": "Node DID document"},
			{"path": "/api/did/contact", "method": "GET", "desc": "Contact DID verification"},
			{"path": "/api/ai/steward-did", "method": "GET", "desc": "Steward AI agent DID"},
			{"path": "/api/radio/ai-content", "method": "POST", "desc": "AI radio content generation"},
			{"path": "/api/tracker/nodes", "method": "GET", "desc": "Network tracker nodes"},
			{"path": "/api/chat/bridge-health", "method": "GET", "desc": "Bridge health with latency"},
			{"path": "/api/chat/bridge-config", "method": "GET", "desc": "Bridge configuration"},
		}
		writeJSON(w, map[string]any{
			"total": len(routes),
			"routes": routes,
		})
	}
}

// ── Disk Usage Analyzer (Cycle 60) ────────────────────────────────────────────

// DiskUsageHandler reports disk usage for key directories (data, radio, etc.).
func DiskUsageHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		type dirInfo struct {
			Path string  `json:"path"`
			Size string `json:"size"`
			MB   float64 `json:"mb"`
		}
		var dirs []dirInfo
		home := os.Getenv("HOME")
		targets := []string{
			home + "/.local/share/simplex-node",
			home + "/simplex-node",
			home + "/A1-backups",
			home + "/bin/simplex-node",
		}
		for _, d := range targets {
			info, err := os.Stat(d)
			if err != nil {
				continue
			}
			if info.IsDir() {
				var size int64
				filepath.Walk(d, func(p string, fi os.FileInfo, err error) error {
					if err == nil && !fi.IsDir() {
						size += fi.Size()
					}
					return nil
				})
				dirs = append(dirs, dirInfo{Path: d, Size: fmt.Sprintf("%.1f MB", float64(size)/1024/1024), MB: float64(size) / 1024 / 1024})
			} else {
				dirs = append(dirs, dirInfo{Path: d, Size: fmt.Sprintf("%.1f MB", float64(info.Size())/1024/1024), MB: float64(info.Size()) / 1024 / 1024})
			}
		}
		var disk fs
		diskUsage("/", &disk)
		writeJSON(w, map[string]any{
			"directories": dirs,
			"root_disk": map[string]any{
				"total_gb": disk.Total / 1024 / 1024 / 1024,
				"used_gb":  disk.Used / 1024 / 1024 / 1024,
				"avail_gb": disk.Avail / 1024 / 1024 / 1024,
				"used_pct": fmt.Sprintf("%.1f%%", float64(disk.Used)/float64(disk.Total)*100),
			},
		})
	}
}

// ── Maintenance Mode (Cycle 11) ────────────────────────────────────────────────

type maintenanceState struct {
	Active    bool   `json:"active"`
	Message   string `json:"message"`
	StartedAt string `json:"started_at,omitempty"`
	EndAt     string `json:"end_at,omitempty"`
}

var (
	maintenanceMu   sync.RWMutex
	maintenanceMode bool
	maintenanceMsg  string
	maintenanceFile string
	maintenanceStart string
	maintenanceEnd  string
)

func initMaintenanceFile(dataDir string) {
	maintenanceFile = filepath.Join(dataDir, "config", "maintenance.json")
	os.MkdirAll(filepath.Dir(maintenanceFile), 0755)
	b, err := os.ReadFile(maintenanceFile)
	if err != nil {
		return
	}
	var st maintenanceState
	if json.Unmarshal(b, &st) == nil {
		maintenanceMode = st.Active
		maintenanceMsg = st.Message
		maintenanceStart = st.StartedAt
		maintenanceEnd = st.EndAt
	}
}

func saveMaintenanceState() {
	if maintenanceFile == "" {
		return
	}
	st := maintenanceState{
		Active:    maintenanceMode,
		Message:   maintenanceMsg,
		StartedAt: maintenanceStart,
		EndAt:     maintenanceEnd,
	}
	b, _ := json.MarshalIndent(st, "", "  ")
	os.WriteFile(maintenanceFile, b, 0644)
}

// MaintenanceHandler returns current maintenance mode status and allows toggling.
func MaintenanceHandler(dataDir string) http.HandlerFunc {
	initMaintenanceFile(dataDir)
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			maintenanceMu.RLock()
			on := maintenanceMode
			msg := maintenanceMsg
			start := maintenanceStart
			end := maintenanceEnd
			maintenanceMu.RUnlock()
			writeJSON(w, map[string]any{
				"maintenance": on,
				"message":     msg,
				"started_at":  start,
				"end_at":      end,
			})
		case http.MethodPost:
			var body struct {
				Maintenance bool   `json:"maintenance"`
				Message     string `json:"message,omitempty"`
				EndAt       string `json:"end_at,omitempty"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid JSON", 400)
				return
			}
			maintenanceMu.Lock()
			maintenanceMode = body.Maintenance
			maintenanceMsg = body.Message
			if body.Maintenance && maintenanceStart == "" {
				maintenanceStart = time.Now().UTC().Format(time.RFC3339)
			}
			if !body.Maintenance {
				maintenanceEnd = time.Now().UTC().Format(time.RFC3339)
			}
			if body.EndAt != "" {
				maintenanceEnd = body.EndAt
			}
			maintenanceMu.Unlock()
			saveMaintenanceState()
			state := "disabled"
			if body.Maintenance {
				state = "enabled"
			}
			logAudit("maintenance", "admin", fmt.Sprintf("maintenance mode %s: %s", state, body.Message))
			writeJSON(w, map[string]any{"ok": true, "maintenance": body.Maintenance, "message": body.Message, "started_at": maintenanceStart, "end_at": maintenanceEnd})
		default:
			http.Error(w, "GET or POST", 400)
		}
	}
}

// IsMaintenanceMode returns true if maintenance mode is active.
func IsMaintenanceMode() bool {
	maintenanceMu.RLock()
	defer maintenanceMu.RUnlock()
	return maintenanceMode
}

// ── Disk Cleanup Handler (Cycle 2) ─────────────────────────────────────────────

// DiskCleanupHandler runs cleanup: docker prune, old logs, temp files, go build cache.
func DiskCleanupHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", 400)
			return
		}
		results := map[string]any{}
		home := os.Getenv("HOME")

		// 1. Docker prune dangling images + build cache
		if out, err := exec.Command("docker", "system", "prune", "-f", "--filter", "until=24h").CombinedOutput(); err == nil {
			results["docker_prune"] = strings.TrimSpace(string(out))
		} else {
			results["docker_prune"] = "error: " + err.Error()
		}

		// 2. Docker builder prune
		if out, err := exec.Command("docker", "builder", "prune", "-f", "--filter", "until=24h").CombinedOutput(); err == nil {
			results["docker_builder_prune"] = strings.TrimSpace(string(out))
		} else {
			results["docker_builder_prune"] = "error: " + err.Error()
		}

		// 3. Go build cache
		goBuildCache := filepath.Join(home, ".cache", "go-build")
		if entries, err := os.ReadDir(goBuildCache); err == nil {
			var total int64
			for _, e := range entries {
				if info, err := e.Info(); err == nil {
					total += info.Size()
				}
			}
			os.RemoveAll(goBuildCache)
			results["go_build_cache"] = fmt.Sprintf("cleaned %d entries (%.1f MB)", len(entries), float64(total)/1024/1024)
		} else {
			results["go_build_cache"] = "not found"
		}

		// 4. Old logs (keep last 3, compress others)
		logDir := filepath.Join(home, ".local", "share", "simplex-node", "logs")
		if entries, err := os.ReadDir(logDir); err == nil {
			type logFile struct {
				name string
				mod  time.Time
				size int64
			}
			var logs []logFile
			for _, e := range entries {
				if info, err := e.Info(); err == nil && !info.IsDir() && !strings.HasSuffix(e.Name(), ".gz") {
					logs = append(logs, logFile{name: e.Name(), mod: info.ModTime(), size: info.Size()})
				}
			}
			if len(logs) > 3 {
				sort.Slice(logs, func(i, j int) bool { return logs[i].mod.After(logs[j].mod) })
				var compressed int64
				for _, lf := range logs[3:] {
					p := filepath.Join(logDir, lf.name)
					gzPath := p + ".gz"
					if _, err := os.Stat(gzPath); err == nil {
						os.Remove(p)
						compressed += lf.size
						continue
					}
					// Compress with gzip
					data, err := os.ReadFile(p)
					if err != nil {
						continue
					}
					var buf bytes.Buffer
					gw := gzip.NewWriter(&buf)
					gw.Write(data)
					gw.Close()
					if err := os.WriteFile(gzPath, buf.Bytes(), 0644); err == nil {
						os.Remove(p)
						compressed += lf.size
					}
				}
				results["old_logs_compressed"] = fmt.Sprintf("compressed %d logs (saved %.1f MB)", len(logs)-3, float64(compressed)/1024/1024)
			} else {
				results["old_logs_compressed"] = "none (only 3 or fewer)"
			}
		} else {
			results["old_logs_compressed"] = "no log dir"
		}

		// 5. Old backup cache (A1-backups, keep last 5)
		a1Backups := filepath.Join(home, "A1-backups")
		if entries, err := os.ReadDir(a1Backups); err == nil {
			type backup struct {
				name string
				mod  time.Time
				size int64
			}
			var backups []backup
			for _, e := range entries {
				if info, err := e.Info(); err == nil {
					backups = append(backups, backup{name: e.Name(), mod: info.ModTime(), size: info.Size()})
				}
			}
			if len(backups) > 5 {
				sort.Slice(backups, func(i, j int) bool { return backups[i].mod.After(backups[j].mod) })
				var removed int64
				for _, b := range backups[5:] {
					p := filepath.Join(a1Backups, b.name)
					if err := os.RemoveAll(p); err == nil {
						removed += b.size
					}
				}
				results["old_backups_removed"] = fmt.Sprintf("removed %d old backups (%.1f MB)", len(backups)-5, float64(removed)/1024/1024)
			} else {
				results["old_backups_removed"] = "none (only 5 or fewer)"
			}
		} else {
			results["old_backups_removed"] = "no A1-backups dir"
		}

		// 6. Docker volumes prune (unused)
		if out, err := exec.Command("docker", "volume", "prune", "-f").CombinedOutput(); err == nil {
			results["docker_volume_prune"] = strings.TrimSpace(string(out))
		} else {
			results["docker_volume_prune"] = "error: " + err.Error()
		}

		// 7. APT cache clean
		if out, err := exec.Command("apt-get", "clean").CombinedOutput(); err == nil {
			results["apt_cache"] = "cleaned"
			_ = out
		} else {
			results["apt_cache"] = "error: " + err.Error()
		}

		// 8. Journalctl vacuum (keep 200MB)
		if out, err := exec.Command("journalctl", "--vacuum-size=200M").CombinedOutput(); err == nil {
			results["journald"] = strings.TrimSpace(string(out))
		} else {
			results["journald"] = "error: " + err.Error()
		}

		// 9. Snap retain limit
		if _, err := exec.Command("snap", "set", "system", "snap.retain=2").CombinedOutput(); err == nil {
			results["snap_retain"] = "set retain=2"
		} else {
			results["snap_retain"] = "error: " + err.Error()
		}

		// 10. Pip cache
		pipCache := filepath.Join(home, ".cache", "pip")
		if fi, err := os.Stat(pipCache); err == nil {
			os.RemoveAll(pipCache)
			results["pip_cache"] = fmt.Sprintf("removed (%.1f MB)", float64(fi.Size())/1024/1024)
		} else {
			results["pip_cache"] = "not found"
		}

		writeJSON(w, map[string]any{"ok": true, "results": results})
	}
}

// ── System Logs Viewer (Cycle 66) ─────────────────────────────────────────────

// LogsHandler lists and returns the last N lines of log files in the data directory.
func LogsHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logFile := r.URL.Query().Get("file")
		n := 50
		if s := r.URL.Query().Get("lines"); s != "" {
			if v, err := strconv.Atoi(s); err == nil && v > 0 && v <= 500 {
				n = v
			}
		}

		logDir := filepath.Join(dataDir, "logs")
		if logFile == "" {
			// List available log files
			entries, err := os.ReadDir(logDir)
			if err != nil {
				writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
				return
			}
			var files []string
			for _, e := range entries {
				if !e.IsDir() {
					files = append(files, e.Name())
				}
			}
			writeJSON(w, map[string]any{"ok": true, "files": files, "dir": logDir})
			return
		}

		// Sanitize: prevent path traversal
		if strings.Contains(logFile, "..") || strings.Contains(logFile, "/") {
			http.Error(w, "invalid file name", 400)
			return
		}

		path := filepath.Join(logDir, logFile)
		info, err := os.Stat(path)
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "file not found"})
			return
		}
		if info.Size() > 10*1024*1024 {
			writeJSON(w, map[string]any{"ok": false, "error": "file too large (>10MB)"})
			return
		}
		if info.Size() == 0 {
			writeJSON(w, map[string]any{"ok": true, "lines": []string{}, "file": logFile, "size": 0})
			return
		}

		data, err := os.ReadFile(path)
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		allLines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
		start := 0
		if len(allLines) > n {
			start = len(allLines) - n
		}
		lines := allLines[start:]

		writeJSON(w, map[string]any{
			"ok":    true,
			"file":  logFile,
			"lines": lines,
			"total": len(allLines),
			"shown": len(lines),
			"size":  info.Size(),
		})
	}
}

// ── Container Health Auto-Healer (Cycle 58) ────────────────────────────────────

// StartContainerAutoHealer runs a goroutine that checks Docker container health
// every 5 minutes and auto-restarts unhealthy containers.
func StartContainerAutoHealer() {
	go func() {
		slog.Info("container auto-healer started (interval: 5m)")
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		for range ticker.C {
			status := dockerStatus()
			containers, _ := status["containers"].(map[string]string)
			if containers == nil {
				continue
			}
			for name, st := range containers {
				if strings.Contains(st, "unhealthy") {
					slog.Warn("auto-heal: restarting unhealthy container", "name", name, "status", st)
					cmd := exec.Command("docker", "restart", name)
					if out, err := cmd.CombinedOutput(); err != nil {
						slog.Error("auto-heal: restart failed", "name", name, "err", err, "out", string(out))
					} else {
						slog.Info("auto-heal: container restarted", "name", name)
						logAudit("auto-heal", "system", "restarted unhealthy container: "+name)
					}
				}
			}
		}
	}()
}

// ── Rate Limit Config API (b102) ──────────────────────────────────────────────

var (
	configurableLimiters   = map[string]*middleware.RateLimiter{}
	configurableLimitersMu sync.Mutex
)

// RegisterConfigurableLimiter registers a named rate limiter for runtime configuration via the admin API.
func RegisterConfigurableLimiter(name string, rl *middleware.RateLimiter) {
	configurableLimitersMu.Lock()
	configurableLimiters[name] = rl
	configurableLimitersMu.Unlock()
}

// RateLimitConfigHandler gets or sets the rate limiter configuration.
func RateLimitConfigHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			configurableLimitersMu.Lock()
			limiters := make(map[string]any)
			for name, rl := range configurableLimiters {
				limiters[name] = rl.Status()
			}
			configurableLimitersMu.Unlock()
			writeJSON(w, map[string]any{"ok": true, "limiters": limiters})
		case "POST":
			var req struct {
				Name  string `json:"name"`
				Rate  *int   `json:"rate"`
				Burst *int   `json:"burst"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
				return
			}
			configurableLimitersMu.Lock()
			rl, ok := configurableLimiters[req.Name]
			configurableLimitersMu.Unlock()
			if !ok {
				writeJSON(w, map[string]any{"ok": false, "error": "limiter not found"})
				return
			}
			if req.Rate != nil {
				if *req.Rate <= 0 {
					writeJSON(w, map[string]any{"ok": false, "error": "rate must be positive"})
					return
				}
				rl.SetRate(*req.Rate)
			}
			if req.Burst != nil {
				if *req.Burst <= 0 {
					writeJSON(w, map[string]any{"ok": false, "error": "burst must be positive"})
					return
				}
				rl.SetBurst(*req.Burst)
			}
			writeJSON(w, map[string]any{"ok": true, "status": rl.Status()})
		default:
			http.Error(w, "GET/POST", 405)
		}
	}
}

// ── Centralized Threshold Config (Cycle 14) ────────────────────────────────────

// ThresholdConfig holds configurable thresholds for disk, backup, bridge, and health checks.
type ThresholdConfig struct {
	DiskWarnPct                int `json:"disk_warn_pct"`
	DiskFailPct                int `json:"disk_fail_pct"`
	DataDirWarnMB              int `json:"data_dir_warn_mb"`
	DataDirFailMB              int `json:"data_dir_fail_mb"`
	BackupRetentionCount       int `json:"backup_retention_count"`
	LogRetentionCount          int `json:"log_retention_count"`
	AutoCleanupIntervalHours   int `json:"auto_cleanup_interval_hours"`
	AutoCleanupThresholdPct    int `json:"auto_cleanup_threshold_pct"`
	BridgeRestartThreshold     int `json:"bridge_restart_threshold"`
	DiskCleanupThresholdPct    int `json:"disk_cleanup_threshold_pct"`
	DiskCriticalThresholdPct   int `json:"disk_critical_threshold_pct"`
	BridgeReconnectMaxBackoff  int `json:"bridge_reconnect_max_backoff_sec"`
	HealthCheckIntervalSec     int `json:"health_check_interval_sec"`
	BackupIntervalHours        int `json:"backup_interval_hours"`
	LogRetentionDays           int `json:"log_retention_days"`
	AutoArchiveDays            int `json:"auto_archive_days"`
}

func defaultThresholdConfig() ThresholdConfig {
	return ThresholdConfig{
		DiskWarnPct:               80,
		DiskFailPct:               95,
		DataDirWarnMB:             5000,
		DataDirFailMB:             10000,
		BackupRetentionCount:      5,
		LogRetentionCount:         3,
		AutoCleanupIntervalHours:  6,
		AutoCleanupThresholdPct:   85,
		BridgeRestartThreshold:    3,
		DiskCleanupThresholdPct:   85,
		DiskCriticalThresholdPct:  95,
		BridgeReconnectMaxBackoff: 30,
		HealthCheckIntervalSec:    15,
		BackupIntervalHours:       6,
		LogRetentionDays:          3,
		AutoArchiveDays:           90,
	}
}

var (
	configMu   sync.RWMutex
	configFile string
	thresholds ThresholdConfig
)

// ── Backup Remote Sync ────────────────────────────────────────────────────

var (
	backupSyncMu     sync.RWMutex
	backupRemoteURL  string
	backupRemoteKey  string
	backupLastSync   string
	backupLastError  string
)

// BackupSyncHandler syncs the latest backup to a remote URL.
func BackupSyncHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			URL string `json:"url"`
			Key string `json:"key"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
			return
		}
		if req.URL != "" {
			backupSyncMu.Lock()
			backupRemoteURL = req.URL
			backupRemoteKey = req.Key
			backupSyncMu.Unlock()
			// Persist to config
			persistBackupSyncConfig(dataDir)
			writeJSON(w, map[string]any{"ok": true, "message": "remote URL configured"})
			return
		}
		// Trigger sync with configured URL
		backupSyncMu.RLock()
		url := backupRemoteURL
		key := backupRemoteKey
		backupSyncMu.RUnlock()
		if url == "" {
			writeJSON(w, map[string]any{"ok": false, "error": "no remote URL configured"})
			return
		}
		// Find latest backup
		backupDir := filepath.Join(os.Getenv("HOME"), "A1-backups")
		entries, err := os.ReadDir(backupDir)
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "no backups found"})
			return
		}
		var latest os.DirEntry
		var latestMod time.Time
		for _, e := range entries {
			if e.IsDir() {
				info, err := e.Info()
				if err != nil {
					continue
				}
				if info.ModTime().After(latestMod) {
					latest = e
					latestMod = info.ModTime()
				}
			}
		}
		if latest == nil {
			writeJSON(w, map[string]any{"ok": false, "error": "no backup directories found"})
			return
		}
		go func() {
			err := syncBackupToRemote(filepath.Join(backupDir, latest.Name()), url, key)
			backupSyncMu.Lock()
			backupLastSync = time.Now().UTC().Format(time.RFC3339)
			if err != nil {
				backupLastError = err.Error()
				slog.Error("backup sync failed", "error", err)
			} else {
				backupLastError = ""
				slog.Info("backup sync completed", "backup", latest.Name())
			}
			backupSyncMu.Unlock()
		}()
		writeJSON(w, map[string]any{"ok": true, "message": "sync started"})
	}
}

func syncBackupToRemote(path, url, key string) error {
	data, err := tarGzDir(path)
	if err != nil {
		return fmt.Errorf("archive: %w", err)
	}
	req, err := http.NewRequest("PUT", url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	req.Header.Set("Content-Type", "application/gzip")
	if key != "" {
		mac := hmac.New(sha256.New, []byte(key))
		mac.Write(data)
		sig := hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-HMAC-Signature", sig)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("remote returned %d", resp.StatusCode)
	}
	return nil
}

func tarGzDir(dir string) ([]byte, error) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		if rel == "." {
			return nil
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = rel
		if info.IsDir() {
			hdr.Typeflag = tar.TypeDir
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if !info.IsDir() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if _, err := tw.Write(data); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// BackupSyncStatusHandler returns the sync status.
func BackupSyncStatusHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		backupSyncMu.RLock()
		defer backupSyncMu.RUnlock()
		writeJSON(w, map[string]any{
			"ok":         true,
			"remote_url": backupRemoteURL,
			"last_sync":  backupLastSync,
			"last_error": backupLastError,
		})
	}
}

func persistBackupSyncConfig(dataDir string) {
	backupSyncMu.RLock()
	defer backupSyncMu.RUnlock()
	cfg := map[string]string{
		"backup_remote_url": backupRemoteURL,
		"backup_remote_key": backupRemoteKey,
	}
	b, _ := json.Marshal(cfg)
	os.WriteFile(filepath.Join(dataDir, "config", "backup_sync.json"), b, 0600)
}

// InitBackupSync loads persisted backup sync configuration from disk.
func InitBackupSync(dataDir string) {
	backupSyncMu.Lock()
	defer backupSyncMu.Unlock()
	b, err := os.ReadFile(filepath.Join(dataDir, "config", "backup_sync.json"))
	if err != nil {
		return
	}
	var cfg map[string]string
	if json.Unmarshal(b, &cfg) == nil {
		backupRemoteURL = cfg["backup_remote_url"]
		backupRemoteKey = cfg["backup_remote_key"]
	}
}

func initThresholdConfig(dataDir string) {
	configFile = filepath.Join(dataDir, "config", "thresholds.json")
	os.MkdirAll(filepath.Dir(configFile), 0755)
	thresholds = defaultThresholdConfig()
	if b, err := os.ReadFile(configFile); err == nil {
		var cfg ThresholdConfig
		if json.Unmarshal(b, &cfg) == nil {
			thresholds = cfg
		}
	}
}

func saveThresholdConfig() {
	b, _ := json.MarshalIndent(thresholds, "", "  ")
	os.WriteFile(configFile, b, 0644)
}

// GetThreshold returns the config value for a given key, falling back to defaults.
func GetThreshold(key string) int {
	configMu.RLock()
	defer configMu.RUnlock()
	dflt := defaultThresholdConfig()
	switch key {
	case "disk_warn_pct":
		if thresholds.DiskWarnPct != 0 {
			return thresholds.DiskWarnPct
		}
		return dflt.DiskWarnPct
	case "disk_fail_pct":
		if thresholds.DiskFailPct != 0 {
			return thresholds.DiskFailPct
		}
		return dflt.DiskFailPct
	case "data_dir_warn_mb":
		if thresholds.DataDirWarnMB != 0 {
			return thresholds.DataDirWarnMB
		}
		return dflt.DataDirWarnMB
	case "data_dir_fail_mb":
		if thresholds.DataDirFailMB != 0 {
			return thresholds.DataDirFailMB
		}
		return dflt.DataDirFailMB
	case "backup_retention_count":
		if thresholds.BackupRetentionCount != 0 {
			return thresholds.BackupRetentionCount
		}
		return dflt.BackupRetentionCount
	case "log_retention_count":
		if thresholds.LogRetentionCount != 0 {
			return thresholds.LogRetentionCount
		}
		return dflt.LogRetentionCount
	case "auto_cleanup_interval_hours":
		if thresholds.AutoCleanupIntervalHours != 0 {
			return thresholds.AutoCleanupIntervalHours
		}
		return dflt.AutoCleanupIntervalHours
	case "auto_cleanup_threshold_pct":
		if thresholds.AutoCleanupThresholdPct != 0 {
			return thresholds.AutoCleanupThresholdPct
		}
		return dflt.AutoCleanupThresholdPct
	case "bridge_restart_threshold":
		if thresholds.BridgeRestartThreshold != 0 {
			return thresholds.BridgeRestartThreshold
		}
		return dflt.BridgeRestartThreshold
	case "disk_cleanup_threshold_pct":
		if thresholds.DiskCleanupThresholdPct != 0 {
			return thresholds.DiskCleanupThresholdPct
		}
		return dflt.DiskCleanupThresholdPct
	case "disk_critical_threshold_pct":
		if thresholds.DiskCriticalThresholdPct != 0 {
			return thresholds.DiskCriticalThresholdPct
		}
		return dflt.DiskCriticalThresholdPct
	case "bridge_reconnect_max_backoff_sec":
		if thresholds.BridgeReconnectMaxBackoff != 0 {
			return thresholds.BridgeReconnectMaxBackoff
		}
		return dflt.BridgeReconnectMaxBackoff
	case "health_check_interval_sec":
		if thresholds.HealthCheckIntervalSec != 0 {
			return thresholds.HealthCheckIntervalSec
		}
		return dflt.HealthCheckIntervalSec
	case "backup_interval_hours":
		if thresholds.BackupIntervalHours != 0 {
			return thresholds.BackupIntervalHours
		}
		return dflt.BackupIntervalHours
	case "log_retention_days":
		if thresholds.LogRetentionDays != 0 {
			return thresholds.LogRetentionDays
		}
		return dflt.LogRetentionDays
	}
	return 0
}

// ConfigHandler returns current threshold config (GET) or updates it (POST).
func ConfigHandler(dataDir string) http.HandlerFunc {
	initThresholdConfig(dataDir)
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			configMu.RLock()
			cfg := thresholds
			configMu.RUnlock()
			writeJSON(w, map[string]any{"ok": true, "config": cfg})
		case "POST":
			var req ThresholdConfig
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
				return
			}
			configMu.Lock()
			if req.DiskWarnPct != 0 {
				thresholds.DiskWarnPct = req.DiskWarnPct
			}
			if req.DiskFailPct != 0 {
				thresholds.DiskFailPct = req.DiskFailPct
			}
			if req.DataDirWarnMB != 0 {
				thresholds.DataDirWarnMB = req.DataDirWarnMB
			}
			if req.DataDirFailMB != 0 {
				thresholds.DataDirFailMB = req.DataDirFailMB
			}
			if req.BackupRetentionCount != 0 {
				thresholds.BackupRetentionCount = req.BackupRetentionCount
			}
			if req.LogRetentionCount != 0 {
				thresholds.LogRetentionCount = req.LogRetentionCount
			}
			if req.AutoCleanupIntervalHours != 0 {
				thresholds.AutoCleanupIntervalHours = req.AutoCleanupIntervalHours
			}
			if req.AutoCleanupThresholdPct != 0 {
				thresholds.AutoCleanupThresholdPct = req.AutoCleanupThresholdPct
			}
			if req.BridgeRestartThreshold != 0 {
				thresholds.BridgeRestartThreshold = req.BridgeRestartThreshold
			}
			if req.DiskCleanupThresholdPct != 0 {
				thresholds.DiskCleanupThresholdPct = req.DiskCleanupThresholdPct
			}
			if req.DiskCriticalThresholdPct != 0 {
				thresholds.DiskCriticalThresholdPct = req.DiskCriticalThresholdPct
			}
			if req.BridgeReconnectMaxBackoff != 0 {
				thresholds.BridgeReconnectMaxBackoff = req.BridgeReconnectMaxBackoff
			}
			if req.HealthCheckIntervalSec != 0 {
				thresholds.HealthCheckIntervalSec = req.HealthCheckIntervalSec
			}
			if req.BackupIntervalHours != 0 {
				thresholds.BackupIntervalHours = req.BackupIntervalHours
			}
			if req.LogRetentionDays != 0 {
				thresholds.LogRetentionDays = req.LogRetentionDays
			}
			saveThresholdConfig()
			configMu.Unlock()
			logAudit("config_update", "admin", "threshold config updated")
			writeJSON(w, map[string]any{"ok": true, "config": thresholds})
		default:
			http.Error(w, "GET/POST", 405)
		}
	}
}

// ── Silver Coin Shop (b122+cycles+DC) ─────────────────────────────────────────
// Nanotaler (NT) = 1 nanotaler = 1 ng. 1 TLR = 31,103,480,000 NT.
// Physical silver coins sold at 1 TLR each with small premium.

const (
	// NTPerTLR defines the number of nanotalers per 1 TLR (1 troy oz of silver).
	NTPerTLR        int64 = 31_103_480_000
	// CoinPremiumBPS is the premium (in basis points) added to physical silver coin sales.
	CoinPremiumBPS  int64 = 500 // 5% premium over spot
)

// SilverCoin represents a physical silver coin backed by the treasury reserve.
type SilverCoin struct {
	ID             string  `json:"id"`
	Serial         string  `json:"serial"`
	DenominationTLR int    `json:"denomination_tlr"`
	DenominationNT int64   `json:"denomination_nt"`
	PriceNT        int64   `json:"price_nt"`
	SilverSpotUSD  float64 `json:"silver_spot_usd"`
	ReserveRatio   float64 `json:"reserve_ratio"`
	Status         string  `json:"status"`
	Owner          string  `json:"owner,omitempty"`
	BoughtAt       string  `json:"bought_at,omitempty"`
	CreatedAt      string  `json:"created_at"`
}

func shopFile(dataDir string) string {
	return filepath.Join(dataDir, "silver_shop.json")
}

func loadShop(dataDir string) []SilverCoin {
	var coins []SilverCoin
	b, err := os.ReadFile(shopFile(dataDir))
	if err == nil {
		json.Unmarshal(b, &coins)
	}
	if coins == nil {
		coins = make([]SilverCoin, 0)
	}
	return coins
}

func saveShop(dataDir string, coins []SilverCoin) {
	b, _ := json.MarshalIndent(coins, "", "  ")
	os.WriteFile(shopFile(dataDir), b, 0644)
}

// SilverShopHandler returns the silver coin shop inventory.
func SilverShopHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		switch r.Method {
		case "GET":
			listCoins(w, dataDir)
		case "POST":
			mintCoin(w, r, dataDir)
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	}
}

func listCoins(w http.ResponseWriter, dataDir string) {
	coins := loadShop(dataDir)
	// Read current reserve ratio
	reserveNg := int64(0)
	reserveFile := filepath.Join(dataDir, "silver_reserve_ng.txt")
	if b, err := os.ReadFile(reserveFile); err == nil {
		s := strings.TrimSpace(string(b))
		for i := range s {
			if s[i] < '0' || s[i] > '9' {
				s = s[:i]
				break
			}
		}
		fmt.Sscanf(s, "%d", &reserveNg)
	}
	ledger := loadLedger(dataDir)
	backingRatio := 0.0
	if ledger.TotalSupply > 0 {
		backingRatio = float64(reserveNg) / float64(ledger.TotalSupply)
	}
	// Read silver spot
	spotUsd := 75.0
	if b, err := os.ReadFile(filepath.Join(dataDir, "silver_spot_usd.txt")); err == nil {
		fmt.Sscanf(strings.TrimSpace(string(b)), "%f", &spotUsd)
	}
	writeJSON(w, map[string]any{
		"ok":             true,
		"coins":          coins,
		"count":          len(coins),
		"reserve_ng":     reserveNg,
		"backing_ratio":  backingRatio,
		"silver_spot_usd": spotUsd,
		"nt_per_tlr":     NTPerTLR,
	})
}

func mintCoin(w http.ResponseWriter, r *http.Request, dataDir string) {
	// Mint a new 1 TLR silver coin for the island shop
	coins := loadShop(dataDir)
	nextID := len(coins) + 1
	now := time.Now().UTC()

	// Read silver spot
	spotUsd := 75.0
	if b, err := os.ReadFile(filepath.Join(dataDir, "silver_spot_usd.txt")); err == nil {
		fmt.Sscanf(strings.TrimSpace(string(b)), "%f", &spotUsd)
	}
	// Price = 1 TLR in NT + 5% premium
	baseNT := NTPerTLR
	premiumNT := baseNT * CoinPremiumBPS / 10000
	priceNT := baseNT + premiumNT

	// Reserve check
	reserveNg := int64(0)
	reserveFile := filepath.Join(dataDir, "silver_reserve_ng.txt")
	if b, err := os.ReadFile(reserveFile); err == nil {
		s := strings.TrimSpace(string(b))
		for i := range s {
			if s[i] < '0' || s[i] > '9' {
				s = s[:i]
				break
			}
		}
		fmt.Sscanf(s, "%d", &reserveNg)
	}
	if reserveNg < NTPerTLR {
		writeJSON(w, map[string]any{"ok": false, "error": "insufficient silver reserve"})
		return
	}

	coin := SilverCoin{
		ID:              fmt.Sprintf("silver-coin-%d", now.UnixMilli()),
		Serial:          fmt.Sprintf("SILVER-1TLR-%04d", nextID),
		DenominationTLR: 1,
		DenominationNT:  NTPerTLR,
		PriceNT:         priceNT,
		SilverSpotUSD:   spotUsd,
		Status:          "available",
		CreatedAt:       now.Format(time.RFC3339),
	}
	coins = append(coins, coin)
	saveShop(dataDir, coins)

	// Deduct from reserve
	reserveNg -= NTPerTLR
	os.WriteFile(reserveFile, []byte(fmt.Sprintf("%d", reserveNg)), 0644)

	writeJSON(w, map[string]any{"ok": true, "coin": coin, "reserve_ng": reserveNg})
}

// SilverBuyHandler purchases a silver coin from the shop using wallet NT.
func SilverBuyHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, `{"error":"POST required"}`, 405)
			return
		}
		var req struct {
			CoinID string `json:"coin_id"`
			Buyer  string `json:"buyer"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
			return
		}
		if req.CoinID == "" || req.Buyer == "" {
			writeJSON(w, map[string]any{"ok": false, "error": "coin_id and buyer required"})
			return
		}
		coins := loadShop(dataDir)
		var coin *SilverCoin
		for i := range coins {
			if coins[i].ID == req.CoinID {
				coin = &coins[i]
				break
			}
		}
		if coin == nil {
			writeJSON(w, map[string]any{"ok": false, "error": "coin not found"})
			return
		}
		if coin.Status != "available" {
			writeJSON(w, map[string]any{"ok": false, "error": "coin not available"})
			return
		}
		ledger := economy.LoadLedger(dataDir)
		ledger.EnsureAccount(req.Buyer)
		bal := ledger.Balance(req.Buyer)
		if bal < coin.PriceNT {
			writeJSON(w, map[string]any{"ok": false, "error": "insufficient balance"})
			return
		}
		// Transfer funds: buyer → treasury
		ledger.Transfer(req.Buyer, "treasury", coin.PriceNT)
		ledger.Save(dataDir)

		// Mark coin as sold
		coin.Status = "sold"
		coin.Owner = req.Buyer
		coin.BoughtAt = time.Now().UTC().Format(time.RFC3339)
		saveShop(dataDir, coins)

		writeJSON(w, map[string]any{"ok": true, "coin": coin})
	}
}

// SilverMyCoinsHandler returns the caller's owned silver coins.
func SilverMyCoinsHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pubkey := r.URL.Query().Get("pubkey")
		if pubkey == "" {
			http.Error(w, `{"error":"pubkey required"}`, 400)
			return
		}
		coins := loadShop(dataDir)
		mine := make([]SilverCoin, 0)
		for _, c := range coins {
			if c.Owner == pubkey {
				mine = append(mine, c)
			}
		}
		writeJSON(w, map[string]any{"ok": true, "coins": mine, "count": len(mine)})
	}
}

// SilverRedeemHandler redeems a silver coin for 99% NT refund.
func SilverRedeemHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, `{"error":"POST required"}`, 405)
			return
		}
		var req struct {
			CoinID string `json:"coin_id"`
			Holder string `json:"holder"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
			return
		}
		if req.CoinID == "" || req.Holder == "" {
			writeJSON(w, map[string]any{"ok": false, "error": "coin_id and holder required"})
			return
		}
		coins := loadShop(dataDir)
		var coin *SilverCoin
		for i := range coins {
			if coins[i].ID == req.CoinID {
				coin = &coins[i]
				break
			}
		}
		if coin == nil {
			writeJSON(w, map[string]any{"ok": false, "error": "coin not found"})
			return
		}
		if coin.Owner != req.Holder {
			writeJSON(w, map[string]any{"ok": false, "error": "not your coin"})
			return
		}
		if coin.Status != "sold" {
			writeJSON(w, map[string]any{"ok": false, "error": "coin not redeemable"})
			return
		}

		// Refund buyer — minus 1% fee
		refund := coin.PriceNT * 99 / 100
		ledger := economy.LoadLedger(dataDir)
		ledger.Transfer("treasury", req.Holder, refund)
		ledger.Save(dataDir)

		// Return coin to shop
		coin.Status = "available"
		coin.Owner = ""
		coin.BoughtAt = ""
		saveShop(dataDir, coins)

		writeJSON(w, map[string]any{"ok": true, "refund_nt": refund, "coin": coin})
	}
}

// ── Silver-Backed Asset API (b103) ────────────────────────────────────────────

// SilverAssetMintHandler creates a new silver-backed asset.
func SilverAssetMintHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		var req struct {
			Holder   string `json:"holder"`
			AmountNg int64  `json:"amount_ng"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
			return
		}
		if req.Holder == "" || req.AmountNg <= 0 {
			writeJSON(w, map[string]any{"ok": false, "error": "holder and amount_ng required"})
			return
		}
		// Check silver reserve
		reserveNg := int64(0)
		if b, err := os.ReadFile(filepath.Join(dataDir, "silver_reserve_ng.txt")); err == nil {
			fmt.Sscanf(strings.TrimSpace(string(b)), "%d", &reserveNg)
		}
		if req.AmountNg > reserveNg {
			writeJSON(w, map[string]any{"ok": false, "error": "insufficient silver reserve"})
			return
		}
		// Mint silver-backed asset
		assetFile := filepath.Join(dataDir, "silver_assets.json")
		var assets []map[string]any
		if b, err := os.ReadFile(assetFile); err == nil {
			json.Unmarshal(b, &assets)
		}
		if assets == nil {
			assets = make([]map[string]any, 0)
		}
		asset := map[string]any{
			"id":         fmt.Sprintf("silver-%d", time.Now().UnixNano()),
			"holder":     req.Holder,
			"amount_ng":  req.AmountNg,
			"created_at": time.Now().UTC().Format(time.RFC3339),
			"status":     "active",
		}
		assets = append(assets, asset)
		fileutil.WriteJSON(assetFile, assets)
		// Deduct from reserve
		newReserve := reserveNg - req.AmountNg
		os.WriteFile(filepath.Join(dataDir, "silver_reserve_ng.txt"), []byte(fmt.Sprintf("%d\n", newReserve)), 0600)
		ledger := economy.LoadLedger(dataDir)
		ledger.EnsureAccount(req.Holder)
		ledger.Mint(req.Holder, req.AmountNg)
		ledger.Save(dataDir)
		logAudit("silver_mint", req.Holder, fmt.Sprintf("minted %d ng, reserve -> %d, asset_id=%s", req.AmountNg, newReserve, asset["id"]))
		writeJSON(w, map[string]any{
			"ok":           true,
			"asset":        asset,
			"reserve_ng":   newReserve,
		})
	}
}

// SilverAssetBurnHandler destroys a silver-backed asset and returns its value.
func SilverAssetBurnHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		var req struct {
			AssetID  string `json:"asset_id"`
			Holder   string `json:"holder"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
			return
		}
		assetFile := filepath.Join(dataDir, "silver_assets.json")
		var assets []map[string]any
		assetData, err := os.ReadFile(assetFile)
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "no assets found"})
			return
		}
		json.Unmarshal(assetData, &assets)
		found := false
		var amountNg int64
		remaining := make([]map[string]any, 0, len(assets))
		for _, a := range assets {
			if a["id"] == req.AssetID && a["holder"] == req.Holder && a["status"] == "active" {
				found = true
				a["status"] = "burned"
				a["burned_at"] = time.Now().UTC().Format(time.RFC3339)
				amountNg, _ = a["amount_ng"].(int64)
				if amountNg == 0 {
					if f, ok := a["amount_ng"].(float64); ok {
						amountNg = int64(f)
					}
				}
			}
			remaining = append(remaining, a)
		}
		if !found {
			writeJSON(w, map[string]any{"ok": false, "error": "asset not found"})
			return
		}
		fileutil.WriteJSON(assetFile, remaining)
		// Return to reserve
		reserveNg := int64(0)
		if b, err := os.ReadFile(filepath.Join(dataDir, "silver_reserve_ng.txt")); err == nil {
			fmt.Sscanf(strings.TrimSpace(string(b)), "%d", &reserveNg)
		}
		newReserve := reserveNg + amountNg
		os.WriteFile(filepath.Join(dataDir, "silver_reserve_ng.txt"), []byte(fmt.Sprintf("%d\n", newReserve)), 0600)
		ledger := economy.LoadLedger(dataDir)
		ledger.Transfer(req.Holder, "reserve", amountNg)
		ledger.Save(dataDir)
		writeJSON(w, map[string]any{"ok": true, "burned": req.AssetID, "returned_ng": amountNg, "reserve_ng": newReserve})
	}
}

// SilverAssetListHandler returns all silver-backed assets.
func SilverAssetListHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		assetFile := filepath.Join(dataDir, "silver_assets.json")
		var assets []map[string]any
		if b, err := os.ReadFile(assetFile); err == nil {
			json.Unmarshal(b, &assets)
		}
		if assets == nil {
			assets = make([]map[string]any, 0)
		}
		writeJSON(w, map[string]any{"ok": true, "assets": assets, "count": len(assets)})
	}
}

// ── Enhanced Proof-of-Reserve (b104) ──────────────────────────────────────────

// ProofOfReserveDetailHandler returns detailed proof-of-reserve with backing ratio.
func ProofOfReserveDetailHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		reserveNg := int64(0)
		reserveFile := filepath.Join(dataDir, "silver_reserve_ng.txt")
		if b, err := os.ReadFile(reserveFile); err == nil {
			s := strings.TrimSpace(string(b))
			for i := range s {
				if s[i] < '0' || s[i] > '9' {
					s = s[:i]
					break
				}
			}
			fmt.Sscanf(s, "%d", &reserveNg)
		}
		ledger := economy.LoadLedger(dataDir)
		totalSupply := ledger.TotalSupply
		accounts := len(ledger.Accounts)
		// Load silver assets
		assetFile := filepath.Join(dataDir, "silver_assets.json")
		var assets []map[string]any
		if b, err := os.ReadFile(assetFile); err == nil {
			json.Unmarshal(b, &assets)
		}
		activeAssets := 0
		totalAssetNg := int64(0)
		for _, a := range assets {
			if a["status"] == "active" {
				activeAssets++
				if f, ok := a["amount_ng"].(float64); ok {
					totalAssetNg += int64(f)
				}
			}
		}
		// Check backing ratio
		backingRatio := 0.0
		if totalSupply > 0 {
			backingRatio = float64(reserveNg) / float64(totalSupply)
		}
		writeJSON(w, map[string]any{
			"ok":             true,
			"silver_reserve_ng": reserveNg,
			"total_supply_ng":   totalSupply,
			"backing_ratio":     backingRatio,
			"active_assets":     activeAssets,
			"total_asset_ng":    totalAssetNg,
			"accounts":          accounts,
			"healthy":           backingRatio >= 0.5,
			"status":            "audited",
			"audited_at":        time.Now().UTC().Format(time.RFC3339),
		})
	}
}

// ── Backup Verification (Cycle 15, Enhanced C32) ───────────────────────────────

// BackupManifestEntry records verification results for a single backup directory.
type BackupManifestEntry struct {
	Name       string `json:"name"`
	Date       string `json:"date"`
	SizeMB     float64 `json:"size_mb"`
	Verified   bool    `json:"verified"`
	VerifyDate string  `json:"verify_date,omitempty"`
	Status     string  `json:"status"`
}

func backupManifestPath() string {
	return filepath.Join(os.Getenv("HOME"), "A1-backups", "backup_manifest.json")
}

func loadBackupManifest() []BackupManifestEntry {
	var entries []BackupManifestEntry
	b, err := os.ReadFile(backupManifestPath())
	if err == nil {
		json.Unmarshal(b, &entries)
	}
	if entries == nil {
		entries = make([]BackupManifestEntry, 0)
	}
	return entries
}

func saveBackupManifest(entries []BackupManifestEntry) {
	b, _ := json.MarshalIndent(entries, "", "  ")
	os.WriteFile(backupManifestPath(), b, 0644)
}

func verifyBackupDir(backupPath string) (map[string]any, bool) {
	required := map[string]bool{"chat_history.json": false, "invoices.json": false}
	var files []string
	var totalSize int64
	filepath.Walk(backupPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(backupPath, path)
		if info.IsDir() {
			return nil
		}
		files = append(files, rel)
		totalSize += info.Size()
		if _, ok := required[info.Name()]; ok {
			required[info.Name()] = true
		}
		return nil
	})
	allFound := true
	for name, found := range required {
		if !found {
			allFound = false
			slog.Warn("backup verify: missing required file", "file", name, "backup", backupPath)
		}
	}
	status := "verified"
	if !allFound {
		status = "incomplete"
	}
	return map[string]any{
		"files":    files,
		"size_mb":  float64(totalSize) / 1024 / 1024,
		"status":   status,
		"all_ok":   allFound,
	}, allFound
}

// BackupVerifyHandler verifies A1-backups integrity (existing).
// POST /api/admin/backup/verify
func BackupVerifyHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", 400)
			return
		}
		backupDir := filepath.Join(os.Getenv("HOME"), "A1-backups")
		entries, err := os.ReadDir(backupDir)
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "cannot read backups: " + err.Error()})
			return
		}
		if len(entries) == 0 {
			writeJSON(w, map[string]any{"ok": false, "error": "no backups found"})
			return
		}
		sort.Slice(entries, func(i, j int) bool {
			ii, _ := entries[i].Info()
			ji, _ := entries[j].Info()
			return ii.ModTime().After(ji.ModTime())
		})
		latest := entries[0].Name()
		latestPath := filepath.Join(backupDir, latest)

		result, allOk := verifyBackupDir(latestPath)
		// Update manifest
		manifest := loadBackupManifest()
		found := false
		for i := range manifest {
			if manifest[i].Name == latest {
				manifest[i].Verified = allOk
				manifest[i].VerifyDate = time.Now().UTC().Format(time.RFC3339)
				manifest[i].Status = result["status"].(string)
				manifest[i].SizeMB = result["size_mb"].(float64)
				found = true
				break
			}
		}
		if !found {
			manifest = append(manifest, BackupManifestEntry{
				Name:       latest,
				Date:       time.Now().UTC().Format(time.RFC3339),
				SizeMB:     result["size_mb"].(float64),
				Verified:   allOk,
				VerifyDate: time.Now().UTC().Format(time.RFC3339),
				Status:     result["status"].(string),
			})
		}
		saveBackupManifest(manifest)

		writeJSON(w, map[string]any{
			"ok":            allOk,
			"latest_backup": latestPath,
			"files":         result["files"],
			"size_mb":       result["size_mb"],
			"status":        result["status"],
		})
	}
}

// BackupVerifyAllHandler verifies integrity of all backups in the A1-backups directory.
func BackupVerifyAllHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", 400)
			return
		}
		backupDir := filepath.Join(os.Getenv("HOME"), "A1-backups")
		entries, err := os.ReadDir(backupDir)
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "cannot read backups: " + err.Error()})
			return
		}
		manifest := make([]BackupManifestEntry, 0)
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			backupPath := filepath.Join(backupDir, e.Name())
			result, allOk := verifyBackupDir(backupPath)
			entry := BackupManifestEntry{
				Name:       e.Name(),
				Date:       time.Now().UTC().Format(time.RFC3339),
				SizeMB:     result["size_mb"].(float64),
				Verified:   allOk,
				VerifyDate: time.Now().UTC().Format(time.RFC3339),
				Status:     result["status"].(string),
			}
			manifest = append(manifest, entry)
			slog.Info("backup verify-all", "name", e.Name(), "status", entry.Status, "size_mb", entry.SizeMB)
		}
		saveBackupManifest(manifest)
		writeJSON(w, map[string]any{"ok": true, "verified": len(manifest), "backups": manifest})
	}
}

func autoVerifyBackup(backupDir, backupName string) {
	backupPath := filepath.Join(backupDir, backupName)
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return
	}
	result, allOk := verifyBackupDir(backupPath)
	manifest := loadBackupManifest()
	entry := BackupManifestEntry{
		Name:       backupName,
		Date:       time.Now().UTC().Format(time.RFC3339),
		SizeMB:     result["size_mb"].(float64),
		Verified:   allOk,
		VerifyDate: time.Now().UTC().Format(time.RFC3339),
		Status:     result["status"].(string),
	}
	found := false
	for i := range manifest {
		if manifest[i].Name == backupName {
			manifest[i] = entry
			found = true
			break
		}
	}
	if !found {
		manifest = append(manifest, entry)
	}
	saveBackupManifest(manifest)
	slog.Info("auto-verify backup", "name", backupName, "status", entry.Status, "size_mb", entry.SizeMB)
}

// ── Watchdog Ping (Cycle 18) ──────────────────────────────────────────────────

// PingHandler returns a lightweight health check response with uptime and bridge status.
func PingHandler(startTime time.Time) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		msgCount := 0
		if GlobalChatHub != nil {
			msgCount = GlobalChatHub.MessageCount()
		}
		writeJSON(w, map[string]any{
			"ok":              true,
			"timestamp":       time.Now().UTC().Format(time.RFC3339),
			"uptime_hours":    fmt.Sprintf("%.1f", time.Since(startTime).Hours()),
			"bridge":          BridgeConnected,
			"bridge_reconnects": BridgeReconnectCount,
			"messages":        msgCount,
			"goroutines":      runtime.NumGoroutine(),
		})
	}
}

// MobileStatusHandler returns lightweight JSON for mobile clients.
func MobileStatusHandler(startTime time.Time) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		msgCount := 0
		if GlobalChatHub != nil {
			msgCount = GlobalChatHub.MessageCount()
		}
		var diskPct float64
		var disk fs
		if err := diskUsage("/", &disk); err == nil && disk.Total > 0 {
			diskPct = float64(disk.Used) / float64(disk.Total) * 100
		}
		uptime := time.Since(startTime).Hours()
		writeJSON(w, map[string]any{
			"ok":            true,
			"bridge":        BridgeConnected,
			"messages":      msgCount,
			"uptime_hours":  float64(int(uptime*10)) / 10,
			"disk_pct":      int(diskPct),
		})
	}
}

// ── Relay Test / Status ──────────────────────────────────────────────────────

// RelayStatus holds the test results and quality metrics for a relay node.
type RelayStatus struct {
	ID           string  `json:"id"`
	Address      string  `json:"address"`
	LatencyMs    int     `json:"latency_ms"`
	QualityScore float64 `json:"quality_score"` // 0-100
	Reachable    bool    `json:"reachable"`
	LastTested   string  `json:"last_tested,omitempty"`
}

var (
	relayStore   map[string]*RelayStatus
	relayStoreMu sync.RWMutex
)

func init() {
	relayStore = make(map[string]*RelayStatus)
}

// RelayTestHandler tests reachability and latency of a relay node by TCP dial.
func RelayTestHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		id := r.URL.Query().Get("id")
		addr := r.URL.Query().Get("addr")
		if id == "" || addr == "" {
			http.Error(w, "id and addr query params required", http.StatusBadRequest)
			return
		}
		start := time.Now()
		conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
		latency := time.Since(start)
		reachable := err == nil
		if conn != nil {
			conn.Close()
		}
		score := 100.0
		latencyMs := int(latency.Milliseconds())
		if reachable {
			switch {
			case latencyMs < 50:
				score = 100
			case latencyMs < 100:
				score = 90
			case latencyMs < 200:
				score = 70
			case latencyMs < 500:
				score = 50
			default:
				score = 30
			}
		} else {
			score = 0
		}
		rs := &RelayStatus{
			ID:           id,
			Address:      addr,
			LatencyMs:    latencyMs,
			QualityScore: score,
			Reachable:    reachable,
			LastTested:   time.Now().UTC().Format(time.RFC3339),
		}
		relayStoreMu.Lock()
		relayStore[id] = rs
		relayStoreMu.Unlock()
		writeJSON(w, map[string]any{"ok": true, "relay": rs})
	}
}

// RelayStatusHandler returns all tested relay nodes sorted by quality score.
func RelayStatusHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		relayStoreMu.RLock()
		var list []*RelayStatus
		for _, rs := range relayStore {
			list = append(list, rs)
		}
		relayStoreMu.RUnlock()
		sort.Slice(list, func(i, j int) bool {
			return list[i].QualityScore > list[j].QualityScore
		})
		writeJSON(w, map[string]any{"ok": true, "relays": list, "count": len(list)})
	}
}
