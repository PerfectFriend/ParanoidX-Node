// Package api provides HTTP handlers and API endpoints for the simplex-node server
package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"ParanoidX/internal/middleware"
	"ParanoidX/internal/store"
)

// MsgStatus represents the delivery status of a chat message.
type MsgStatus string

const (
	StatusSending   MsgStatus = "sending"
	StatusSent      MsgStatus = "sent"
	StatusDelivered MsgStatus = "delivered"
	StatusFailed    MsgStatus = "failed"
)

// ChatMessage represents a single message in the chat history.
type ChatMessage struct {
	ID        string            `json:"id"`
	From      string            `json:"from"`
	Text      string            `json:"text"`
	Timestamp string            `json:"timestamp"`
	IsUser    bool              `json:"is_user"`
	ChatID    string            `json:"chat_id"`
	Status    MsgStatus         `json:"status"`
	ReplyToID string            `json:"reply_to_id,omitempty"`
	Pinned    bool              `json:"pinned,omitempty"`
	Reactions map[string]string `json:"reactions,omitempty"`

	// Cycle 1: Money transfer
	MoneyAmount *float64 `json:"money_amount,omitempty"`
	MoneyAsset  string   `json:"money_asset,omitempty"`
	MoneyTxID   string   `json:"money_tx_id,omitempty"`

	// Cycle 3: Voice message
	VoiceURL string `json:"voice_url,omitempty"`
	VoiceDur int    `json:"voice_duration,omitempty"`

	// Cycle 8: File attachments
	FileName string `json:"file_name,omitempty"`
	FileURL  string `json:"file_url,omitempty"`
	FileSize int64  `json:"file_size,omitempty"`

	// Cycle 11: Read receipt
	ReadAt  string `json:"read_at,omitempty"`
	DeliverAt string `json:"delivered_at,omitempty"`

	// Cycle 12: Recall
	Recalled   bool   `json:"recalled,omitempty"`
	RecalledAt string `json:"recalled_at,omitempty"`

	// Cycle 20: Encryption indicator
	Encryption string `json:"encryption,omitempty"` // e2e, none, unknown
}

// ChatHub manages in-memory chat messages, SSE streaming, and persistence.
type ChatHub struct {
	mu         sync.RWMutex
	messages   []ChatMessage
	sseClients map[chan ChatMessage]struct{}
	filePath   string
	ChatStore *store.ChatStore
}

// MaxChatHistory caps the number of messages retained in memory.
const MaxChatHistory = 500

// GlobalChatHub is the singleton chat hub instance used across the server.
var GlobalChatHub = NewChatHub()

// GlobalAutoArchiveDays is the default age (in days) at which messages are auto-archived.
var GlobalAutoArchiveDays = 90

// NewChatHub creates a new ChatHub with initialized message buffer and SSE client map.
func NewChatHub() *ChatHub {
	return &ChatHub{
		messages:   make([]ChatMessage, 0, MaxChatHistory),
		sseClients: make(map[chan ChatMessage]struct{}),
	}
}

// WithFile loads persisted chat messages from a JSON file and enables auto-flush.
func (h *ChatHub) WithFile(path string) *ChatHub {
	h.filePath = path
	b, err := os.ReadFile(path)
	if err == nil {
		var msgs []ChatMessage
		if json.Unmarshal(b, &msgs) == nil && msgs != nil {
			h.messages = msgs
		}
	}
	return h
}

func (h *ChatHub) flush() {
	if h.filePath == "" {
		return
	}
	b, _ := json.Marshal(h.messages)
	os.WriteFile(h.filePath, b, 0600)
}

// AddMessage appends a message to the hub, persists it, broadcasts to SSE clients,
// indexes it for search, evaluates auto-reply rules, and auto-reads after 30s for user messages.
func (h *ChatHub) AddMessage(msg ChatMessage) {
	h.mu.Lock()
	h.messages = append(h.messages, msg)
	if len(h.messages) > MaxChatHistory {
		excess := len(h.messages) - MaxChatHistory
		h.messages = h.messages[excess:]
	}
	clients := make([]chan ChatMessage, 0, len(h.sseClients))
	for ch := range h.sseClients {
		clients = append(clients, ch)
	}
	h.mu.Unlock()

	h.flush()

	// Persist to SQLite ChatStore if available
	if h.ChatStore != nil {
		stored := store.StoredMessage{
			ID:        msg.ID,
			ChatID:    msg.ChatID,
			Sender:    msg.From,
			Text:      msg.Text,
			Timestamp: msg.Timestamp,
			Status:    string(msg.Status),
			Metadata:  "{}",
			IsUser:    msg.IsUser,
			Pinned:    msg.Pinned,
		}
		if err := h.ChatStore.AddMessage(stored); err != nil {
			slog.Error("chat store add", "error", err)
		}
	}

	for _, ch := range clients {
		select {
		case ch <- msg:
		default:
		}
	}

	// Index message in inverted search index
	if GlobalInvertedIndex != nil {
		GlobalInvertedIndex.AddMessage(msg)
	}

	// Auto-reply: match incoming messages against rules
	if !msg.IsUser && msg.Text != "" && SimplexCmd != nil {
		if reply, ok := matchAutoReply(msg.Text); ok {
			cid := parseChatID(msg.ChatID)
			if cid > 0 {
				go func() {
					SimplexCmd(fmt.Sprintf("/_send %d %s", cid, reply))
				}()
			}
		}
	}

	// Cycle 14: Auto-mark unread messages as read after 30s when user sends a message
	if msg.IsUser {
		chatID := msg.ChatID
		time.AfterFunc(30*time.Second, func() {
			h.mu.Lock()
			now := time.Now().UTC().Format(time.RFC3339)
			for i, m := range h.messages {
				if m.ChatID == chatID && !m.IsUser && m.ReadAt == "" {
					h.messages[i].ReadAt = now
					h.messages[i].Status = StatusDelivered
				}
			}
			h.mu.Unlock()
			h.flush()
		})
	}
}

// ClearMessages removes all messages from the hub and persists the empty state.
func (h *ChatHub) ClearMessages() {
	h.mu.Lock()
	h.messages = make([]ChatMessage, 0)
	h.mu.Unlock()
	h.flush()
}

// DeleteMessage removes a single message by ID from the hub.
func (h *ChatHub) DeleteMessage(id string) {
	h.mu.Lock()
	for i, m := range h.messages {
		if m.ID == id {
			h.messages = append(h.messages[:i], h.messages[i+1:]...)
			break
		}
	}
	h.mu.Unlock()
	h.flush()
}

// EditMessageText replaces the text of a message by ID and updates its timestamp.
func (h *ChatHub) EditMessageText(id, newText string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i, m := range h.messages {
		if m.ID == id {
			h.messages[i].Text = newText
			h.messages[i].Timestamp = time.Now().UTC().Format(time.RFC3339)
			h.flush()
			return true
		}
	}
	return false
}

// TogglePin flips the pinned state of a message by ID.
func (h *ChatHub) TogglePin(id string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i, m := range h.messages {
		if m.ID == id {
			h.messages[i].Pinned = !h.messages[i].Pinned
			h.flush()
			return true
		}
	}
	return false
}

// GetPinned returns all messages that have been pinned.
func (h *ChatHub) GetPinned() []ChatMessage {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]ChatMessage, 0)
	for _, m := range h.messages {
		if m.Pinned {
			out = append(out, m)
		}
	}
	return out
}

// ToggleReaction adds or removes a reaction emoji from a message for a given user.
func (h *ChatHub) ToggleReaction(id, emoji, user string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i, m := range h.messages {
		if m.ID == id {
			if h.messages[i].Reactions == nil {
				h.messages[i].Reactions = make(map[string]string)
			}
			if existing, ok := h.messages[i].Reactions[emoji]; ok && existing == user {
				delete(h.messages[i].Reactions, emoji)
				h.flush()
				return "removed"
			}
			h.messages[i].Reactions[emoji] = user
			h.flush()
			return "added"
		}
	}
	return "not found"
}

// DeleteContactMessages removes all messages associated with a given chat/contact ID.
func (h *ChatHub) DeleteContactMessages(chatID string) {
	h.mu.Lock()
	filtered := make([]ChatMessage, 0, len(h.messages))
	for _, m := range h.messages {
		if m.ChatID != chatID {
			filtered = append(filtered, m)
		}
	}
	h.messages = filtered
	h.mu.Unlock()
	h.flush()
}

// UpdateMessageStatus updates the delivery status of a message by ID.
func (h *ChatHub) UpdateMessageStatus(id string, status MsgStatus) {
	h.mu.Lock()
	for i, m := range h.messages {
		if m.ID == id {
			h.messages[i].Status = status
			break
		}
	}
	h.mu.Unlock()
	h.flush()
}

// MessageCount returns the total number of messages in the hub.
func (h *ChatHub) MessageCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.messages)
}

// SSEClientCount returns the number of currently connected SSE streaming clients.
func (h *ChatHub) SSEClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.sseClients)
}

// GetMessages returns a copy of all messages currently in the hub.
func (h *ChatHub) GetMessages() []ChatMessage {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]ChatMessage, len(h.messages))
	copy(out, h.messages)
	return out
}

// ArchiveOldMessages moves messages older than N days to a dated archive file and replaces them with placeholders.
func (h *ChatHub) ArchiveOldMessages(dataDir string, days int) (int, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	cutoff := time.Now().AddDate(0, 0, -days)
	archiveDir := filepath.Join(dataDir, "archives")
	os.MkdirAll(archiveDir, 0755)

	var archived []ChatMessage
	var kept []ChatMessage

	for _, m := range h.messages {
		t, err := time.Parse(time.RFC3339, m.Timestamp)
		if err != nil || t.After(cutoff) {
			kept = append(kept, m)
		} else {
			archived = append(archived, m)
		}
	}

	if len(archived) == 0 {
		return 0, nil
	}

	dateStr := time.Now().Format("20060102")
	archivePath := filepath.Join(archiveDir, "chat_archive_"+dateStr+".json")

	existing := make([]ChatMessage, 0)
	if b, err := os.ReadFile(archivePath); err == nil {
		json.Unmarshal(b, &existing)
	}
	existing = append(existing, archived...)

	b, _ := json.MarshalIndent(existing, "", "  ")
	if err := os.WriteFile(archivePath, b, 0600); err != nil {
		return 0, err
	}

	// Replace archived messages with placeholders
	for _, m := range archived {
		kept = append(kept, ChatMessage{
			ID:        m.ID,
			Text:      "[archived]",
			From:      m.From,
			Timestamp: m.Timestamp,
			IsUser:    m.IsUser,
			ChatID:    m.ChatID,
			Status:    m.Status,
		})
	}

	h.messages = kept
	h.flush()

	slog.Info("chat archive", "archived", len(archived), "kept", len(kept), "file", archivePath)
	return len(archived), nil
}

// RestoreArchive loads messages from a dated archive file back into the hub, replacing placeholders.
func (h *ChatHub) RestoreArchive(dataDir, date string) (int, error) {
	archivePath := filepath.Join(dataDir, "archives", "chat_archive_"+date+".json")
	b, err := os.ReadFile(archivePath)
	if err != nil {
		return 0, fmt.Errorf("archive not found: %s", date)
	}

	var archived []ChatMessage
	if err := json.Unmarshal(b, &archived); err != nil {
		return 0, fmt.Errorf("parse error: %w", err)
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	// Replace placeholders with restored messages
	restored := 0
	for i, m := range h.messages {
		if m.Text == "[archived]" {
			for _, a := range archived {
				if a.ID == m.ID {
					h.messages[i] = a
					restored++
					break
				}
			}
		}
	}

	// Also append any archived messages not found as placeholders
	existingIDs := make(map[string]bool)
	for _, m := range h.messages {
		existingIDs[m.ID] = true
	}
	for _, a := range archived {
		if !existingIDs[a.ID] {
			h.messages = append(h.messages, a)
			restored++
		}
	}

	h.flush()
	slog.Info("chat archive restore", "date", date, "restored", restored)
	return restored, nil
}

// ListArchives returns a sorted list of available archive dates from the archives directory.
func (h *ChatHub) ListArchives(dataDir string) ([]string, error) {
	archiveDir := filepath.Join(dataDir, "archives")
	entries, err := os.ReadDir(archiveDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	var archives []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "chat_archive_") {
			archives = append(archives, strings.TrimPrefix(strings.TrimSuffix(e.Name(), ".json"), "chat_archive_"))
		}
	}
	sort.Strings(archives)
	return archives, nil
}

// ArchiveStats returns aggregate statistics about archived messages (file count, total messages).
func (h *ChatHub) ArchiveStats(dataDir string) map[string]int {
	archives, _ := h.ListArchives(dataDir)
	stats := map[string]int{"files": len(archives)}
	totalMessages := 0
	for _, date := range archives {
		path := filepath.Join(dataDir, "archives", "chat_archive_"+date+".json")
		b, _ := os.ReadFile(path)
		var msgs []ChatMessage
		if json.Unmarshal(b, &msgs) == nil {
			totalMessages += len(msgs)
		}
	}
	stats["total_messages"] = totalMessages
	return stats
}

