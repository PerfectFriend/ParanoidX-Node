// Package transport implements the WebSocket transport API
package transport

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"log/slog"

	"ParanoidX/internal/api"
)

var upgrader = websocket.Upgrader{
	CheckOrigin:   func(r *http.Request) bool { return true },
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

func (h *Hub) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		apiKey := r.Header.Get("X-API-Key")
		if apiKey == "" {
			apiKey = r.URL.Query().Get("api_key")
		}
		app := h.Authenticate(apiKey)
		if app == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		r.Header.Set("X-App-ID", app.ID)
		r.Header.Set("X-App-Name", app.Name)
		next(w, r)
	}
}

func writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// RegisterHandler registers a new app.
// POST /api/transport/v1/register
func (h *Hub) RegisterHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, `{"error":"POST required"}`, http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
			http.Error(w, `{"error":"name required"}`, http.StatusBadRequest)
			return
		}
		app, err := h.Register(req.Name)
		if err != nil {
			writeJSON(w, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, map[string]any{
			"ok":              true,
			"app_id":          app.ID,
			"api_key":         app.APIKey,
			"reconnect_token": app.ReconnectToken,
			"name":            app.Name,
		})
	}
}

// WsHandler handles WebSocket connections with auto-reconnect support.
// GET /api/transport/v1/ws?api_key=...&reconnect=...
func (h *Hub) WsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		apiKey := r.Header.Get("X-API-Key")
		if apiKey == "" {
			apiKey = r.URL.Query().Get("api_key")
		}
		reconnectToken := r.URL.Query().Get("reconnect")

		var app *App
		if reconnectToken != "" {
			app = h.AuthenticateReconnect(reconnectToken)
		}
		if app == nil {
			app = h.Authenticate(apiKey)
		}
		if app == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			slog.Warn("[transport] ws upgrade", "error", err)
			return
		}
		defer conn.Close()

		sub := h.Subscribe(app.ID)
		defer h.Unsubscribe(app.ID, sub)

		slog.Info("[transport] ws connected", "app", app.Name, "id", app.ID)

		var wsMu sync.Mutex
		writeWS := func(msg WSMessage) error {
			wsMu.Lock()
			defer wsMu.Unlock()
			msg.Timestamp = time.Now().UTC().Format(time.RFC3339)
			conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
			return conn.WriteJSON(msg)
		}

		// Replay missed messages on reconnect
		isReconnect := reconnectToken != ""
		if isReconnect {
			h.ReplayHistory(app.ID, sub)
			slog.Info("[transport] replayed history", "app", app.Name)
		}

		connectedMsg := WSMessage{
			Type:  "connected",
			AppID: app.ID,
			Payload: map[string]any{
				"reconnect_token": app.ReconnectToken,
				"reconnected":     isReconnect,
			},
		}
		if err := writeWS(connectedMsg); err != nil {
			return
		}

		done := make(chan struct{}, 2)

		// Read from WS -> relay to bridge
		go func() {
			defer func() { done <- struct{}{} }()
			for {
				var msg WSMessage
				if err := conn.ReadJSON(&msg); err != nil {
					return
				}
				switch msg.Type {
				case "send":
					if !h.CheckRateLimit(app.ID) {
						h.stats.Lock()
						h.stats.rateLimitHits++
						h.stats.Unlock()
						writeWS(WSMessage{Type: "error", Payload: "rate limit exceeded"})
						h.LogAudit(app.ID, app.Name, "send", "rate limit exceeded", msg.ContactID, false)
						continue
					}
					if msg.Text == "" || msg.ContactID <= 0 {
						writeWS(WSMessage{Type: "error", Payload: "text and contact_id required"})
						continue
					}
					err := h.SendThroughBridge(msg.ContactID, msg.Text)
					if err != nil {
						h.stats.Lock()
						h.stats.errors++
						h.stats.Unlock()
						writeWS(WSMessage{Type: "error", Payload: err.Error()})
						h.LogAudit(app.ID, app.Name, "send", err.Error(), msg.ContactID, false)
					} else {
						h.stats.Lock()
						h.stats.messagesSent++
						h.stats.Unlock()
						statusMsg := WSMessage{
							Type:      "sent",
							ContactID: msg.ContactID,
							Text:      msg.Text,
							MsgID:     msg.MsgID,
						}
						writeWS(statusMsg)
						h.LogAudit(app.ID, app.Name, "send", "ok", msg.ContactID, true)
					}

				case "broadcast":
					if !h.CheckRateLimit(app.ID) {
						h.stats.Lock()
						h.stats.rateLimitHits++
						h.stats.Unlock()
						writeWS(WSMessage{Type: "error", Payload: "rate limit exceeded"})
						h.LogAudit(app.ID, app.Name, "broadcast", "rate limit exceeded", 0, false)
						continue
					}
					if msg.Text == "" {
						writeWS(WSMessage{Type: "error", Payload: "text required"})
						continue
					}
					app.mu.Lock()
					contacts := make([]int64, len(app.Contacts))
					copy(contacts, app.Contacts)
					app.mu.Unlock()
					sent := 0
					for _, cid := range contacts {
						if err := h.SendThroughBridge(cid, msg.Text); err == nil {
							sent++
						}
					}
					writeWS(WSMessage{
						Type:    "broadcast_result",
						Payload: map[string]any{"sent": sent, "total": len(contacts)},
					})
					h.stats.Lock()
					h.stats.messagesSent += int64(sent)
					h.stats.Unlock()
					h.LogAudit(app.ID, app.Name, "broadcast", fmt.Sprintf("sent=%d total=%d", sent, len(contacts)), 0, true)

				case "register_contact":
					if msg.ContactID > 0 {
						h.AddContact(app.ID, msg.ContactID)
						writeWS(WSMessage{Type: "contact_registered", ContactID: msg.ContactID})
						h.LogAudit(app.ID, app.Name, "register_contact", "", msg.ContactID, true)
					}

				case "unregister_contact":
					if msg.ContactID > 0 {
						h.RemoveContact(app.ID, msg.ContactID)
						writeWS(WSMessage{Type: "contact_unregistered", ContactID: msg.ContactID})
						h.LogAudit(app.ID, app.Name, "unregister_contact", "", msg.ContactID, true)
					}

				case "ping":
					writeWS(WSMessage{Type: "pong", Timestamp: time.Now().UTC().Format(time.RFC3339)})

				case "clear_history":
					h.ClearAppHistory(app.ID)
					writeWS(WSMessage{Type: "history_cleared"})

				default:
					writeWS(WSMessage{Type: "error", Payload: fmt.Sprintf("unknown type: %s", msg.Type)})
				}
			}
		}()

		// Write from hub -> WS (with heartbeat keepalive)
		go func() {
			defer func() { done <- struct{}{} }()
			hb := time.NewTicker(30 * time.Second)
			defer hb.Stop()
			for {
				select {
				case <-hb.C:
					writeWS(WSMessage{Type: "hb", Timestamp: time.Now().UTC().Format(time.RFC3339)})
				case msg, ok := <-sub:
					if !ok {
						return
					}
					if err := writeWS(msg); err != nil {
						return
					}
				}
			}
		}()

		<-done
		slog.Info("[transport] ws disconnected", "app", app.Name, "id", app.ID)
	}
}

