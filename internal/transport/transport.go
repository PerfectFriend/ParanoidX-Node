// Package transport implements the WebSocket transport API
package transport

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"log/slog"

	"ParanoidX/internal/api"
)

// AuditEntry represents a single audit log entry.
type AuditEntry struct {
	Time      string `json:"time"`
	AppID     string `json:"app_id"`
	AppName   string `json:"app_name"`
	Action    string `json:"action"`
	Detail    string `json:"detail,omitempty"`
	ContactID int64  `json:"contact_id,omitempty"`
	Success   bool   `json:"success"`
}

const (
	maxAuditEntries = 1000
)

const (
	msgHistorySize = 200
	rateLimitBurst = 10
	rateLimitPerSec = 5
)

// App represents a registered application that uses the node as transport.
type App struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	APIKey         string    `json:"api_key"`
	ReconnectToken string    `json:"reconnect_token,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	LastSeen       time.Time `json:"last_seen"`
	RateLimitBurst int       `json:"rate_limit_burst"`
	RateLimitPerSec float64 `json:"rate_limit_per_sec"`
	Contacts       []int64   `json:"contacts,omitempty"`
	WebhookURL     string    `json:"webhook_url,omitempty"`

	mu            sync.Mutex
	msgHistory    []WSMessage
	rateTokens    float64
	rateLast      time.Time
}

// WSMessage is the wire format for transport WebSocket messages.
type WSMessage struct {
	Type      string `json:"type"`
	AppID     string `json:"app_id,omitempty"`
	ContactID int64  `json:"contact_id,omitempty"`
	Text      string `json:"text,omitempty"`
	MsgID     string `json:"msg_id,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
	Payload   any    `json:"payload,omitempty"`
}

// Hub is the transport hub that manages app registrations and WebSocket connections.
type Hub struct {
	mu          sync.RWMutex
	apps        map[string]*App
	apiKeys     map[string]string
	reconnectMap map[string]string
	wsClients   map[string][]chan WSMessage
	persistPath string
	dataDir     string
	serverURL   string
	auditLog    []AuditEntry
	auditPath   string
	stats       struct {
		sync.Mutex
		messagesSent     int64
		messagesReceived int64
		wsConnections    int64
		authFailures     int64
		rateLimitHits    int64
		errors           int64
	}
}

var GlobalTransport *Hub


// NewHub handles the NewHub HTTP request.
func NewHub(dataDir string) *Hub {
	path := filepath.Join(dataDir, "transport_apps.json")
	auditPath := filepath.Join(dataDir, "transport_audit.json")
	h := &Hub{
		apps:         make(map[string]*App),
		apiKeys:      make(map[string]string),
		reconnectMap: make(map[string]string),
		wsClients:    make(map[string][]chan WSMessage),
		persistPath:  path,
		auditPath:    auditPath,
		dataDir:      dataDir,
		serverURL:    fmt.Sprintf("http://127.0.0.1:8080"),
		auditLog:     make([]AuditEntry, 0, maxAuditEntries),
	}
	h.load()
	h.loadAudit()
	GlobalTransport = h
	slog.Info("[transport] hub initialized", "apps", len(h.apps))
	return h
}

func (h *Hub) load() {
	b, err := os.ReadFile(h.persistPath)
	if err != nil {
		return
	}
	var apps []*App
	if err := json.Unmarshal(b, &apps); err != nil {
		return
	}
	for _, a := range apps {
		a.RateLimitBurst = rateLimitBurst
		a.RateLimitPerSec = rateLimitPerSec
		a.rateLast = time.Now()
		a.rateTokens = float64(a.RateLimitBurst)
		if a.msgHistory == nil {
			a.msgHistory = make([]WSMessage, 0, msgHistorySize)
		}
		if a.Contacts == nil {
			a.Contacts = make([]int64, 0)
		}
		h.apps[a.ID] = a
		h.apiKeys[a.APIKey] = a.ID
		if a.ReconnectToken != "" {
			h.reconnectMap[a.ReconnectToken] = a.ID
		}
	}
}

func (h *Hub) save() {
	apps := make([]*App, 0, len(h.apps))
	for _, a := range h.apps {
		apps = append(apps, a)
	}
	b, _ := json.Marshal(apps)
	os.WriteFile(h.persistPath, b, 0600)
}