func (h *ChatHub) subscribe() chan ChatMessage {
	ch := make(chan ChatMessage, 64)
	h.mu.Lock()
	h.sseClients[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *ChatHub) unsubscribe(ch chan ChatMessage) {
	h.mu.Lock()
	delete(h.sseClients, ch)
	h.mu.Unlock()
}

// LoadPersisted reads all chat-related state (auto-reply, groups, labels, drafts, webhook, templates) from disk.
func (h *ChatHub) LoadPersisted(dataDir string) {
	p := store.NewChatPersistence(dataDir)
	var ar []AutoReplyRule
	if p.Load("auto_reply", &ar) == nil {
		autoReplyRules = ar
	}
	var gr []ContactGroup
	if p.Load("groups", &gr) == nil {
		groups = gr
	}
	var gs int64
	if p.Load("groups_seq", &gs) == nil {
		groupsSeq = gs
	}
	var lbls struct {
		Msg  map[string][]string `json:"msg"`
		Chat map[string][]string `json:"chat"`
	}
	if p.Load("labels", &lbls) == nil {
		msgLabels = lbls.Msg
		chatLabels = lbls.Chat
	}
	var dr map[string]string
	if p.Load("drafts", &dr) == nil {
		drafts = dr
	}
	var wc WebhookConfig
	if p.Load("webhook", &wc) == nil {
		webhookCfg = wc
	}
	var tmpl []MessageTemplate
	if p.Load("templates", &tmpl) == nil {
		templates = tmpl
	}
	var ts int64
	if p.Load("templates_seq", &ts) == nil {
		templatesSeq = ts
	}

	// Optionally seed in-memory messages from SQLite ChatStore
	if h.ChatStore != nil {
		if all, err := h.ChatStore.GetAllMessages(); err == nil && len(all) > 0 {
			msgs := make([]ChatMessage, 0, len(all))
			for _, sm := range all {
				msgs = append(msgs, ChatMessage{
					ID:        sm.ID,
					From:      sm.Sender,
					Text:      sm.Text,
					Timestamp: sm.Timestamp,
					IsUser:    sm.IsUser,
					ChatID:    sm.ChatID,
					Status:    MsgStatus(sm.Status),
					Pinned:    sm.Pinned,
				})
			}
			h.mu.Lock()
			h.messages = msgs
			if len(h.messages) > MaxChatHistory {
				h.messages = h.messages[len(h.messages)-MaxChatHistory:]
			}
			h.mu.Unlock()
			slog.Info("loaded messages from SQLite ChatStore", "count", len(msgs))
		}
	}
}

// SaveAll persists all chat-related state (auto-reply, groups, labels, drafts, webhook, templates) to disk.
func (h *ChatHub) SaveAll(dataDir string) {
	p := store.NewChatPersistence(dataDir)
	p.Save("auto_reply", autoReplyRules)
	p.Save("groups", groups)
	p.Save("groups_seq", groupsSeq)
	p.Save("labels", map[string]interface{}{"msg": msgLabels, "chat": chatLabels})
	p.Save("drafts", drafts)
	p.Save("webhook", webhookCfg)
	p.Save("templates", templates)
	p.Save("templates_seq", templatesSeq)
}

// GET /api/chat/history
func ChatHistoryHandler(hub *ChatHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		chatID := r.URL.Query().Get("chat_id")
		messages := hub.GetMessages()
		if chatID != "" {
			filtered := make([]ChatMessage, 0)
			for _, m := range messages {
				if m.ChatID == chatID {
					filtered = append(filtered, m)
				}
			}
			writeJSON(w, map[string]any{"messages": filtered})
			return
		}
		writeJSON(w, map[string]any{"messages": messages})
	}
}

// POST /api/chat/delete/contact — delete all messages for a contact
func ChatDeleteContactHandler(hub *ChatHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		if !requireContentType(r) {
			writeJSON(w, map[string]any{"ok": false, "error": "Content-Type must be application/json"})
			return
		}
		var req struct {
			ChatID string `json:"chat_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
			return
		}
		if req.ChatID == "" {
			writeJSON(w, map[string]any{"ok": false, "error": "chat_id required"})
			return
		}
		hub.DeleteContactMessages(req.ChatID)
		writeJSON(w, map[string]any{"ok": true})
	}
}

// POST /api/chat/delete — delete message by id
func ChatDeleteHandler(hub *ChatHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		if !requireContentType(r) {
			writeJSON(w, map[string]any{"ok": false, "error": "Content-Type must be application/json"})
			return
		}
		var req struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
			return
		}
		if req.ID == "" {
			writeJSON(w, map[string]any{"ok": false, "error": "id required"})
			return
		}
		hub.DeleteMessage(req.ID)
		auditInfo("message_deleted", fmt.Sprintf("id=%s", req.ID))
		writeJSON(w, map[string]any{"ok": true})
	}
}

// POST /api/chat/clear
func ChatClearHandler(hub *ChatHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		hub.ClearMessages()
		writeJSON(w, map[string]any{"ok": true})
	}
}

// GET /api/chat/stream — SSE endpoint
func ChatStreamHandler(hub *ChatHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		ch := hub.subscribe()
		defer hub.unsubscribe(ch)

		fmt.Fprintf(w, "event: connected\ndata: {}\n\n")
		flusher.Flush()

		for {
			select {
			case <-r.Context().Done():
				return
			case msg := <-ch:
				data, _ := json.Marshal(msg)
				fmt.Fprintf(w, "event: message\ndata: %s\n\n", data)
				flusher.Flush()
			case <-time.After(30 * time.Second):
				fmt.Fprintf(w, ": keepalive\n\n")
				flusher.Flush()
			}
		}
	}
}

// GET /api/chat/status
func ChatStatusHandler(hub *ChatHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		contactCount := 0
		if SimplexCmd != nil && BridgeConnected {
			resp, err := SimplexCmd("/_contacts 1")
			if err == nil {
				rType, _ := resp["type"].(string)
				if rType == "contactsList" {
					contacts, _ := resp["contacts"].([]any)
					contactCount = len(contacts)
				}
			}
		}
		writeJSON(w, map[string]any{
			"bridge":           BridgeConnected,
			"contacts":         contactCount,
			"messages":         hub.MessageCount(),
			"simplex_cmd":      SimplexCmd != nil,
			"reconnect_count":  BridgeReconnectCount,
		})
	}
}

// BridgeSendFunc sends a text message to a SimpleX contact via WS.
// Set by the bridge package.
// BridgeSendFunc sends a text message to a SimpleX contact via WS.
// Set by the bridge package.
var BridgeSendFunc func(text string, userID, contactID int64)

var chatSendLimiter = middleware.NewRateLimiter(5, 10, time.Second)

// Cycle 13: Per-contact rate limiter
var (
	contactRateLimit = make(map[int64]time.Time)
	contactRateMu    sync.Mutex
)

// GetChatSendLimiter returns the global chat send rate limiter instance.
func GetChatSendLimiter() *middleware.RateLimiter {
	return chatSendLimiter
}

// POST /api/chat/send
func ChatSendHandler(hub *ChatHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		if !requireContentType(r) {
			w.WriteHeader(400)
			writeJSON(w, map[string]any{"ok": false, "error": "Content-Type must be application/json"})
			return
		}
		ip := r.RemoteAddr
		if !chatSendLimiter.Allow(ip) {
			http.Error(w, "429 Too Many Requests", http.StatusTooManyRequests)
			return
		}
		var req struct {
			Text      string `json:"text"`
			ContactID int64  `json:"contact_id"`
			ChatID    string `json:"chat_id"`
			ReplyToID string `json:"reply_to_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad json", 400)
			return
		}
		if req.Text == "" {
			http.Error(w, "text required", 400)
			return
		}

		if GlobalContentFilter != nil {
			filtered, note, blocked := GlobalContentFilter.Filter(req.Text)
			if blocked {
				logAudit("content_filter_blocked", "chat", "blocked message: "+note)
				writeJSON(w, map[string]any{"ok": false, "error": "message blocked by content filter"})
				return
			}
			if filtered != req.Text {
				req.Text = filtered
				logAudit("content_filter_replaced", "chat", note)
			}
		}

		// Cycle 13: Per-contact rate limiter
		contactRateMu.Lock()
		if last, ok := contactRateLimit[req.ContactID]; ok && time.Since(last) < 500*time.Millisecond {
			contactRateMu.Unlock()
			http.Error(w, "rate limited", http.StatusTooManyRequests)
			return
		}
		contactRateLimit[req.ContactID] = time.Now()
		contactRateMu.Unlock()
		go func() {
			contactRateMu.Lock()
			for cid, ts := range contactRateLimit {
				if time.Since(ts) > 10*time.Second {
					delete(contactRateLimit, cid)
				}
			}
			contactRateMu.Unlock()
		}()

		sendText := req.Text
		if req.ReplyToID != "" {
			hub.mu.RLock()
			for _, m := range hub.messages {
				if m.ID == req.ReplyToID {
					prefix := ""
					if len(m.Text) > 80 {
						prefix = m.Text[:80] + "…"
					} else {
						prefix = m.Text
					}
					sendText = fmt.Sprintf("> %s\n\n%s", prefix, req.Text)
					break
				}
			}
			hub.mu.RUnlock()
		}

		msg := ChatMessage{
			ID:        fmt.Sprintf("msg-%d", time.Now().UnixNano()),
			From:      "admin",
			Text:      sendText,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			IsUser:    true,
			ChatID:    req.ChatID,
			Status:    StatusSending,
			ReplyToID: req.ReplyToID,
		}
		hub.AddMessage(msg)

		if BridgeSendFunc != nil && req.ContactID > 0 {
			BridgeSendFunc(req.Text, 1, req.ContactID)
			msg.Status = StatusSent
			hub.UpdateMessageStatus(msg.ID, StatusSent)
		} else {
			msg.Status = StatusSent
		}

		auditInfo("message_sent", fmt.Sprintf("ChatID=%s, text_len=%d", req.ChatID, len(req.Text)))
		writeJSON(w, map[string]any{"ok": true, "message": msg})
	}
}

// SimplexCmd iface — set by bridge for HTTP handlers to send commands to WS.
// SimplexCmd is the function to send a command to the SimpleX chat CLI via WebSocket.
// Set by the bridge package.
var SimplexCmd func(cmd string) (map[string]any, error)

// BridgeConnected tracks WS connection state, set by bridge.
var BridgeConnected bool

// BridgeReconnectCount tracks total reconnections, set by bridge.
var BridgeReconnectCount int64

// Bridge health metrics, set by bridge package.
var (
	BridgeConnectedSince  string    // ISO 8601 timestamp of last bridge connection
	BridgeLastDisconnect  string    // ISO 8601 timestamp of last bridge disconnection
	BridgeLastCmdLatency  time.Duration // latency of the most recent command
	BridgeCmdCount        int64     // total commands sent since server start
	BridgeMinLatency      time.Duration // minimum observed command latency
	BridgeMaxLatency      time.Duration // maximum observed command latency
	BridgeMsgQueueDepth   int64     // pending messages in the outbound queue
	BridgeLastWsPing      time.Time // timestamp of last WebSocket ping
)

// BridgeHealthScore returns bridge health metrics with a computed health score.
func BridgeHealthScore() map[string]any {
	score := 0
	if BridgeConnected {
		score = 100
	}
	score -= int(BridgeReconnectCount) * 10
	if score < 0 {
		score = 0
	}
	status := "healthy"
	if score < 50 {
		status = "unhealthy"
	} else if score < 80 {
		status = "degraded"
	}
	return map[string]any{
		"connected":    BridgeConnected,
		"reconnects":   BridgeReconnectCount,
		"health_score": score,
		"status":       status,
	}
}

// BridgeHealthHandler returns bridge connection health with latency and uptime.
func BridgeHealthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"ok":               true,
			"connected":        BridgeConnected,
			"connected_since":  BridgeConnectedSince,
			"last_disconnect":  BridgeLastDisconnect,
			"last_cmd_latency": BridgeLastCmdLatency.String(),
			"min_cmd_latency":  BridgeMinLatency.String(),
			"max_cmd_latency":  BridgeMaxLatency.String(),
			"cmd_count":        BridgeCmdCount,
			"reconnect_count":  BridgeReconnectCount,
		})
	}
}

// BridgeHeartbeatHandler returns bridge heartbeat stats with timestamp.
func BridgeHeartbeatHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		healthScore := BridgeHealthScore()
		var pingMs float64
		if !BridgeLastWsPing.IsZero() {
			pingMs = float64(time.Since(BridgeLastWsPing).Milliseconds())
		}
		writeJSON(w, map[string]any{
			"ok":               true,
			"connected":        BridgeConnected,
			"connected_since":  BridgeConnectedSince,
			"last_disconnect":  BridgeLastDisconnect,
			"last_cmd_latency": BridgeLastCmdLatency.String(),
			"cmd_count":        BridgeCmdCount,
			"reconnect_count":  BridgeReconnectCount,
			"msg_queue_depth":  BridgeMsgQueueDepth,
			"last_ws_ping_ms":  pingMs,
			"health_score":     healthScore["health_score"],
			"health_status":    healthScore["status"],
			"timestamp":        time.Now().UTC().Format(time.RFC3339),
		})
	}
}