// SendHandler sends a message through the bridge via HTTP.
// POST /api/transport/v1/send
func (h *Hub) SendHandler() http.HandlerFunc {
	return h.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, `{"error":"POST required"}`, http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			ContactID int64  `json:"contact_id"`
			Text      string `json:"text"`
			MsgID     string `json:"msg_id,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
			return
		}
		if req.Text == "" || req.ContactID <= 0 {
			http.Error(w, `{"error":"text and contact_id required"}`, http.StatusBadRequest)
			return
		}
		if !h.CheckRateLimit(r.Header.Get("X-App-ID")) {
			h.stats.Lock()
			h.stats.rateLimitHits++
			h.stats.Unlock()
			writeJSON(w, map[string]any{"ok": false, "error": "rate limit exceeded"})
			h.LogAudit(r.Header.Get("X-App-ID"), r.Header.Get("X-App-Name"), "send_http", "rate limit exceeded", req.ContactID, false)
			return
		}
		if err := h.SendThroughBridge(req.ContactID, req.Text); err != nil {
			h.stats.Lock()
			h.stats.errors++
			h.stats.Unlock()
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			h.LogAudit(r.Header.Get("X-App-ID"), r.Header.Get("X-App-Name"), "send_http", err.Error(), req.ContactID, false)
			return
		}
		h.stats.Lock()
		h.stats.messagesSent++
		h.stats.Unlock()
		writeJSON(w, map[string]any{"ok": true, "sent": true, "msg_id": req.MsgID})
		h.LogAudit(r.Header.Get("X-App-ID"), r.Header.Get("X-App-Name"), "send_http", "ok", req.ContactID, true)
	})
}

// HealthHandler returns transport health.
// GET /api/transport/v1/health
func (h *Hub) HealthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, h.Status())
	}
}

// StatsHandler returns detailed transport stats.
// GET /api/transport/v1/stats
func (h *Hub) StatsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, h.Stats())
	}
}

// ListHandler lists registered apps (requires auth).
// GET /api/transport/v1/apps
func (h *Hub) ListHandler() http.HandlerFunc {
	return h.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		apps := h.ListApps()
		type appInfo struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			CreatedAt string `json:"created_at"`
			LastSeen  string `json:"last_seen"`
			Contacts  int    `json:"contacts"`
		}
		list := make([]appInfo, 0, len(apps))
		for _, a := range apps {
			a.mu.Lock()
			cnt := len(a.Contacts)
			a.mu.Unlock()
			list = append(list, appInfo{
				ID:        a.ID,
				Name:      a.Name,
				CreatedAt: a.CreatedAt.Format(time.RFC3339),
				LastSeen:  a.LastSeen.Format(time.RFC3339),
				Contacts:  cnt,
			})
		}
		writeJSON(w, map[string]any{"ok": true, "apps": list})
	})
}

// ContactHandler manages app contacts.
// POST /api/transport/v1/contacts (add/remove with action field)
// GET /api/transport/v1/contacts (list contacts)
func (h *Hub) ContactHandler() http.HandlerFunc {
	return h.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		appID := r.Header.Get("X-App-ID")
		switch r.Method {
		case "GET":
			app := h.GetApp(appID)
			if app == nil {
				writeJSON(w, map[string]any{"ok": false, "error": "app not found"})
				return
			}
			app.mu.Lock()
			contacts := make([]int64, len(app.Contacts))
			copy(contacts, app.Contacts)
			app.mu.Unlock()
			writeJSON(w, map[string]any{"ok": true, "contacts": contacts, "count": len(contacts)})

		case "POST":
			var req struct {
				Action    string `json:"action"`
				ContactID int64  `json:"contact_id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ContactID <= 0 {
				http.Error(w, `{"error":"contact_id required"}`, http.StatusBadRequest)
				return
			}
			switch req.Action {
			case "add":
				h.AddContact(appID, req.ContactID)
				writeJSON(w, map[string]any{"ok": true, "added": req.ContactID})
			case "remove":
				h.RemoveContact(appID, req.ContactID)
				writeJSON(w, map[string]any{"ok": true, "removed": req.ContactID})
			default:
				http.Error(w, `{"error":"action must be 'add' or 'remove'"}`, http.StatusBadRequest)
			}

		default:
			http.Error(w, `{"error":"GET or POST required"}`, http.StatusMethodNotAllowed)
		}
	})
}