func (h *Hub) loadAudit() {
	b, err := os.ReadFile(h.auditPath)
	if err != nil {
		return
	}
	var entries []AuditEntry
	if json.Unmarshal(b, &entries) == nil {
		if len(entries) > maxAuditEntries {
			entries = entries[len(entries)-maxAuditEntries:]
		}
		h.auditLog = entries
	}
}

func (h *Hub) saveAudit() {
	b, _ := json.Marshal(h.auditLog)
	os.WriteFile(h.auditPath, b, 0600)
}

// LogAudit records an audit event.
func (h *Hub) LogAudit(appID, appName, action, detail string, contactID int64, success bool) {
	entry := AuditEntry{
		Time:      time.Now().UTC().Format(time.RFC3339),
		AppID:     appID,
		AppName:   appName,
		Action:    action,
		Detail:    detail,
		ContactID: contactID,
		Success:   success,
	}
	h.mu.Lock()
	h.auditLog = append(h.auditLog, entry)
	if len(h.auditLog) > maxAuditEntries {
		h.auditLog = h.auditLog[len(h.auditLog)-maxAuditEntries:]
	}
	h.mu.Unlock()
	h.saveAudit()
}

// GetAuditLog returns recent audit entries.
func (h *Hub) GetAuditLog(limit int) []AuditEntry {
	if limit <= 0 || limit > maxAuditEntries {
		limit = maxAuditEntries
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	start := 0
	if len(h.auditLog) > limit {
		start = len(h.auditLog) - limit
	}
	result := make([]AuditEntry, len(h.auditLog)-start)
	copy(result, h.auditLog[start:])
	return result
}

// SetServerURL sets the public URL for remote app configuration.
func (h *Hub) SetServerURL(url string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.serverURL = url
}

func generateAPIKey() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func generateAppID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func generateReconnectToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// Register creates a new app registration with a unique API key.
// Returns the App pointer directly (safe under h.mu lock — caller does not
// retain the pointer beyond the immediate response serialization).
func (h *Hub) Register(name string) (*App, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for _, a := range h.apps {
		if a.Name == name {
			a.LastSeen = time.Now()
			return a, nil
		}
	}

	app := &App{
		ID:              generateAppID(),
		Name:            name,
		APIKey:          generateAPIKey(),
		ReconnectToken:  generateReconnectToken(),
		CreatedAt:       time.Now(),
		LastSeen:        time.Now(),
		RateLimitBurst:  rateLimitBurst,
		RateLimitPerSec: rateLimitPerSec,
		rateLast:        time.Now(),
		rateTokens:      float64(rateLimitBurst),
		msgHistory:      make([]WSMessage, 0, msgHistorySize),
	}
	h.apps[app.ID] = app
	h.apiKeys[app.APIKey] = app.ID
	h.reconnectMap[app.ReconnectToken] = app.ID
	h.save()

	slog.Info("[transport] app registered", "name", name, "id", app.ID)
	return app, nil
}

// Authenticate validates an API key and returns the app.
func (h *Hub) Authenticate(apiKey string) *App {
	if apiKey == "" {
		return nil
	}
	h.mu.RLock()
	appID, ok := h.apiKeys[apiKey]
	h.mu.RUnlock()
	if !ok {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	app := h.apps[appID]
	if app != nil {
		app.LastSeen = time.Now()
	}
	return app
}

// AuthenticateReconnect validates a reconnect token and returns the app.
func (h *Hub) AuthenticateReconnect(token string) *App {
	if token == "" {
		return nil
	}
	h.mu.RLock()
	appID, ok := h.reconnectMap[token]
	h.mu.RUnlock()
	if !ok {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.apps[appID]
}

// Subscribe creates a WS message channel for an app.
func (h *Hub) Subscribe(appID string) chan WSMessage {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch := make(chan WSMessage, 100)
	h.wsClients[appID] = append(h.wsClients[appID], ch)
	return ch
}

// Unsubscribe removes a WS message channel for an app.
func (h *Hub) Unsubscribe(appID string, ch chan WSMessage) {
	h.mu.Lock()
	defer h.mu.Unlock()
	clients := h.wsClients[appID]
	for i, c := range clients {
		if c == ch {
			h.wsClients[appID] = append(clients[:i], clients[i+1:]...)
			break
		}
	}
}

// AddToHistory stores a message in the app's ring buffer for replay.
func (h *Hub) AddToHistory(appID string, msg WSMessage) {
	h.mu.RLock()
	app := h.apps[appID]
	h.mu.RUnlock()
	if app == nil {
		return
	}
	app.mu.Lock()
	app.msgHistory = append(app.msgHistory, msg)
	if len(app.msgHistory) > msgHistorySize {
		app.msgHistory = app.msgHistory[len(app.msgHistory)-msgHistorySize:]
	}
	app.mu.Unlock()
}

// ReplayHistory sends missed messages to a newly connected WS client.
func (h *Hub) ReplayHistory(appID string, ch chan WSMessage) {
	h.mu.RLock()
	app := h.apps[appID]
	h.mu.RUnlock()
	if app == nil {
		return
	}
	app.mu.Lock()
	history := make([]WSMessage, len(app.msgHistory))
	copy(history, app.msgHistory)
	app.mu.Unlock()
	for _, msg := range history {
		select {
		case ch <- msg:
		default:
		}
	}
}

// BroadcastToApp sends a message to all WS subscribers of an app
// and pushes via webhook if configured.
func (h *Hub) BroadcastToApp(appID string, msg WSMessage) {
	h.mu.RLock()
	clients := h.wsClients[appID]
	app := h.apps[appID]
	var webhookURL string
	if app != nil {
		webhookURL = app.WebhookURL
	}
	h.mu.RUnlock()

	// WS broadcast
	for _, ch := range clients {
		select {
		case ch <- msg:
		default:
		}
	}

	// Webhook push
	if webhookURL != "" {
		go h.webhookPush(webhookURL, appID, msg)
	}

	h.AddToHistory(appID, msg)
}

// webhookPush sends a message to an app's webhook URL.
func (h *Hub) webhookPush(url, appID string, msg WSMessage) {
	body, err := json.Marshal(map[string]any{
		"type":       "transport.message",
		"app_id":     appID,
		"message":    msg,
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		slog.Warn("[transport] webhook push failed", "app", appID, "error", err)
		return
	}
	resp.Body.Close()
}

// BroadcastAll sends a message to all registered apps (not just connected ones).
func (h *Hub) BroadcastAll(msg WSMessage) {
	h.mu.RLock()
	appIDs := make([]string, 0, len(h.wsClients))
	for appID := range h.wsClients {
		appIDs = append(appIDs, appID)
	}
	allApps := make([]string, 0, len(h.apps))
	for appID := range h.apps {
		allApps = append(allApps, appID)
	}
	h.mu.RUnlock()

	// Broadcast to connected apps
	for _, appID := range appIDs {
		h.BroadcastToApp(appID, msg)
	}
	// Add to history for disconnected apps too
	for _, appID := range allApps {
		h.AddToHistory(appID, msg)
	}
}

// SendThroughBridge sends a message through the SimpleX bridge.
func (h *Hub) SendThroughBridge(contactID int64, text string) error {
	if contactID <= 0 {
		return fmt.Errorf("invalid contact_id")
	}
	if api.SimplexCmd == nil {
		return fmt.Errorf("bridge not available")
	}
	cmd := fmt.Sprintf("/_send @%d json [{\"msgContent\":{\"type\":\"text\",\"text\":%s}}]",
		contactID, mustJSON(text))
	_, err := api.SimplexCmd(cmd)
	if err != nil {
		return fmt.Errorf("bridge send: %w", err)
	}
	return nil
}

func mustJSON(v string) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// ListApps returns all registered apps.
func (h *Hub) ListApps() []*App {
	h.mu.RLock()
	defer h.mu.RUnlock()
	apps := make([]*App, 0, len(h.apps))
	for _, a := range h.apps {
		apps = append(apps, a)
	}
	return apps
}

// GetApp returns an app by ID.
func (h *Hub) GetApp(id string) *App {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.apps[id]
}

// CheckRateLimit returns true if the app is allowed to send a message.
func (h *Hub) CheckRateLimit(appID string) bool {
	h.mu.RLock()
	app := h.apps[appID]
	h.mu.RUnlock()
	if app == nil {
		return false
	}
	app.mu.Lock()
	defer app.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(app.rateLast).Seconds()
	app.rateTokens = min(app.rateTokens+elapsed*app.RateLimitPerSec, float64(app.RateLimitBurst))
	app.rateLast = now

	if app.rateTokens < 1 {
		return false
	}
	app.rateTokens--
	return true
}

// AddContact registers a contact for an app.
func (h *Hub) AddContact(appID string, contactID int64) {
	h.mu.RLock()
	app := h.apps[appID]
	h.mu.RUnlock()
	if app == nil {
		return
	}
	app.mu.Lock()
	defer app.mu.Unlock()
	for _, c := range app.Contacts {
		if c == contactID {
			return
		}
	}
	app.Contacts = append(app.Contacts, contactID)
	h.save()
}

// RemoveContact removes a contact from an app.
func (h *Hub) RemoveContact(appID string, contactID int64) {
	h.mu.RLock()
	app := h.apps[appID]
	h.mu.RUnlock()
	if app == nil {
		return
	}
	app.mu.Lock()
	defer app.mu.Unlock()
	for i, c := range app.Contacts {
		if c == contactID {
			app.Contacts = append(app.Contacts[:i], app.Contacts[i+1:]...)
			break
		}
	}
	h.save()
}

// AppForContact returns the first app that owns a given contact.
func (h *Hub) AppForContact(contactID int64) *App {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, app := range h.apps {
		app.mu.Lock()
		for _, c := range app.Contacts {
			if c == contactID {
				app.mu.Unlock()
				return app
			}
		}
		app.mu.Unlock()
	}
	return nil
}

// Config returns the transport config for remote app connections.
func (h *Hub) Config() map[string]any {
	h.mu.RLock()
	url := h.serverURL
	h.mu.RUnlock()
	return map[string]any{
		"ok":               true,
		"server_url":       url,
		"ws_url":           url + "/api/transport/v1/ws",
		"wss_url":          "",
		"version":          "transport-v1",
		"features":         []string{"ws_reconnect", "rate_limiting", "contact_routing", "message_history"},
		"bridge_connected": api.BridgeConnected,
	}
}

// Stats returns detailed transport stats.
func (h *Hub) Stats() map[string]any {
	h.mu.RLock()
	appCount := len(h.apps)
	wsCount := 0
	totalMsgs := 0
	totalContacts := 0
	for _, clients := range h.wsClients {
		wsCount += len(clients)
	}
	for _, app := range h.apps {
		app.mu.Lock()
		totalMsgs += len(app.msgHistory)
		totalContacts += len(app.Contacts)
		app.mu.Unlock()
	}
	auditCount := len(h.auditLog)
	h.mu.RUnlock()

	h.stats.Lock()
	msgsSent := h.stats.messagesSent
	msgsRecv := h.stats.messagesReceived
	authFail := h.stats.authFailures
	rateHits := h.stats.rateLimitHits
	errs := h.stats.errors
	wsConns := h.stats.wsConnections
	h.stats.Unlock()

	healthScore := 100
	if !api.BridgeConnected {
		healthScore -= 30
	}
	if rateHits > 100 {
		healthScore -= 20
	}
	if errs > 10 {
		healthScore -= 20
	}
	if healthScore < 0 {
		healthScore = 0
	}

	return map[string]any{
		"ok":                    true,
		"registered_apps":       appCount,
		"active_ws_connections": wsCount,
		"total_history_msgs":    totalMsgs,
		"total_contacts":        totalContacts,
		"audit_log_entries":     auditCount,
		"stats": map[string]any{
			"messages_sent":     msgsSent,
			"messages_received": msgsRecv,
			"auth_failures":     authFail,
			"rate_limit_hits":   rateHits,
			"errors":            errs,
			"ws_connections":    wsConns,
		},
		"bridge": map[string]any{
			"connected":  api.BridgeConnected,
			"reconnects": api.BridgeReconnectCount,
		},
		"health_score": healthScore,
		"health":       healthStatus(healthScore),
	}
}

func healthStatus(score int) string {
	if score >= 80 {
		return "healthy"
	}
	if score >= 50 {
		return "degraded"
	}
	return "unhealthy"
}

// Status returns transport hub status.
func (h *Hub) Status() map[string]any {
	h.mu.RLock()
	appCount := len(h.apps)
	wsCount := 0
	for _, clients := range h.wsClients {
		wsCount += len(clients)
	}
	h.mu.RUnlock()

	return map[string]any{
		"ok":                   true,
		"registered_apps":      appCount,
		"active_ws_connections": wsCount,
		"bridge_connected":     api.BridgeConnected,
	}
}

// ClearAppHistory clears the message history for an app.
func (h *Hub) ClearAppHistory(appID string) {
	h.mu.RLock()
	app := h.apps[appID]
	h.mu.RUnlock()
	if app == nil {
		return
	}
	app.mu.Lock()
	app.msgHistory = make([]WSMessage, 0, msgHistorySize)
	app.mu.Unlock()
}