// BridgeMetricsHandler returns detailed bridge metrics.
func BridgeMetricsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var pingMs float64
		if !BridgeLastWsPing.IsZero() {
			pingMs = float64(time.Since(BridgeLastWsPing).Milliseconds())
		}
		uptimeSec := int64(0)
		if BridgeConnectedSince != "" {
			t, err := time.Parse(time.RFC3339, BridgeConnectedSince)
			if err == nil {
				uptimeSec = int64(time.Since(t).Seconds())
			}
		}
		writeJSON(w, map[string]any{
			"ok":              true,
			"connected":       BridgeConnected,
			"msg_count":       BridgeCmdCount,
			"reconnect_count": BridgeReconnectCount,
			"queue_depth":     BridgeMsgQueueDepth,
			"uptime_seconds":  uptimeSec,
			"last_ping_ms":    pingMs,
			"last_error":      BridgeLastDisconnect,
		})
	}
}

// GET /api/chat/address — returns contact link
func ChatAddressHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if SimplexCmd == nil {
			writeJSON(w, map[string]any{"ok": false, "error": "simplex not connected"})
			return
		}
		resp, err := SimplexCmd("/_show_address 1")
		if err != nil {
			link := readTrim(dataDir + "/island_contact_link.txt")
			if link != "" {
				writeJSON(w, map[string]any{"ok": true, "link": link})
				return
			}
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		rType, _ := resp["type"].(string)
		switch rType {
		case "userContactLink":
			if cl, ok := resp["contactLink"].(map[string]any); ok {
				if cr, _ := cl["connReq"].(string); cr != "" {
					writeJSON(w, map[string]any{"ok": true, "link": cr})
					return
				}
				if clc, ok := cl["connLinkContact"].(map[string]any); ok {
					if cfl, _ := clc["connFullLink"].(string); cfl != "" {
						writeJSON(w, map[string]any{"ok": true, "link": cfl})
						return
					}
				}
			}
			writeJSON(w, map[string]any{"ok": false, "error": "no link"})
		case "chatCmdError":
			writeJSON(w, map[string]any{"ok": false, "error": "no address"})
		default:
			writeJSON(w, map[string]any{"ok": false, "error": rType})
		}
	}
}

// POST /api/chat/address (create new)
func ChatAddressCreateHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		if SimplexCmd == nil {
			writeJSON(w, map[string]any{"ok": false, "error": "simplex not connected"})
			return
		}
		resp, err := SimplexCmd("/_address 1")
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		rType, _ := resp["type"].(string)
		if rType == "userContactLinkCreated" {
			if clc, ok := resp["connLinkContact"].(map[string]any); ok {
				if cfl, _ := clc["connFullLink"].(string); cfl != "" {
					link := cfl + "\n"
					writeFile(dataDir+"/island_contact_link.txt", []byte(link))
					writeJSON(w, map[string]any{"ok": true, "link": cfl})
					return
				}
			}
		}
		writeJSON(w, map[string]any{"ok": false, "error": rType})
	}
}

// GET /api/chat/contact?id=@N
func ChatContactInfoHandler(hub *ChatHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		chatID := r.URL.Query().Get("id")
		if chatID == "" {
			writeJSON(w, map[string]any{"ok": false, "error": "id required"})
			return
		}
		all := hub.GetMessages()
		count := 0
		var lastTime string
		for _, m := range all {
			if m.ChatID == chatID {
				count++
				if m.Timestamp > lastTime {
					lastTime = m.Timestamp
				}
			}
		}
		writeJSON(w, map[string]any{"ok": true, "count": count, "last_message": lastTime})
	}
}

// ChatContactHandler returns the full contact list from SimpleX CLI.
func ChatContactHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if SimplexCmd == nil {
			writeJSON(w, map[string]any{"ok": false, "error": "simplex not connected"})
			return
		}
		id := r.URL.Query().Get("id")
		if id == "" {
			writeJSON(w, map[string]any{"ok": false, "error": "id required"})
			return
		}
		resp, err := SimplexCmd("/_contacts 1")
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		rType, _ := resp["type"].(string)
		if rType != "contactsList" {
			writeJSON(w, map[string]any{"ok": false, "error": rType})
			return
		}
		contacts, _ := resp["contacts"].([]any)
		for _, c := range contacts {
			cm, ok := c.(map[string]any)
			if !ok {
				continue
			}
			cid, _ := cm["contactId"].(float64)
			if fmt.Sprintf("@%d", int64(cid)) == id || fmt.Sprintf("%d", int64(cid)) == id {
				writeJSON(w, map[string]any{"ok": true, "contact": cm})
				return
			}
		}
		writeJSON(w, map[string]any{"ok": false, "error": "not found"})
	}
}

// GET /api/chat/contacts
func ChatContactsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if SimplexCmd == nil {
			writeJSON(w, map[string]any{"ok": false, "contacts": []any{}})
			return
		}
		resp, err := SimplexCmd("/_contacts 1")
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "contacts": []any{}, "error": err.Error()})
			return
		}
		rType, _ := resp["type"].(string)
		if rType == "contactsList" {
			contacts, _ := resp["contacts"].([]any)
			writeJSON(w, map[string]any{"ok": true, "contacts": contacts})
		} else {
			writeJSON(w, map[string]any{"ok": false, "contacts": []any{}, "error": rType})
		}
	}
}

// POST /api/chat/connect — connect to a contact via their link
func ChatConnectHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		if SimplexCmd == nil {
			writeJSON(w, map[string]any{"ok": false, "error": "simplex not connected"})
			return
		}
		var req struct {
			Link string `json:"link"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
			return
		}
		if req.Link == "" {
			writeJSON(w, map[string]any{"ok": false, "error": "link required"})
			return
		}
		resp, err := SimplexCmd(fmt.Sprintf("/_connect 1 %s", req.Link))
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		rType, _ := resp["type"].(string)
		switch rType {
		case "sentConfirmation", "sentInvitation":
			auditInfo("contact_connected", fmt.Sprintf("link_type=%s", rType))
			writeJSON(w, map[string]any{"ok": true, "type": rType})
		case "contactAlreadyExists":
			writeJSON(w, map[string]any{"ok": true, "type": rType, "info": "contact already exists"})
		default:
			writeJSON(w, map[string]any{"ok": false, "error": rType})
		}
	}
}

// GET /api/chat/qr — generate QR code PNG from the contact link
func ChatQRHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		link := readTrim(dataDir + "/island_contact_link.txt")
		if link == "" {
			// Try to get from SimpleX
			if SimplexCmd != nil {
				resp, err := SimplexCmd("/_show_address 1")
				if err == nil {
					rType, _ := resp["type"].(string)
					if rType == "userContactLink" {
						if cl, ok := resp["contactLink"].(map[string]any); ok {
							if clc, ok := cl["connLinkContact"].(map[string]any); ok {
								if cfl, _ := clc["connFullLink"].(string); cfl != "" {
									link = cfl
								}
							}
						}
					}
				}
			}
		}
		if link == "" {
			http.Error(w, "no contact link available", 404)
			return
		}

		// Use qrencode CLI tool
		qrcmd := exec.Command("qrencode", "-s", "10", "-o", "-", link)
		png, err := qrcmd.Output()
		if err != nil {
			http.Error(w, "qr generation failed", 500)
			return
		}

		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(png)))
		w.Write(png)
	}
}

// DockerStatus holds the name, status, and health of a Docker container.
type DockerStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Health string `json:"health"`
}

// InquisitorReport consolidates bridge, contacts, messages, Docker, and disk status into a single health report.
type InquisitorReport struct {
	Bridge         bool           `json:"bridge"`
	Contacts       int            `json:"contacts"`
	Messages       int            `json:"messages"`
	BridgeSince    string         `json:"bridge_since,omitempty"`
	BridgeUp       bool           `json:"bridge_up"`
	SSEClients     int            `json:"sse_clients"`
	ReconnectCount int64          `json:"reconnect_count"`
	Docker         []DockerStatus `json:"docker,omitempty"`
	DiskUsed       string         `json:"disk_used_pct,omitempty"`
	DiskAvailGB    float64        `json:"disk_avail_gb,omitempty"`
	Healthy        bool           `json:"healthy"`
	UptimeHours    string         `json:"uptime_hours,omitempty"`
}

func collectDockerStatus() []DockerStatus {
	var out []DockerStatus
	for _, name := range []string{"simplex-node-smp-server", "simplex-node-xftp-server", "simplex-node-tor", "simplex-node-coturn", "simplex-node-v2ray"} {
		b, err := exec.Command("docker", "inspect", name, "--format", "{{.State.Status}}|{{.State.Health.Status}}").Output()
		status := "unknown"
		health := "unknown"
		if err == nil {
			parts := strings.SplitN(strings.TrimSpace(string(b)), "|", 2)
			status = parts[0]
			if len(parts) > 1 {
				health = parts[1]
			}
		}
		// Cycle 15: Test V2Ray upstream connectivity
		if name == "simplex-node-v2ray" {
			conn, dialErr := net.DialTimeout("tcp", "127.0.0.1:10810", 2*time.Second)
			if dialErr == nil {
				conn.Close()
				health = "upstream_ok"
			} else {
				health = "upstream_down"
			}
		}
		out = append(out, DockerStatus{Name: name, Status: status, Health: health})
	}
	wgStatus := "down"
	if b, err := exec.Command("ip", "link", "show", "wg0").Output(); err == nil && strings.Contains(string(b), "UP") {
		wgStatus = "up"
	}
	out = append(out, DockerStatus{Name: "wg0", Status: wgStatus, Health: ""})
	return out
}

func collectDiskInfo() (string, float64) {
	out, err := exec.Command("df", "-BG", "/").Output()
	if err != nil {
		return "unknown", 0
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return "unknown", 0
	}
	line := strings.Fields(lines[1])
	if len(line) >= 5 {
		return line[4], parseFloatGB(line[3])
	}
	return "unknown", 0
}

func parseFloatGB(s string) float64 {
	s = strings.TrimSuffix(s, "G")
	var v float64
	fmt.Sscanf(s, "%f", &v)
	return v
}

// InquisitorReportHandler returns a consolidated inquisitor report with bridge/contacts/SSE status.
func InquisitorReportHandler(hub *ChatHub, startTime time.Time) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		contactCount := 0
		if SimplexCmd != nil && BridgeConnected {
			resp, err := SimplexCmd("/_contacts 1")
			if err == nil {
				rType, _ := resp["type"].(string)
				if rType == "contactsList" {
					contacts, _ := resp["contacts"].([]any)
					contactCount = len(contacts)
				}
			}
		}
		msgCount := hub.MessageCount()
		sseCount := hub.SSEClientCount()
		docker := collectDockerStatus()
		diskPct, diskAvail := collectDiskInfo()
		uptimeHours := fmt.Sprintf("%.1f", time.Since(startTime).Hours())

		healthy := BridgeConnected
		for _, d := range docker {
			if strings.Contains(d.Status, "running") && (d.Health == "" || d.Health == "healthy") {
				continue
			}
			if !strings.Contains(d.Status, "running") {
				healthy = false
			}
		}

		writeJSON(w, InquisitorReport{
			Bridge:         BridgeConnected,
			Contacts:       contactCount,
			Messages:       msgCount,
			BridgeUp:       BridgeConnected,
			SSEClients:     sseCount,
			ReconnectCount: BridgeReconnectCount,
			Docker:         docker,
			DiskUsed:       diskPct,
			DiskAvailGB:    diskAvail,
			Healthy:        healthy,
			UptimeHours:    uptimeHours,
		})
	}
}

// ChatEditHandler edits a previously sent message.
func ChatEditHandler(hub *ChatHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		var req struct {
			ID   string `json:"id"`
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
			return
		}
		if req.ID == "" || req.Text == "" {
			writeJSON(w, map[string]any{"ok": false, "error": "id and text required"})
			return
		}
		if hub.EditMessageText(req.ID, req.Text) {
			writeJSON(w, map[string]any{"ok": true})
		} else {
			writeJSON(w, map[string]any{"ok": false, "error": "message not found"})
		}
	}
}

// ChatSearchHandler searches messages by text query with optional date range.
func ChatSearchHandler(hub *ChatHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		fromStr := r.URL.Query().Get("from")
		toStr := r.URL.Query().Get("to")
		if q == "" && fromStr == "" {
			writeJSON(w, map[string]any{"ok": false, "error": "q or from required"})
			return
		}
		var fromTime, toTime time.Time
		if fromStr != "" {
			if !validateDateParam(fromStr) {
				writeJSON(w, map[string]any{"ok": false, "error": "invalid 'from' date format (use RFC3339)"})
				return
			}
			fromTime, _ = time.Parse(time.RFC3339, fromStr)
		}
		if toStr != "" {
			if !validateDateParam(toStr) {
				writeJSON(w, map[string]any{"ok": false, "error": "invalid 'to' date format (use RFC3339)"})
				return
			}
			toTime, _ = time.Parse(time.RFC3339, toStr)
		} else {
			toTime = time.Now().UTC()
		}
		out := make([]ChatMessage, 0)
		if q != "" && fromStr == "" && GlobalInvertedIndex != nil {
			results := GlobalInvertedIndex.Search(q, 100)
			for _, m := range results {
				t, err := time.Parse(time.RFC3339, m.Timestamp)
				if err != nil {
					continue
				}
				if !toTime.IsZero() && t.After(toTime) {
					continue
				}
				out = append(out, m)
			}
		} else {
			all := hub.GetMessages()
			for _, m := range all {
				if q != "" && !contains(m.Text, q) && !contains(m.ID, q) && !contains(m.From, q) {
					continue
				}
				t, err := time.Parse(time.RFC3339, m.Timestamp)
				if err != nil {
					continue
				}
				if !fromTime.IsZero() && t.Before(fromTime) {
					continue
				}
				if !toTime.IsZero() && t.After(toTime) {
					continue
				}
				out = append(out, m)
			}
		}
		writeJSON(w, map[string]any{"ok": true, "messages": out})
	}
}

// ChatPinHandler toggles message pin status.
func ChatPinHandler(hub *ChatHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			writeJSON(w, map[string]any{"ok": true, "pinned": hub.GetPinned()})
			return
		}
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		var req struct{ ID string `json:"id"` }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
			return
		}
		if hub.TogglePin(req.ID) {
			writeJSON(w, map[string]any{"ok": true})
		} else {
			writeJSON(w, map[string]any{"ok": false, "error": "not found"})
		}
	}
}

// ChatReactHandler toggles an emoji reaction on a message.
func ChatReactHandler(hub *ChatHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		var req struct {
			ID    string `json:"id"`
			Emoji string `json:"emoji"`
			User  string `json:"user"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
			return
		}
		if req.ID == "" || req.Emoji == "" || req.User == "" {
			writeJSON(w, map[string]any{"ok": false, "error": "id, emoji, user required"})
			return
		}
		result := hub.ToggleReaction(req.ID, req.Emoji, req.User)
		writeJSON(w, map[string]any{"ok": result != "not found", "action": result})
	}
}