// BatchHandler handles batch message operations.
// POST /api/transport/v1/batch
func (h *Hub) BatchHandler() http.HandlerFunc {
	return h.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, `{"error":"POST required"}`, http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Messages []struct {
				ContactID int64  `json:"contact_id"`
				Text      string `json:"text"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
			return
		}
		appID := r.Header.Get("X-App-ID")
		results := make([]map[string]any, 0, len(req.Messages))
		for _, m := range req.Messages {
			res := map[string]any{"contact_id": m.ContactID, "text": m.Text}
			if m.Text == "" || m.ContactID <= 0 {
				res["error"] = "invalid params"
			} else if !h.CheckRateLimit(appID) {
				res["error"] = "rate limit exceeded"
			} else if err := h.SendThroughBridge(m.ContactID, m.Text); err != nil {
				res["error"] = err.Error()
			} else {
				res["sent"] = true
			}
			results = append(results, res)
		}
		writeJSON(w, map[string]any{"ok": true, "results": results})
	})
}

// ConfigHandler returns transport config for remote apps.
// GET /api/transport/v1/config
func (h *Hub) ConfigHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, h.Config())
	}
}

// AuditHandler returns recent audit log entries.
// GET /api/transport/v1/audit?limit=50
func (h *Hub) AuditHandler() http.HandlerFunc {
	return h.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		limit := 50
		if l := r.URL.Query().Get("limit"); l != "" {
			if parsed, err := parseInt(l); err == nil && parsed > 0 && parsed <= 1000 {
				limit = parsed
			}
		}
		entries := h.GetAuditLog(limit)
		writeJSON(w, map[string]any{"ok": true, "entries": entries, "count": len(entries)})
	})
}

// BackpressureHandler returns bridge backpressure status.
// GET /api/transport/v1/backpressure
func (h *Hub) BackpressureHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bridgeOK := api.BridgeConnected && api.SimplexCmd != nil
		status := "ok"
		var detail string
		if !api.BridgeConnected {
			status = "bridge_disconnected"
			detail = "SimpleX bridge is not connected"
		} else if api.SimplexCmd == nil {
			status = "bridge_unavailable"
			detail = "SimpleX command interface not available"
		}
		if api.BridgeReconnectCount > 5 {
			status = "bridge_unstable"
			detail = fmt.Sprintf("bridge reconnected %d times", api.BridgeReconnectCount)
		}
		writeJSON(w, map[string]any{
			"ok":               bridgeOK,
			"status":           status,
			"detail":           detail,
			"bridge_connected": api.BridgeConnected,
			"reconnects":       api.BridgeReconnectCount,
			"msg_queue_depth":  api.BridgeMsgQueueDepth,
		})
	}
}

func parseInt(s string) (int, error) {
	var n int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("not a number")
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

// WebhookHandler configures webhook URL for an app.
// POST /api/transport/v1/webhook
func (h *Hub) WebhookHandler() http.HandlerFunc {
	return h.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		appID := r.Header.Get("X-App-ID")
		if r.Method == "GET" {
			h.mu.RLock()
			app := h.apps[appID]
			h.mu.RUnlock()
			writeJSON(w, map[string]any{"ok": true, "webhook_url": app.WebhookURL})
			return
		}
		if r.Method != "POST" {
			http.Error(w, `{"error":"GET or POST required"}`, http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			URL string `json:"url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
			return
		}
		h.mu.Lock()
		if app := h.apps[appID]; app != nil {
			app.WebhookURL = req.URL
			h.save()
		}
		h.mu.Unlock()
		writeJSON(w, map[string]any{"ok": true, "webhook_url": req.URL})
		h.LogAudit(appID, r.Header.Get("X-App-Name"), "webhook", req.URL, 0, true)
	})
}

// BackupHandler exports all app data.
// GET /api/transport/v1/backup
func (h *Hub) BackupHandler() http.HandlerFunc {
	return h.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, `{"error":"GET required"}`, http.StatusMethodNotAllowed)
			return
		}
		appID := r.Header.Get("X-App-ID")
		app := h.GetApp(appID)
		if app == nil {
			writeJSON(w, map[string]any{"ok": false, "error": "app not found"})
			return
		}
		app.mu.Lock()
		backup := map[string]any{
			"app_id":     app.ID,
			"name":       app.Name,
			"contacts":   app.Contacts,
			"history":    app.msgHistory,
			"webhook":    app.WebhookURL,
			"exported_at": time.Now().UTC().Format(time.RFC3339),
		}
		app.mu.Unlock()
		writeJSON(w, backup)
	})
}

// RestoreHandler imports app data from a backup.
// POST /api/transport/v1/backup
func (h *Hub) RestoreHandler() http.HandlerFunc {
	return h.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, `{"error":"POST required"}`, http.StatusMethodNotAllowed)
			return
		}
		appID := r.Header.Get("X-App-ID")
		var backup struct {
			Contacts []int64     `json:"contacts"`
			History  []WSMessage `json:"history"`
			Webhook  string      `json:"webhook"`
		}
		if err := json.NewDecoder(r.Body).Decode(&backup); err != nil {
			http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
			return
		}
		h.mu.Lock()
		app := h.apps[appID]
		if app != nil {
			app.mu.Lock()
			app.Contacts = backup.Contacts
			app.WebhookURL = backup.Webhook
			if len(backup.History) > 0 {
				app.msgHistory = backup.History
			}
			app.mu.Unlock()
			h.save()
		}
		h.mu.Unlock()
		writeJSON(w, map[string]any{"ok": true, "restored": true})
		h.LogAudit(appID, r.Header.Get("X-App-Name"), "restore", fmt.Sprintf("%d contacts", len(backup.Contacts)), 0, true)
	})
}