var (
	serverStatusMu   sync.RWMutex
	serverStatus     string
	serverStatusFile string
)

func loadServerStatus() {
	if serverStatusFile == "" {
		return
	}
	b, err := os.ReadFile(serverStatusFile)
	if err != nil {
		return
	}
	var data struct {
		Status string `json:"status"`
	}
	if json.Unmarshal(b, &data) == nil {
		serverStatusMu.Lock()
		serverStatus = data.Status
		serverStatusMu.Unlock()
	}
}

func saveServerStatus() {
	if serverStatusFile == "" {
		return
	}
	data := map[string]string{"status": serverStatus}
	b, _ := json.Marshal(data)
	os.WriteFile(serverStatusFile, b, 0600)
}

// SetServerStatusHandler gets/sets the server status message broadcast to clients.
func SetServerStatusHandler(dataDir string) http.HandlerFunc {
	serverStatusFile = filepath.Join(dataDir, "server_status.json")
	loadServerStatus()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			serverStatusMu.RLock()
			s := serverStatus
			serverStatusMu.RUnlock()
			writeJSON(w, map[string]any{"ok": true, "status": s})
			return
		}
		if r.Method != "POST" {
			http.Error(w, "GET or POST", 405)
			return
		}
		var req struct {
			Status string `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
			return
		}
		serverStatusMu.Lock()
		serverStatus = req.Status
		serverStatusMu.Unlock()
		saveServerStatus()
		writeJSON(w, map[string]any{"ok": true, "status": serverStatus})
	}
}

// ChatClearOldHandler deletes messages older than N days.
func ChatClearOldHandler(hub *ChatHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		var req struct {
			Before string `json:"before"` // RFC3339 date
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
			return
		}
		cutoff, err := time.Parse(time.RFC3339, req.Before)
		if err != nil {
			cutoff = time.Now().UTC().Add(-7 * 24 * time.Hour) // default: 7 days
		}
		hub.mu.Lock()
		filtered := make([]ChatMessage, 0, len(hub.messages))
		for _, m := range hub.messages {
			t, err := time.Parse(time.RFC3339, m.Timestamp)
			if err != nil || t.After(cutoff) {
				filtered = append(filtered, m)
			}
		}
		removed := len(hub.messages) - len(filtered)
		hub.messages = filtered
		hub.mu.Unlock()
		hub.flush()
		writeJSON(w, map[string]any{"ok": true, "removed": removed, "remaining": len(filtered)})
	}
}

// ChatStatsHandler returns message statistics (total, today, per chat).
func ChatStatsHandler(hub *ChatHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		all := hub.GetMessages()
		total := len(all)
		byChat := map[string]int{}
		today := 0
		now := time.Now().UTC()
		for _, m := range all {
			byChat[m.ChatID]++
			t, err := time.Parse(time.RFC3339, m.Timestamp)
			if err == nil && t.Year() == now.Year() && t.YearDay() == now.YearDay() {
				today++
			}
		}
		writeJSON(w, map[string]any{
			"ok":       true,
			"total":    total,
			"today":    today,
			"per_chat": byChat,
		})
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && len(sub) > 0 && searchIndex(s, sub) >= 0
}

func searchIndex(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		match := true
		for j := 0; j < len(sub); j++ {
			si, sj := s[i+j], sub[j]
			if si >= 'A' && si <= 'Z' {
				si += 32
			}
			if sj >= 'A' && sj <= 'Z' {
				sj += 32
			}
			if si != sj {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

// ChatAliasHandler sets a display alias for a contact.
func ChatAliasHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		if SimplexCmd == nil {
			writeJSON(w, map[string]any{"ok": false, "error": "simplex not connected"})
			return
		}
		var req struct {
			ContactID int64  `json:"contact_id"`
			Alias     string `json:"alias"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
			return
		}
		if req.ContactID <= 0 || req.Alias == "" {
			writeJSON(w, map[string]any{"ok": false, "error": "contact_id and alias required"})
			return
		}
		auditInfo("contact_alias", fmt.Sprintf("contact_id=%d, alias=%s", req.ContactID, req.Alias))
		resp, err := SimplexCmd(fmt.Sprintf("/_set_alias %d %s", req.ContactID, req.Alias))
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "response": resp})
	}
}

func writeFile(path string, data []byte) {
	os.WriteFile(path, data, 0600)
}

// ChatBackupHandler downloads or uploads full chat backup.
func ChatBackupHandler(hub *ChatHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			msgs := hub.GetMessages()
			b, _ := json.MarshalIndent(msgs, "", "  ")
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Content-Disposition", "attachment; filename=chat_backup.json")
			w.Write(b)
			return
		}
		if r.Method == "POST" {
			var req struct {
				Messages []ChatMessage `json:"messages"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
				return
			}
			hub.mu.Lock()
			hub.messages = req.Messages
			if hub.messages == nil {
				hub.messages = make([]ChatMessage, 0, MaxChatHistory)
			}
			hub.mu.Unlock()
			hub.flush()
			writeJSON(w, map[string]any{"ok": true, "count": len(req.Messages)})
			return
		}
		http.Error(w, "GET or POST only", 405)
	}
}

// ChatExportHandler exports chat history in JSON or HTML format.
func ChatExportHandler(hub *ChatHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		format := r.URL.Query().Get("format")
		msgs := hub.GetMessages()
		if format == "html" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Content-Disposition", "attachment; filename=chat_export.html")
			w.Write([]byte("<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n<title>Chat Export</title>\n<style>\n*{box-sizing:border-box;margin:0;padding:0}\nbody{background:#1a1a2e;color:#e0e0e0;font-family:system-ui,-apple-system,sans-serif;max-width:800px;margin:40px auto;padding:0 20px}\nh1{color:#b8860b;text-align:center;margin-bottom:30px;font-size:1.5rem}\n.msg{background:#16213e;border-radius:8px;padding:12px 16px;margin:12px 0;border-left:3px solid #0f3460}\n.from{color:#93c5fd;font-weight:bold;font-size:0.85em;margin-bottom:4px}\n.text{color:#e0e0e0;margin:6px 0;line-height:1.5}\n.time{color:#64748b;font-size:0.75em}\n.user{background:#1a2e1a;border-left-color:#22c55e}\n.user .from{color:#86efac}\nfooter{text-align:center;margin-top:40px;color:#475569;font-size:0.75rem}\n</style>\n</head>\n<body>\n<h1>Chat Export</h1>\n"))
			for _, m := range msgs {
				cls := "msg"
				if m.IsUser {
					cls += " user"
				}
				fmt.Fprintf(w, "<div class=\"%s\"><div class=\"from\">%s</div><div class=\"text\">%s</div><div class=\"time\">%s</div></div>\n", cls, htmlEsc(m.From), htmlEsc(m.Text), htmlEsc(m.Timestamp))
			}
			w.Write([]byte("<footer>simplex-node chat export &mdash; " + time.Now().UTC().Format("2006-01-02 15:04 UTC") + "</footer>\n</body>\n</html>"))
			return
		}
		b, _ := json.MarshalIndent(msgs, "", "  ")
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", "attachment; filename=chat_export.json")
		w.Write(b)
	}
}

func htmlEsc(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

// ChatArchiveHandler cold-archives old messages to USB storage.
func ChatArchiveHandler(hub *ChatHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		daysStr := r.URL.Query().Get("older_than")
		days := validateIntParam(daysStr, 30)
		if days < 1 {
			days = 30
		}
		cutoff := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
		hub.mu.Lock()
		archived := make([]ChatMessage, 0)
		kept := make([]ChatMessage, 0, len(hub.messages))
		for _, m := range hub.messages {
			t, err := time.Parse(time.RFC3339, m.Timestamp)
			if err == nil && t.Before(cutoff) {
				archived = append(archived, m)
			} else {
				kept = append(kept, m)
			}
		}
		hub.messages = kept
		hub.mu.Unlock()
		hub.flush()
		if len(archived) > 0 {
			archiveDir := filepath.Join(filepath.Dir(hub.filePath), "archives")
			os.MkdirAll(archiveDir, 0700)
			archivePath := filepath.Join(archiveDir, fmt.Sprintf("archive-%s.json", time.Now().Format("20060102-150405")))
			b, _ := json.MarshalIndent(archived, "", "  ")
			os.WriteFile(archivePath, b, 0600)
			// Also copy to USB if mounted
			if b, err := os.ReadFile("/mnt/simplex-backup/auto.txt"); err == nil && string(b) != "" {
				usbDir := "/mnt/simplex-backup/chat-archives"
				os.MkdirAll(usbDir, 0755)
				os.WriteFile(filepath.Join(usbDir, fmt.Sprintf("archive-%s.json", time.Now().Format("20060102-150405"))), b, 0644)
			}
		}
		writeJSON(w, map[string]any{"ok": true, "archived": len(archived), "remaining": len(kept)})
	}
}

// ChatArchiveListHandler returns a list of available chat archive dates with stats.
func ChatArchiveListHandler(hub *ChatHub, dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		archives, err := hub.ListArchives(dataDir)
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		stats := hub.ArchiveStats(dataDir)
		writeJSON(w, map[string]any{"ok": true, "archives": archives, "stats": stats})
	}
}

// ChatArchiveRestoreHandler restores messages from a dated archive back into the chat hub.
func ChatArchiveRestoreHandler(hub *ChatHub, dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		var req struct {
			Date string `json:"date"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
			return
		}
		if req.Date == "" {
			writeJSON(w, map[string]any{"ok": false, "error": "date required (YYYYMMDD)"})
			return
		}
		if len(req.Date) != 8 {
			writeJSON(w, map[string]any{"ok": false, "error": "invalid date format, expected YYYYMMDD"})
			return
		}
		if _, err := time.Parse("20060102", req.Date); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "invalid date: " + err.Error()})
			return
		}
		archives, err := hub.ListArchives(dataDir)
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "failed to list archives: " + err.Error()})
			return
		}
		found := false
		for _, a := range archives {
			if a == req.Date {
				found = true
				break
			}
		}
		if !found {
			writeJSON(w, map[string]any{"ok": false, "error": "archive not found: " + req.Date + " (available: " + strings.Join(archives, ", ") + ")"})
			return
		}
		restored, err := hub.RestoreArchive(dataDir, req.Date)
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		logAudit("chat_archive_restore", "admin", "restored archive from "+req.Date)
		writeJSON(w, map[string]any{"ok": true, "restored": restored})
	}
}

// ChatBroadcastHandler sends a message to all contacts.
func ChatBroadcastHandler(hub *ChatHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		if !requireContentType(r) {
			writeJSON(w, map[string]any{"ok": false, "error": "Content-Type must be application/json"})
			return
		}
		if SimplexCmd == nil || !BridgeConnected {
			writeJSON(w, map[string]any{"ok": false, "error": "bridge not connected"})
			return
		}
		var req struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
			return
		}
		if req.Text == "" {
			writeJSON(w, map[string]any{"ok": false, "error": "text required"})
			return
		}
		resp, err := SimplexCmd(fmt.Sprintf("/_contacts 1"))
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		rType, _ := resp["type"].(string)
		if rType != "contactsList" {
			writeJSON(w, map[string]any{"ok": false, "error": "unexpected response"})
			return
		}
		contacts, _ := resp["contacts"].([]any)
		sent := 0
		failed := 0
		for _, c := range contacts {
			cm, ok := c.(map[string]any)
			if !ok {
				continue
			}
			id, _ := cm["contactId"].(float64)
			_, err := SimplexCmd(fmt.Sprintf("/_send %d %s", int64(id), req.Text))
			if err != nil {
				failed++
			} else {
				sent++
			}
			time.Sleep(100 * time.Millisecond)
		}
		hub.AddMessage(ChatMessage{
			ID:        fmt.Sprintf("broadcast-%d", time.Now().UnixNano()),
			Text:      req.Text,
			From:      "admin",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			IsUser:    true,
			ChatID:    "broadcast",
		})
		writeJSON(w, map[string]any{"ok": true, "sent": sent, "failed": failed, "total": len(contacts)})
	}
}

// ChatLastMessageHandler returns the most recent message from each chat.
func ChatLastMessageHandler(hub *ChatHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		chatID := r.URL.Query().Get("chat_id")
		msgs := hub.GetMessages()
		var last *ChatMessage
		for i := len(msgs) - 1; i >= 0; i-- {
			if chatID == "" || msgs[i].ChatID == chatID {
				last = &msgs[i]
				break
			}
		}
		if last == nil {
			writeJSON(w, map[string]any{"ok": true, "last_message": nil})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "last_message": last})
	}
}

// ── Typing Indicator ──────────────────────────────────────────────────────────

var (
	typingMu          sync.RWMutex
	typingState       = map[string]time.Time{} // chatID → last typing timestamp
	typingCleanupOnce sync.Once
)

// ChatTypingHandler sets or clears typing indicator for a chat.
func ChatTypingHandler() http.HandlerFunc {
	// Cycle 16: Start typing indicator cleanup goroutine once
	typingCleanupOnce.Do(func() {
		go func() {
			for {
				time.Sleep(5 * time.Second)
				typingMu.Lock()
				now := time.Now()
				for cid, ts := range typingState {
					if now.Sub(ts) > 10*time.Second {
						delete(typingState, cid)
					}
				}
				typingMu.Unlock()
			}
		}()
	})
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			var req struct {
				ChatID string `json:"chat_id"`
				Active bool   `json:"active"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
				return
			}
			typingMu.Lock()
			if req.Active {
				typingState[req.ChatID] = time.Now()
			} else {
				delete(typingState, req.ChatID)
			}
			typingMu.Unlock()
			writeJSON(w, map[string]any{"ok": true})
			return
		}
		chatID := r.URL.Query().Get("chat_id")
		typingMu.RLock()
		ts, ok := typingState[chatID]
		typingMu.RUnlock()
		if ok && time.Since(ts) < 10*time.Second {
			writeJSON(w, map[string]any{"ok": true, "typing": true, "since": ts.Format(time.RFC3339)})
		} else {
			writeJSON(w, map[string]any{"ok": true, "typing": false})
		}
	}
}

// ── Scheduled Messages ────────────────────────────────────────────────────────

// ScheduledMessage represents a message scheduled for future delivery.
type ScheduledMessage struct {
	ID        string `json:"id"`
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	ContactID int64  `json:"contact_id"`
	SendAt    string `json:"send_at"`
	CreatedAt string `json:"created_at"`
	Cancelled bool   `json:"cancelled,omitempty"`
}

// ScheduleManager manages scheduled message delivery with a background loop.
type ScheduleManager struct {
	mu  sync.RWMutex
	msgs []ScheduledMessage
	hub  *ChatHub
}

var GlobalSchedule *ScheduleManager

// NewScheduleManager creates a ScheduleManager, loads persisted scheduled messages, and starts the delivery loop.
func NewScheduleManager(hub *ChatHub, dataDir string) *ScheduleManager {
	sm := &ScheduleManager{hub: hub}
	sm.load(dataDir)
	go sm.deliveryLoop(dataDir)
	return sm
}

func (sm *ScheduleManager) save(dataDir string) {
	data, _ := json.Marshal(sm.msgs)
	os.WriteFile(filepath.Join(dataDir, "scheduled_messages.json"), data, 0644)
}

func (sm *ScheduleManager) load(dataDir string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	data, err := os.ReadFile(filepath.Join(dataDir, "scheduled_messages.json"))
	if err != nil {
		return
	}
	json.Unmarshal(data, &sm.msgs)
}

func (sm *ScheduleManager) deliveryLoop(dataDir string) {
	for {
		time.Sleep(30 * time.Second)
		sm.mu.Lock()
		now := time.Now()
		remaining := make([]ScheduledMessage, 0, len(sm.msgs))
		for _, m := range sm.msgs {
			if m.Cancelled {
				continue
			}
			t, err := time.Parse(time.RFC3339, m.SendAt)
			if err != nil || t.After(now) {
				remaining = append(remaining, m)
				continue
			}
			if SimplexCmd != nil && BridgeConnected {
				SimplexCmd(fmt.Sprintf("/_send %d %s", m.ContactID, m.Text))
			}
			sm.hub.AddMessage(ChatMessage{
				ID:        m.ID,
				Text:      m.Text,
				From:      "admin",
				Timestamp: now.Format(time.RFC3339),
				IsUser:    true,
				ChatID:    m.ChatID,
			})
		}
		sm.msgs = remaining
		if dataDir != "" {
			sm.save(dataDir)
		}
		sm.mu.Unlock()
	}
}

// ChatScheduleHandler schedules a message for later delivery.
func ChatScheduleHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			GlobalSchedule.mu.RLock()
			out := make([]ScheduledMessage, 0, len(GlobalSchedule.msgs))
			for _, m := range GlobalSchedule.msgs {
				if !m.Cancelled {
					out = append(out, m)
				}
			}
			GlobalSchedule.mu.RUnlock()
			writeJSON(w, map[string]any{"ok": true, "scheduled": out})
			return
		}
		if r.Method == "POST" {
			var req struct {
				ChatID    string `json:"chat_id"`
				Text      string `json:"text"`
				ContactID int64  `json:"contact_id"`
				SendAt    string `json:"send_at"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
				return
			}
			if req.Text == "" || req.SendAt == "" {
				writeJSON(w, map[string]any{"ok": false, "error": "text and send_at required"})
				return
			}
			sm := ScheduledMessage{
				ID:        fmt.Sprintf("sched-%d", time.Now().UnixNano()),
				ChatID:    req.ChatID,
				Text:      req.Text,
				ContactID: req.ContactID,
				SendAt:    req.SendAt,
				CreatedAt: time.Now().UTC().Format(time.RFC3339),
			}
			GlobalSchedule.mu.Lock()
			GlobalSchedule.msgs = append(GlobalSchedule.msgs, sm)
			GlobalSchedule.mu.Unlock()
			writeJSON(w, map[string]any{"ok": true, "scheduled": sm})
			return
		}
		if r.Method == "DELETE" {
			id := r.URL.Query().Get("id")
			GlobalSchedule.mu.Lock()
			for i := range GlobalSchedule.msgs {
				if GlobalSchedule.msgs[i].ID == id {
					GlobalSchedule.msgs[i].Cancelled = true
					break
				}
			}
			GlobalSchedule.mu.Unlock()
			writeJSON(w, map[string]any{"ok": true})
			return
		}
		http.Error(w, "GET/POST/DELETE", 405)
	}
}

// ── Auto-Reply Rules ──────────────────────────────────────────────────────────

// AutoReplyRule defines a keyword-pattern-based auto-reply rule for incoming messages.
type AutoReplyRule struct {
	ID      string `json:"id"`
	Pattern string `json:"pattern"` // exact match or prefix
	Reply   string `json:"reply"`
	Enabled bool   `json:"enabled"`
}

var (
	autoReplyMu    sync.RWMutex
	autoReplyRules []AutoReplyRule
)

// ChatAutoReplyHandler manages auto-reply rules (keyword/response pairs).
func ChatAutoReplyHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			autoReplyMu.RLock()
			out := append([]AutoReplyRule{}, autoReplyRules...)
			autoReplyMu.RUnlock()
			writeJSON(w, map[string]any{"ok": true, "rules": out})
		case "POST":
			var rule AutoReplyRule
			if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
				writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
				return
			}
			rule.ID = fmt.Sprintf("ar-%d", time.Now().UnixNano())
			autoReplyMu.Lock()
			autoReplyRules = append(autoReplyRules, rule)
			autoReplyMu.Unlock()
			writeJSON(w, map[string]any{"ok": true, "rule": rule})
		case "PUT":
			id := r.URL.Query().Get("id")
			var rule AutoReplyRule
			if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
				writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
				return
			}
			autoReplyMu.Lock()
			for i, ru := range autoReplyRules {
				if ru.ID == id {
					rule.ID = id
					autoReplyRules[i] = rule
					break
				}
			}
			autoReplyMu.Unlock()
			writeJSON(w, map[string]any{"ok": true})
		case "DELETE":
			id := r.URL.Query().Get("id")
			autoReplyMu.Lock()
			remaining := make([]AutoReplyRule, 0, len(autoReplyRules))
			for _, ru := range autoReplyRules {
				if ru.ID != id {
					remaining = append(remaining, ru)
				}
			}
			autoReplyRules = remaining
			autoReplyMu.Unlock()
			writeJSON(w, map[string]any{"ok": true})
		default:
			http.Error(w, "GET/POST/DELETE", 405)
		}
	}
}

func parseChatID(chatID string) int64 {
	s := strings.TrimPrefix(chatID, "@")
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// matchAutoReply checks enabled auto-reply rules against text and returns the
// first matching reply. Supports regex patterns. Fallback: exact/prefix match.
func matchAutoReply(text string) (string, bool) {
	autoReplyMu.RLock()
	defer autoReplyMu.RUnlock()
	for _, rule := range autoReplyRules {
		if !rule.Enabled {
			continue
		}
		if p, err := regexp.Compile(rule.Pattern); err == nil {
			if p.MatchString(text) {
				return rule.Reply, true
			}
		} else if strings.HasPrefix(text, rule.Pattern) || text == rule.Pattern {
			return rule.Reply, true
		}
	}
	return "", false
}

// ── Contact Groups ────────────────────────────────────────────────────────────

// ContactGroup represents a named group of contact IDs for batch operations.
type ContactGroup struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Members []string `json:"members"` // contact IDs
}

var (
	groupsMu  sync.RWMutex
	groups    []ContactGroup
	groupsSeq int64
)

// ChatGroupsHandler manages contact groups (create, list, add, remove).
func ChatGroupsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			groupsMu.RLock()
			out := append([]ContactGroup{}, groups...)
			groupsMu.RUnlock()
			writeJSON(w, map[string]any{"ok": true, "groups": out})
		case "POST":
			var req struct {
				Name    string   `json:"name"`
				Members []string `json:"members"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
				return
			}
			groupsSeq++
			g := ContactGroup{
				ID:      fmt.Sprintf("g-%d", groupsSeq),
				Name:    req.Name,
				Members: req.Members,
			}
			groupsMu.Lock()
			groups = append(groups, g)
			groupsMu.Unlock()
			writeJSON(w, map[string]any{"ok": true, "group": g})
		case "DELETE":
			id := r.URL.Query().Get("id")
			groupsMu.Lock()
			remaining := make([]ContactGroup, 0, len(groups))
			for _, g := range groups {
				if g.ID != id {
					remaining = append(remaining, g)
				}
			}
			groups = remaining
			groupsMu.Unlock()
			writeJSON(w, map[string]any{"ok": true})
		case "PATCH":
			id := r.URL.Query().Get("id")
			var req struct {
				Name    string   `json:"name"`
				Members []string `json:"members"`
			}
			json.NewDecoder(r.Body).Decode(&req)
			groupsMu.Lock()
			for i := range groups {
				if groups[i].ID == id {
					if req.Name != "" {
						groups[i].Name = req.Name
					}
					if req.Members != nil {
						groups[i].Members = req.Members
					}
					break
				}
			}
			groupsMu.Unlock()
			writeJSON(w, map[string]any{"ok": true})
		default:
			http.Error(w, "GET/POST/DELETE/PATCH", 405)
		}
	}
}

// ── Labels / Tags ─────────────────────────────────────────────────────────────

var (
	labelsMu   sync.RWMutex
	msgLabels  = map[string][]string{} // messageID → label list
	chatLabels = map[string][]string{} // chatID → label list
)

// ChatLabelsHandler manages message labels.
func ChatLabelsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "POST":
			var req struct {
				MessageID string   `json:"message_id"`
				ChatID    string   `json:"chat_id"`
				Labels    []string `json:"labels"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
				return
			}
			labelsMu.Lock()
			if req.MessageID != "" {
				msgLabels[req.MessageID] = req.Labels
			}
			if req.ChatID != "" {
				chatLabels[req.ChatID] = req.Labels
			}
			labelsMu.Unlock()
			writeJSON(w, map[string]any{"ok": true})
		case "GET":
			msgID := r.URL.Query().Get("message_id")
			chatID := r.URL.Query().Get("chat_id")
			labelsMu.RLock()
			var ml, cl []string
			if msgID != "" {
				ml = msgLabels[msgID]
			}
			if chatID != "" {
				cl = chatLabels[chatID]
			}
			labelsMu.RUnlock()
			writeJSON(w, map[string]any{"ok": true, "message_labels": ml, "chat_labels": cl})
		default:
			http.Error(w, "POST/GET", 405)
		}
	}
}