// DiscoveryHandler finds which app owns a contact.
// GET /api/transport/v1/discover?contact_id=123
func (h *Hub) DiscoveryHandler() http.HandlerFunc {
	return h.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		contactIDStr := r.URL.Query().Get("contact_id")
		contactID := int64(0)
		if n, err := parseInt(contactIDStr); err == nil {
			contactID = int64(n)
		}
		type appInfo struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		var results []appInfo
		if contactID > 0 {
			app := h.AppForContact(contactID)
			if app != nil {
				results = append(results, appInfo{ID: app.ID, Name: app.Name})
			}
		} else {
			// Return all apps with contacts
			for _, a := range h.ListApps() {
				a.mu.Lock()
				if len(a.Contacts) > 0 {
					results = append(results, appInfo{ID: a.ID, Name: a.Name})
				}
				a.mu.Unlock()
			}
		}
		writeJSON(w, map[string]any{"ok": true, "results": results, "count": len(results)})
	})
}

// GatewayHandler proxies API calls through transport.
// POST /api/transport/v1/gateway
func (h *Hub) GatewayHandler() http.HandlerFunc {
	return h.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, `{"error":"POST required"}`, http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Method string `json:"method"`
			Path   string `json:"path"`
			Body   any    `json:"body,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
			return
		}
		if req.Method == "" {
			req.Method = "GET"
		}
		if req.Path == "" {
			http.Error(w, `{"error":"path required"}`, http.StatusBadRequest)
			return
		}
		appID := r.Header.Get("X-App-ID")
		appName := r.Header.Get("X-App-Name")

		bodyBytes, _ := json.Marshal(req.Body)
		targetURL := fmt.Sprintf("http://127.0.0.1:8080%s", req.Path)
		httpReq, err := http.NewRequest(req.Method, targetURL, bytes.NewReader(bodyBytes))
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		httpReq.Header.Set("Content-Type", "application/json")
		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(httpReq)
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			h.LogAudit(appID, appName, "gateway", fmt.Sprintf("%s %s failed: %s", req.Method, req.Path, err.Error()), 0, false)
			return
		}
		defer resp.Body.Close()
		var respData any
		json.NewDecoder(resp.Body).Decode(&respData)
		writeJSON(w, map[string]any{
			"ok":     true,
			"status": resp.StatusCode,
			"body":   respData,
		})
		h.LogAudit(appID, appName, "gateway", fmt.Sprintf("%s %s -> %d", req.Method, req.Path, resp.StatusCode), 0, true)
	})
}