// ── Message Drafts ────────────────────────────────────────────────────────────

var (
	draftsMu  sync.RWMutex
	drafts    = map[string]string{} // chatID → draft text
)

// ChatDraftsHandler manages draft messages per chat.
func ChatDraftsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		chatID := r.URL.Query().Get("chat_id")
		switch r.Method {
		case "GET":
			draftsMu.RLock()
			text := drafts[chatID]
			draftsMu.RUnlock()
			writeJSON(w, map[string]any{"ok": true, "draft": text})
		case "POST":
			var req struct {
				ChatID string `json:"chat_id"`
				Text   string `json:"text"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
				return
			}
			draftsMu.Lock()
			if req.Text == "" {
				delete(drafts, req.ChatID)
			} else {
				drafts[req.ChatID] = req.Text
			}
			draftsMu.Unlock()
			writeJSON(w, map[string]any{"ok": true})
		case "DELETE":
			draftsMu.Lock()
			delete(drafts, chatID)
			draftsMu.Unlock()
			writeJSON(w, map[string]any{"ok": true})
		default:
			http.Error(w, "GET/POST/DELETE", 405)
		}
	}
}

// ── Webhook Config ────────────────────────────────────────────────────────────

// WebhookConfig defines the configuration for outbound webhook notifications.
type WebhookConfig struct {
	URL        string   `json:"url"`
	Events     []string `json:"events"` // message,send,contact
	Secret     string   `json:"secret,omitempty"`
	Enabled    bool     `json:"enabled"`
	Retries    int      `json:"retries"`    // max retry count
	RetryDelay int      `json:"retry_delay"` // seconds between retries
}

var (
	webhookMu   sync.RWMutex
	webhookCfg  WebhookConfig
)

// ChatWebhookHandler manages webhook configuration for incoming messages.
func ChatWebhookHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			webhookMu.RLock()
			cfg := webhookCfg
			webhookMu.RUnlock()
			writeJSON(w, map[string]any{"ok": true, "config": cfg})
		case "POST":
			var cfg WebhookConfig
			if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
				writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
				return
			}
			webhookMu.Lock()
			webhookCfg = cfg
			webhookMu.Unlock()
			writeJSON(w, map[string]any{"ok": true, "config": cfg})
		case "DELETE":
			webhookMu.Lock()
			webhookCfg = WebhookConfig{}
			webhookMu.Unlock()
			writeJSON(w, map[string]any{"ok": true})
		default:
			http.Error(w, "GET/POST/DELETE", 405)
		}
	}
}

// ── Advanced Search ───────────────────────────────────────────────────────────

// ChatAdvancedSearchHandler performs advanced search with multiple filters.
func ChatAdvancedSearchHandler(hub *ChatHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		chatID := r.URL.Query().Get("chat_id")
		label := r.URL.Query().Get("label")
		sender := r.URL.Query().Get("from")
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
		if limit < 1 {
			limit = 50
		}
		if offset < 0 {
			offset = 0
		}

		all := hub.GetMessages()
		labelsMu.RLock()
		defer labelsMu.RUnlock()

		out := make([]ChatMessage, 0, len(all))
		for _, m := range all {
			if q != "" && !contains(m.Text, q) && !contains(m.ID, q) {
				continue
			}
			if chatID != "" && m.ChatID != chatID {
				continue
			}
			if sender != "" && m.From != sender {
				continue
			}
			if label != "" {
				ml := msgLabels[m.ID]
				if !inList(ml, label) {
					continue
				}
			}
			out = append(out, m)
		}
		total := len(out)
		if offset >= total {
			out = []ChatMessage{}
		} else {
			end := offset + limit
			if end > total {
				end = total
			}
			out = out[offset:end]
		}
		writeJSON(w, map[string]any{"ok": true, "messages": out, "total": total, "limit": limit, "offset": offset})
	}
}

func inList(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// ChatPayHandler sends XAG to a contact via chat.
func ChatPayHandler(hub *ChatHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", 405)
			return
		}
		var req struct {
			ContactID int64   `json:"contact_id"`
			ChatID    string  `json:"chat_id"`
			Amount    float64 `json:"amount"`
			Asset     string  `json:"asset"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad json", 400)
			return
		}
		if req.Amount <= 0 {
			http.Error(w, "amount must be positive", 400)
			return
		}
		asset := req.Asset
		if asset == "" {
			asset = "XAG"
		}
		msg := ChatMessage{
			ID:          fmt.Sprintf("pay-%d", time.Now().UnixNano()),
			From:        "admin",
			Text:        fmt.Sprintf("💸 Sent %.2f %s", req.Amount, asset),
			Timestamp:   time.Now().UTC().Format(time.RFC3339),
			IsUser:      true,
			ChatID:      req.ChatID,
			Status:      StatusSent,
			MoneyAmount: &req.Amount,
			MoneyAsset:  asset,
			MoneyTxID:   fmt.Sprintf("tx-%d", time.Now().UnixNano()),
		}
		hub.AddMessage(msg)
		if BridgeSendFunc != nil {
			BridgeSendFunc(msg.Text, 1, req.ContactID)
		}
		writeJSON(w, map[string]any{"ok": true, "message": msg})
	}
}

// ChatRecallHandler recalls (unsends) a message within 5 minutes.
func ChatRecallHandler(hub *ChatHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", 405)
			return
		}
		var req struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad json", 400)
			return
		}
		hub.mu.Lock()
		found := false
		for i, m := range hub.messages {
			if m.ID == req.ID {
				ts, err := time.Parse(time.RFC3339, m.Timestamp)
				if err != nil || time.Since(ts) > 5*time.Minute {
					hub.mu.Unlock()
					writeJSON(w, map[string]any{"ok": false, "error": "can only recall within 5 minutes"})
					return
				}
				hub.messages[i].Recalled = true
				hub.messages[i].RecalledAt = time.Now().UTC().Format(time.RFC3339)
				hub.messages[i].Text = "[message recalled]"
				found = true
				break
			}
		}
		hub.mu.Unlock()
		if !found {
			writeJSON(w, map[string]any{"ok": false, "error": "message not found"})
			return
		}
		hub.flush()
		writeJSON(w, map[string]any{"ok": true, "status": "recalled"})
	}
}

// ChatReadReceiptHandler marks messages as read.
func ChatReadReceiptHandler(hub *ChatHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", 405)
			return
		}
		var req struct {
			IDs []string `json:"ids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad json", 400)
			return
		}
		hub.mu.Lock()
		readAt := time.Now().UTC().Format(time.RFC3339)
		for _, id := range req.IDs {
			for i, m := range hub.messages {
				if m.ID == id && !m.IsUser {
					hub.messages[i].ReadAt = readAt
					hub.messages[i].Status = StatusDelivered
				}
			}
		}
		hub.mu.Unlock()
		hub.flush()
		writeJSON(w, map[string]any{"ok": true, "count": len(req.IDs)})
	}
}

// ChatVoiceHandler records voice message metadata.
func ChatVoiceHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", 405)
			return
		}
		var req struct {
			ChatID    string `json:"chat_id"`
			ContactID int64  `json:"contact_id"`
			VoiceURL  string `json:"voice_url"`
			Duration  int    `json:"duration"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad json", 400)
			return
		}
		msg := ChatMessage{
			ID:        fmt.Sprintf("voice-%d", time.Now().UnixNano()),
			From:      "admin",
			Text:      "🎤 Voice message",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			IsUser:    true,
			ChatID:    req.ChatID,
			Status:    StatusSent,
			VoiceURL:  req.VoiceURL,
			VoiceDur:  req.Duration,
		}
		GlobalChatHub.AddMessage(msg)
		if BridgeSendFunc != nil && req.ContactID > 0 {
			BridgeSendFunc(msg.Text, 1, req.ContactID)
		}
		writeJSON(w, map[string]any{"ok": true, "message": msg})
	}
}

// ChatSendFileHandler accepts file uploads via multipart form.
func ChatSendFileHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", 405)
			return
		}
		const maxSize = 20 << 20
		r.Body = http.MaxBytesReader(w, r.Body, maxSize)
		if err := r.ParseMultipartForm(maxSize); err != nil {
			http.Error(w, "file too large (max 20MB)", 400)
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "missing file field", 400)
			return
		}
		defer file.Close()

		dir := filepath.Join(dataDir, "files")
		if err := os.MkdirAll(dir, 0755); err != nil {
			slog.Error("mkdir files", "err", err)
			http.Error(w, "server error", 500)
			return
		}

		ext := filepath.Ext(header.Filename)
		savedName := fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)
		dst, err := os.Create(filepath.Join(dir, savedName))
		if err != nil {
			slog.Error("create file", "err", err)
			http.Error(w, "server error", 500)
			return
		}
		defer dst.Close()

		written, err := io.Copy(dst, file)
		if err != nil {
			slog.Error("save file", "err", err)
			http.Error(w, "server error", 500)
			return
		}

		fileURL := "/files/" + savedName
		chatID := r.FormValue("chat_id")

		msg := ChatMessage{
			ID:        fmt.Sprintf("file-%d", time.Now().UnixNano()),
			From:      "admin",
			Text:      "📎 " + header.Filename,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			IsUser:    true,
			ChatID:    chatID,
			Status:    StatusSent,
			FileName:  header.Filename,
			FileURL:   fileURL,
			FileSize:  written,
		}
		GlobalChatHub.AddMessage(msg)

		writeJSON(w, map[string]any{
			"ok":       true,
			"file_url": fileURL,
			"filename": header.Filename,
			"size":     written,
			"message":  msg,
		})
	}
}

// ChatThemeHandler manages per-contact theme colors.
var (
	chatThemesMu sync.RWMutex
	chatThemes   = map[string]string{} // chatID -> theme color
)

// ChatThemeHandler manages per-chat theme color settings (GET/POST/DELETE).
func ChatThemeHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			chatID := r.URL.Query().Get("chat_id")
			chatThemesMu.RLock()
			color := chatThemes[chatID]
			chatThemesMu.RUnlock()
			if color == "" {
				color = "default"
			}
			writeJSON(w, map[string]any{"ok": true, "chat_id": chatID, "theme": color})
		case http.MethodPost:
			var req struct {
				ChatID string `json:"chat_id"`
				Theme  string `json:"theme"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "bad json", 400)
				return
			}
			if req.Theme == "" || req.Theme == "default" {
				chatThemesMu.Lock()
				delete(chatThemes, req.ChatID)
				chatThemesMu.Unlock()
			} else {
				chatThemesMu.Lock()
				chatThemes[req.ChatID] = req.Theme
				chatThemesMu.Unlock()
			}
			writeJSON(w, map[string]any{"ok": true, "chat_id": req.ChatID, "theme": req.Theme})
		default:
			http.Error(w, "GET or POST required", 405)
		}
	}
}

// ── Encryption Indicators (Cycle 20) ───────────────────────────────────────────

var (
	encMu         sync.RWMutex
	chatEncryption = map[string]string{} // chatID -> encryption type ("e2e", "none", "unknown")
)

// ChatEncryptionHandler gets/sets per-message encryption status for a chat.
// GET  /api/chat/encryption?chat_id=X — returns current encryption type
// POST /api/chat/encryption — sets encryption type {chat_id, encryption}
func ChatEncryptionHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			chatID := r.URL.Query().Get("chat_id")
			encMu.RLock()
			enc := chatEncryption[chatID]
			encMu.RUnlock()
			if enc == "" {
				enc = "unknown"
			}
			writeJSON(w, map[string]any{"ok": true, "chat_id": chatID, "encryption": enc})
		case http.MethodPost:
			var req struct {
				ChatID     string `json:"chat_id"`
				Encryption string `json:"encryption"` // e2e, none, unknown
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
				return
			}
			if req.ChatID == "" || req.Encryption == "" {
				writeJSON(w, map[string]any{"ok": false, "error": "chat_id and encryption required"})
				return
			}
			switch req.Encryption {
			case "e2e", "none", "unknown":
			default:
				writeJSON(w, map[string]any{"ok": false, "error": "encryption must be e2e, none, or unknown"})
				return
			}
			encMu.Lock()
			chatEncryption[req.ChatID] = req.Encryption
			encMu.Unlock()
			writeJSON(w, map[string]any{"ok": true, "chat_id": req.ChatID, "encryption": req.Encryption})
		default:
			http.Error(w, "GET/POST", 405)
		}
	}
}

// ChatLanguageHandler manages user language preference and per-contact language.
var userLanguage = "en"

// Contact language preferences
var (
	contactLangMu   sync.RWMutex
	contactLang     = map[string]string{}    // chatID → language code
	contactAutoTr   = map[string]bool{}      // chatID → auto_translate
	contactLangFile string
)

// InitContactLanguages loads persisted per-contact language preferences from disk.
func InitContactLanguages(dataDir string) {
	contactLangFile = filepath.Join(dataDir, "contact_languages.json")
	b, err := os.ReadFile(contactLangFile)
	if err != nil {
		return
	}
	var data struct {
		Languages     map[string]string `json:"languages"`
		AutoTranslate map[string]bool   `json:"auto_translate"`
	}
	if json.Unmarshal(b, &data) == nil {
		contactLangMu.Lock()
		contactLang = data.Languages
		contactAutoTr = data.AutoTranslate
		contactLangMu.Unlock()
	}
}

func saveContactLanguages() {
	contactLangMu.RLock()
	defer contactLangMu.RUnlock()
	data := map[string]any{
		"languages":     contactLang,
		"auto_translate": contactAutoTr,
	}
	b, _ := json.MarshalIndent(data, "", "  ")
	os.WriteFile(contactLangFile, b, 0600)
}

// ChatLanguageHandler manages user language and per-contact auto-translate settings.
func ChatLanguageHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, map[string]any{"ok": true, "language": userLanguage})
		case http.MethodPost:
			var req struct {
				Language string `json:"language"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "bad json", 400)
				return
			}
			if req.Language != "" {
				userLanguage = req.Language
			}
			writeJSON(w, map[string]any{"ok": true, "language": userLanguage})
		default:
			http.Error(w, "GET or POST required", 405)
		}
	}
}

// ChatContactLanguageHandler manages per-contact language and auto-translate settings.
func ChatContactLanguageHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			chatID := r.URL.Query().Get("chat_id")
			contactLangMu.RLock()
			lang := contactLang[chatID]
			autoTr := contactAutoTr[chatID]
			contactLangMu.RUnlock()
			writeJSON(w, map[string]any{"ok": true, "language": lang, "auto_translate": autoTr})
		case http.MethodPost:
			var req struct {
				ChatID       string `json:"chat_id"`
				Language     string `json:"language"`
				AutoTranslate *bool  `json:"auto_translate"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
				return
			}
			contactLangMu.Lock()
			if req.Language != "" {
				contactLang[req.ChatID] = req.Language
			}
			if req.AutoTranslate != nil {
				contactAutoTr[req.ChatID] = *req.AutoTranslate
			}
			contactLangMu.Unlock()
			saveContactLanguages()
			writeJSON(w, map[string]any{"ok": true})
		default:
			http.Error(w, "GET or POST required", 405)
		}
	}
}

// detectLanguage attempts to detect the language of a text via AI.
func detectLanguage(text string) string {
	client := &http.Client{Timeout: 10 * time.Second}
	prompt := fmt.Sprintf("Respond with ONLY a two-letter language code (en, ru, es) for this text:\n\n%s", text)
	body, _ := json.Marshal(map[string]string{"text": prompt, "user_id": "lang-detect"})
	resp, err := client.Post("http://127.0.0.1:8080/api/ai/chat", "application/json", bytes.NewReader(body))
	if err != nil {
		return "en"
	}
	defer resp.Body.Close()
	var aiResp struct {
		Response string `json:"response"`
	}
	if json.NewDecoder(resp.Body).Decode(&aiResp) != nil || aiResp.Response == "" {
		return "en"
	}
	detected := strings.TrimSpace(strings.ToLower(aiResp.Response))
	if len(detected) >= 2 {
		detected = detected[:2]
	}
	valid := map[string]bool{"en": true, "ru": true, "es": true}
	if valid[detected] {
		return detected
	}
	return "en"
}

// ChatStewardAIHandler routes messages through the AI Steward.
func ChatStewardAIHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", 405)
			return
		}
		var req struct {
			Text      string `json:"text"`
			ChatID    string `json:"chat_id"`
			ContactID int64  `json:"contact_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad json", 400)
			return
		}
		// Forward to AI Steward HTTP endpoint
		go func() {
			client := &http.Client{Timeout: 30 * time.Second}
			body, _ := json.Marshal(map[string]string{
				"text":    req.Text,
				"user_id": fmt.Sprintf("chat-%s", req.ChatID),
			})
			resp, err := client.Post("http://127.0.0.1:8080/api/ai/chat", "application/json", bytes.NewReader(body))
			if err == nil {
				var aiResp struct {
					Response string `json:"response"`
				}
				if json.NewDecoder(resp.Body).Decode(&aiResp) == nil && aiResp.Response != "" {
					msg := ChatMessage{
						ID:        fmt.Sprintf("ai-%d", time.Now().UnixNano()),
						From:      "🤖 Steward",
						Text:      aiResp.Response,
						Timestamp: time.Now().UTC().Format(time.RFC3339),
						IsUser:    false,
						ChatID:    req.ChatID,
						Status:    StatusDelivered,
					}
					GlobalChatHub.AddMessage(msg)
					if BridgeSendFunc != nil && req.ContactID > 0 {
						BridgeSendFunc(aiResp.Response, 1, req.ContactID)
					}
				}
				resp.Body.Close()
			}
		}()
		writeJSON(w, map[string]any{"ok": true, "status": "processing"})
	}
}

// ========================================================================
// Cycle 21: Contact trust/verification system
// ========================================================================

var (
	contactTrustMu sync.RWMutex
	contactTrust   = map[string]string{} // chatID → "trusted"|"verified"|"blocked"
)

// ChatTrustHandler manages contact trust settings.
func ChatTrustHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			chatID := r.URL.Query().Get("chat_id")
			if chatID == "" {
				contactTrustMu.RLock()
				out := make(map[string]string, len(contactTrust))
				for k, v := range contactTrust {
					out[k] = v
				}
				contactTrustMu.RUnlock()
				writeJSON(w, map[string]any{"ok": true, "trust": out})
				return
			}
			contactTrustMu.RLock()
			level := contactTrust[chatID]
			contactTrustMu.RUnlock()
			if level == "" {
				level = "none"
			}
			writeJSON(w, map[string]any{"ok": true, "chat_id": chatID, "trust_level": level})
		case "POST":
			var req struct {
				ChatID string `json:"chat_id"`
				Level  string `json:"level"` // trusted, verified, blocked, none
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
				return
			}
			if req.ChatID == "" {
				writeJSON(w, map[string]any{"ok": false, "error": "chat_id required"})
				return
			}
			valid := map[string]bool{"trusted": true, "verified": true, "blocked": true, "none": true}
			if !valid[req.Level] {
				req.Level = "none"
			}
			contactTrustMu.Lock()
			if req.Level == "none" {
				delete(contactTrust, req.ChatID)
			} else {
				contactTrust[req.ChatID] = req.Level
			}
			contactTrustMu.Unlock()
			auditInfo("contact_trust", fmt.Sprintf("chat_id=%s, level=%s", req.ChatID, req.Level))
			writeJSON(w, map[string]any{"ok": true, "chat_id": req.ChatID, "trust_level": req.Level})
		default:
			http.Error(w, "GET/POST", 405)
		}
	}
}

// ========================================================================
// Cycle 23: Media gallery — list messages with media attachments
// ========================================================================

// ChatMediaHandler handles media file attachments.
func ChatMediaHandler(hub *ChatHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		chatID := r.URL.Query().Get("chat_id")
		mediaType := r.URL.Query().Get("type") // voice, file, image
		all := hub.GetMessages()
		out := make([]ChatMessage, 0)
		for _, m := range all {
			if chatID != "" && m.ChatID != chatID {
				continue
			}
			if m.Recalled {
				continue
			}
			hasMedia := false
			if mediaType == "" || mediaType == "voice" {
				if m.VoiceURL != "" {
					hasMedia = true
				}
			}
			if mediaType == "" || mediaType == "file" {
				if m.FileURL != "" {
					hasMedia = true
				}
			}
			if hasMedia {
				out = append(out, m)
			}
		}
		writeJSON(w, map[string]any{"ok": true, "media": out, "count": len(out)})
	}
}

// ========================================================================
// Cycle 24: Message translation (EN↔RU↔ES via Steward AI)
// ========================================================================

// ChatTranslateHandler translates a message text.
func ChatTranslateHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		var req struct {
			Text        string `json:"text"`
			TargetLang  string `json:"target_lang"`  // ru, en, es
			SourceLang  string `json:"source_lang"`  // auto if empty
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
			return
		}
		if req.Text == "" || req.TargetLang == "" {
			writeJSON(w, map[string]any{"ok": false, "error": "text and target_lang required"})
			return
		}
		validLangs := map[string]bool{"en": true, "ru": true, "es": true}
		if !validLangs[req.TargetLang] {
			writeJSON(w, map[string]any{"ok": false, "error": "unsupported target language (en/ru/es)"})
			return
		}
		source := req.SourceLang
		if source == "" {
			source = "auto"
		}
		// Use AI Steward for translation
		prompt := fmt.Sprintf("Translate the following text to %s. Source language: %s. Return ONLY the translated text, no explanations:\n\n%s", req.TargetLang, source, req.Text)
		client := &http.Client{Timeout: 30 * time.Second}
		body, _ := json.Marshal(map[string]string{"text": prompt, "user_id": "translate-bot"})
		resp, err := client.Post("http://127.0.0.1:8080/api/ai/chat", "application/json", bytes.NewReader(body))
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "ai not available"})
			return
		}
		defer resp.Body.Close()
		var aiResp struct {
			Response string `json:"response"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&aiResp); err != nil || aiResp.Response == "" {
			writeJSON(w, map[string]any{"ok": false, "error": "ai response empty"})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "translated": aiResp.Response, "target_lang": req.TargetLang, "source_lang": source})
	}
}

// ========================================================================
// Cycle 25: Link preview — fetch OG metadata from URL
// ========================================================================

// LinkPreview holds Open Graph metadata extracted from a URL.
type LinkPreview struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Image       string `json:"image"`
	SiteName    string `json:"site_name"`
}

// ChatLinkPreviewHandler generates link previews for URLs.
func ChatLinkPreviewHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		var req struct {
			URL string `json:"url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.URL == "" {
			writeJSON(w, map[string]any{"ok": false, "error": "url required"})
			return
		}
		// Fetch the URL and extract OG tags via simple regex
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Get(req.URL)
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "fetch failed"})
			return
		}
		defer resp.Body.Close()
		var buf bytes.Buffer
		io.CopyN(&buf, resp.Body, 65536) // read up to 64KB
		html := buf.String()

		preview := LinkPreview{URL: req.URL}
		// Simple extraction of OG tags
		if m := regexp.MustCompile(`<meta\s+property="og:title"\s+content="([^"]*)"`).FindStringSubmatch(html); len(m) > 1 {
			preview.Title = m[1]
		} else if m := regexp.MustCompile(`<title>([^<]*)</title>`).FindStringSubmatch(html); len(m) > 1 {
			preview.Title = m[1]
		}
		if m := regexp.MustCompile(`<meta\s+property="og:description"\s+content="([^"]*)"`).FindStringSubmatch(html); len(m) > 1 {
			preview.Description = m[1]
		}
		if m := regexp.MustCompile(`<meta\s+property="og:image"\s+content="([^"]*)"`).FindStringSubmatch(html); len(m) > 1 {
			preview.Image = m[1]
		}
		if m := regexp.MustCompile(`<meta\s+property="og:site_name"\s+content="([^"]*)"`).FindStringSubmatch(html); len(m) > 1 {
			preview.SiteName = m[1]
		}
		writeJSON(w, map[string]any{"ok": true, "preview": preview})
	}
}

// ========================================================================
// Cycle 26: Custom notification sounds per contact
// ========================================================================

var (
	contactSoundsMu sync.RWMutex
	contactSounds   = map[string]string{} // chatID → sound name
)

// ChatSoundHandler manages notification sound settings.
func ChatSoundHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			chatID := r.URL.Query().Get("chat_id")
			if chatID == "" {
				contactSoundsMu.RLock()
				out := make(map[string]string, len(contactSounds))
				for k, v := range contactSounds {
					out[k] = v
				}
				contactSoundsMu.RUnlock()
				writeJSON(w, map[string]any{"ok": true, "sounds": out})
				return
			}
			contactSoundsMu.RLock()
			sound := contactSounds[chatID]
			contactSoundsMu.RUnlock()
			if sound == "" {
				sound = "default"
			}
			writeJSON(w, map[string]any{"ok": true, "chat_id": chatID, "sound": sound})
		case "POST":
			var req struct {
				ChatID string `json:"chat_id"`
				Sound  string `json:"sound"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
				return
			}
			if req.ChatID == "" {
				writeJSON(w, map[string]any{"ok": false, "error": "chat_id required"})
				return
			}
			contactSoundsMu.Lock()
			if req.Sound == "" || req.Sound == "default" {
				delete(contactSounds, req.ChatID)
			} else {
				contactSounds[req.ChatID] = req.Sound
			}
			contactSoundsMu.Unlock()
			writeJSON(w, map[string]any{"ok": true, "chat_id": req.ChatID, "sound": req.Sound})
		default:
			http.Error(w, "GET/POST", 405)
		}
	}
}

// ========================================================================
// Cycle 27: Steward AI context-aware suggestions
// ========================================================================

// ChatSuggestHandler suggests auto-replies based on message context.
func ChatSuggestHandler(hub *ChatHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		var req struct {
			ChatID    string `json:"chat_id"`
			ContactID int64  `json:"contact_id"`
			Context   string `json:"context"` // optional user-provided context
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
			return
		}
		// Gather recent messages for context
		all := hub.GetMessages()
		recent := make([]ChatMessage, 0)
		for i := len(all) - 1; i >= 0; i-- {
			if req.ChatID == "" || all[i].ChatID == req.ChatID {
				recent = append(recent, all[i])
				if len(recent) >= 10 {
					break
				}
			}
		}
		// Build context prompt
		var contextStr string
		if req.Context != "" {
			contextStr = req.Context
		} else {
			contextStr = "Recent conversation:\n"
			for i := len(recent) - 1; i >= 0; i-- {
				m := recent[i]
				sender := "User"
				if !m.IsUser {
					sender = "Contact"
				}
				contextStr += fmt.Sprintf("%s: %s\n", sender, m.Text[:min(len(m.Text), 200)])
			}
		}
		prompt := fmt.Sprintf(`Based on this conversation context, suggest 3-5 short reply options (one per line, max 60 chars each). Be helpful and context-aware. No numbering or prefixes:

%s

Suggested replies:`, contextStr)

		client := &http.Client{Timeout: 30 * time.Second}
		body, _ := json.Marshal(map[string]string{"text": prompt, "user_id": "suggest-bot"})
		resp, err := client.Post("http://127.0.0.1:8080/api/ai/chat", "application/json", bytes.NewReader(body))
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "ai not available"})
			return
		}
		defer resp.Body.Close()
		var aiResp struct {
			Response string `json:"response"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&aiResp); err != nil || aiResp.Response == "" {
			writeJSON(w, map[string]any{"ok": false, "error": "ai response empty"})
			return
		}
		// Parse suggestions (one per line)
		lines := strings.Split(strings.TrimSpace(aiResp.Response), "\n")
		suggestions := make([]string, 0, len(lines))
		for _, l := range lines {
			l = strings.TrimSpace(l)
			// Strip numbering patterns like "1. ", "- ", etc
			l = regexp.MustCompile(`^[\d\-•*]+[\.\)]?\s*`).ReplaceAllString(l, "")
			if l != "" && len(l) < 120 {
				suggestions = append(suggestions, l)
			}
		}
		if len(suggestions) == 0 {
			suggestions = []string{"Tell me more", "Interesting", "I see"}
		}
		writeJSON(w, map[string]any{"ok": true, "suggestions": suggestions})
	}
}

// ========================================================================
// Cycle 28: Bulk message operations (bulk delete / bulk forward)
// ========================================================================

// ChatBulkDeleteHandler deletes multiple messages by ID list.
func ChatBulkDeleteHandler(hub *ChatHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		var req struct {
			IDs []string `json:"ids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
			return
		}
		if len(req.IDs) == 0 {
			writeJSON(w, map[string]any{"ok": false, "error": "ids required"})
			return
		}
		idSet := make(map[string]bool, len(req.IDs))
		for _, id := range req.IDs {
			idSet[id] = true
		}
		hub.mu.Lock()
		filtered := make([]ChatMessage, 0, len(hub.messages))
		for _, m := range hub.messages {
			if !idSet[m.ID] {
				filtered = append(filtered, m)
			}
		}
		removed := len(hub.messages) - len(filtered)
		hub.messages = filtered
		hub.mu.Unlock()
		hub.flush()
		writeJSON(w, map[string]any{"ok": true, "removed": removed})
	}
}

// ChatBulkForwardHandler forwards multiple messages to a contact.
func ChatBulkForwardHandler(hub *ChatHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		var req struct {
			IDs          []string `json:"ids"`
			TargetChatID string   `json:"target_chat_id"`
			TargetContactID int64 `json:"target_contact_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
			return
		}
		if len(req.IDs) == 0 || req.TargetChatID == "" {
			writeJSON(w, map[string]any{"ok": false, "error": "ids and target_chat_id required"})
			return
		}
		all := hub.GetMessages()
		idSet := make(map[string]bool, len(req.IDs))
		for _, id := range req.IDs {
			idSet[id] = true
		}
		forwarded := 0
		for _, m := range all {
			if idSet[m.ID] && !m.Recalled {
				fwdMsg := ChatMessage{
					ID:          fmt.Sprintf("fwd-%d-%d", time.Now().UnixNano(), forwarded),
					From:        "admin",
					Text:        m.Text,
					Timestamp:   time.Now().UTC().Format(time.RFC3339),
					IsUser:      true,
					ChatID:      req.TargetChatID,
					Status:      StatusSent,
					MoneyAmount: m.MoneyAmount,
					MoneyAsset:  m.MoneyAsset,
					VoiceURL:    m.VoiceURL,
					VoiceDur:    m.VoiceDur,
					FileName:    m.FileName,
					FileURL:     m.FileURL,
					FileSize:    m.FileSize,
				}
				hub.AddMessage(fwdMsg)
				if BridgeSendFunc != nil && req.TargetContactID > 0 {
					BridgeSendFunc(m.Text, 1, req.TargetContactID)
				}
				forwarded++
			}
		}
		writeJSON(w, map[string]any{"ok": true, "forwarded": forwarded})
	}
}

// ========================================================================
// Cycle 29: Contact online/status indicators
// ========================================================================

var (
	contactStatusMu sync.RWMutex
	contactStatus   = map[string]string{} // chatID → "online"|"away"|"offline"
	contactSeen     = map[string]string{} // chatID → last seen timestamp
)

// ChatContactStatusHandler sets or gets presence status for a contact.
func ChatContactStatusHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "POST":
			var req struct {
				ChatID string `json:"chat_id"`
				Status string `json:"status"` // online, away, offline
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
				return
			}
			if req.ChatID == "" {
				writeJSON(w, map[string]any{"ok": false, "error": "chat_id required"})
				return
			}
			valid := map[string]bool{"online": true, "away": true, "offline": true}
			if !valid[req.Status] {
				req.Status = "offline"
			}
			contactStatusMu.Lock()
			contactStatus[req.ChatID] = req.Status
			contactSeen[req.ChatID] = time.Now().UTC().Format(time.RFC3339)
			contactStatusMu.Unlock()
			writeJSON(w, map[string]any{"ok": true, "chat_id": req.ChatID, "status": req.Status})
		case "GET":
			chatID := r.URL.Query().Get("chat_id")
			contactStatusMu.RLock()
			if chatID != "" {
				st := contactStatus[chatID]
				if st == "" {
					st = "offline"
				}
				seen := contactSeen[chatID]
				contactStatusMu.RUnlock()
				writeJSON(w, map[string]any{"ok": true, "chat_id": chatID, "status": st, "last_seen": seen})
				return
			}
			// Return all statuses
			out := make(map[string]string, len(contactStatus))
			for k, v := range contactStatus {
				out[k] = v
			}
			contactStatusMu.RUnlock()
			writeJSON(w, map[string]any{"ok": true, "statuses": out})
		default:
			http.Error(w, "GET/POST", 405)
		}
	}
}

// ========================================================================
// Cycle 30: Enhanced audit log (security events)
// ========================================================================

// SecurityEvent records a security-relevant action for audit logging.
type SecurityEvent struct {
	ID        string `json:"id"`
	Event     string `json:"event"`
	Detail    string `json:"detail"`
	Timestamp string `json:"timestamp"`
	Severity  string `json:"severity"` // info, warning, critical
}

var (
	secAuditMu sync.RWMutex
	secAudit   = make([]SecurityEvent, 0, 1000)
	secAuditSeq int64
)

func addSecAudit(event, detail, severity string) {
	secAuditMu.Lock()
	defer secAuditMu.Unlock()
	secAuditSeq++
	e := SecurityEvent{
		ID:        fmt.Sprintf("sec-%d", secAuditSeq),
		Event:     event,
		Detail:    detail,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Severity:  severity,
	}
	secAudit = append(secAudit, e)
	if len(secAudit) > 1000 {
		secAudit = secAudit[len(secAudit)-1000:]
	}
}

// Internal audit helpers
func auditInfo(event, detail string)     { addSecAudit(event, detail, "info") }
func auditWarn(event, detail string)     { addSecAudit(event, detail, "warning") }
func auditCritical(event, detail string) { addSecAudit(event, detail, "critical") }

// AuditLogEnhancedHandler returns enhanced audit log with filtering.
func AuditLogEnhancedHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			severity := r.URL.Query().Get("severity")
			since := r.URL.Query().Get("since")
			limitStr := r.URL.Query().Get("limit")
			limit := 100
			if n, err := fmt.Sscanf(limitStr, "%d", &limit); err != nil || n != 1 || limit < 1 || limit > 500 {
				limit = 100
			}
			var sinceTime time.Time
			if since != "" {
				sinceTime, _ = time.Parse(time.RFC3339, since)
			}
			secAuditMu.RLock()
			out := make([]SecurityEvent, 0, len(secAudit))
			for _, e := range secAudit {
				if severity != "" && e.Severity != severity {
					continue
				}
				if !sinceTime.IsZero() {
					t, err := time.Parse(time.RFC3339, e.Timestamp)
					if err != nil || t.Before(sinceTime) {
						continue
					}
				}
				out = append(out, e)
			}
			secAuditMu.RUnlock()
			// Apply limit from end
			if len(out) > limit {
				out = out[len(out)-limit:]
			}
			writeJSON(w, map[string]any{"ok": true, "events": out, "count": len(out)})
		case "POST":
			// Manually add an audit event
			var req struct {
				Event    string `json:"event"`
				Detail   string `json:"detail"`
				Severity string `json:"severity"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
				return
			}
			if req.Event == "" {
				writeJSON(w, map[string]any{"ok": false, "error": "event required"})
				return
			}
			sev := req.Severity
			if sev != "info" && sev != "warning" && sev != "critical" {
				sev = "info"
			}
			addSecAudit(req.Event, req.Detail, sev)
			writeJSON(w, map[string]any{"ok": true})
		default:
			http.Error(w, "GET/POST", 405)
		}
	}
}

// ── Bridge Latency Ring Buffer (C35) ───────────────────────────────────────────

var (
	latencyMu   sync.Mutex
	latencyRing [100]time.Duration
	latencyPos  int
	latencyFull bool
)

// RecordBridgeLatency records a bridge command latency sample in a 100-element ring buffer.
func RecordBridgeLatency(d time.Duration) {
	latencyMu.Lock()
	latencyRing[latencyPos] = d
	latencyPos = (latencyPos + 1) % 100
	if latencyPos == 0 {
		latencyFull = true
	}
	latencyMu.Unlock()
}

// BridgeLatencyHandler returns bridge command latency statistics (avg, min, max, samples).
func BridgeLatencyHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		latencyMu.Lock()
		var sum time.Duration
		var minLat, maxLat time.Duration
		count := 0
		n := latencyPos
		if latencyFull {
			n = 100
		}
		for i := 0; i < n; i++ {
			d := latencyRing[i]
			if d == 0 {
				continue
			}
			sum += d
			if count == 0 || d < minLat {
				minLat = d
			}
			if d > maxLat {
				maxLat = d
			}
			count++
		}
		var avg float64
		if count > 0 {
			avg = float64(sum.Microseconds()) / float64(count) / 1000.0
		}
		var lastMs float64
		if n > 0 {
			lastMs = float64(latencyRing[(latencyPos-1+100)%100].Microseconds()) / 1000.0
		}
		latencyMu.Unlock()

		writeJSON(w, map[string]any{
			"avg_ms":  avg,
			"min_ms":  float64(minLat.Microseconds()) / 1000.0,
			"max_ms":  float64(maxLat.Microseconds()) / 1000.0,
			"samples": count,
			"last_ms": lastMs,
		})
	}
}

// writeJSON writes JSON to response
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// requireContentType checks that POST/PUT/PATCH requests have application/json Content-Type.
func requireContentType(r *http.Request) bool {
	ct := r.Header.Get("Content-Type")
	// Empty Content-Type is rejected for mutating methods
	if ct == "" {
		return false
	}
	return strings.HasPrefix(ct, "application/json")
}

// validateDateParam validates a date string in RFC3339 format.
func validateDateParam(v string) bool {
	_, err := time.Parse(time.RFC3339, v)
	return err == nil
}

// validateIntParam validates and parses an integer query param.
func validateIntParam(v string, defaultVal int) int {
	if v == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return defaultVal
	}
	return n
}

// min helper
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
