// Package bridge provides WebSocket bridge to the simplex-chat CLI with auto-reconnect
package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"log/slog"

	"ParanoidX/internal/api"
	"ParanoidX/internal/transport"
)

// Bridge manages the WebSocket connection to the simplex-chat CLI process.
// It handles command/response correlation, auto-reconnection, and
// message routing between the HTTP API and the SimpleX network.
type Bridge struct {
	// DataDir is the server data directory for storing chat state.
	DataDir string
	cliBin  string
}


// New handles the New HTTP request.
func New(dataDir string) *Bridge {
	bin := filepath.Join(os.Getenv("HOME"), "bin", "simplex-chat-island")
	if _, err := os.Stat(bin); err != nil {
		bin = "/home/tomas/bin/simplex-chat-island"
	}
	return &Bridge{
		DataDir: dataDir,
		cliBin:  bin,
	}
}

var (
	pendingResp = map[string]chan map[string]any{}
	pendingMu   sync.Mutex
	wsConn      *websocket.Conn
	wsMu        sync.Mutex

	reconnectTrigger chan struct{}
	reconnectOnce    sync.Once

	// msgBuffer holds outgoing messages when bridge is disconnected.
	msgBuffer   []string
	msgBufferMu sync.Mutex
)

func init() {
	reconnectTrigger = make(chan struct{}, 1)
}

// TriggerReconnect sends a signal to the bridge to reconnect immediately.
func TriggerReconnect() {
	select {
	case reconnectTrigger <- struct{}{}:
	default:
	}
}

// BridgeReconnectHandler returns the HTTP handler for manual reconnect.
func BridgeReconnectHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, `{"error":"POST required"}`, http.StatusMethodNotAllowed)
			return
		}
		TriggerReconnect()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "message": "reconnect triggered"})
	}
}

// bufferOutgoing adds a message to the buffer for later replay.
func bufferOutgoing(msg string) {
	msgBufferMu.Lock()
	defer msgBufferMu.Unlock()
	msgBuffer = append(msgBuffer, msg)
	if len(msgBuffer) > 200 {
		msgBuffer = msgBuffer[len(msgBuffer)-200:]
	}
}

// flushBuffer sends all buffered messages through the active WS connection.
func flushBuffer() {
	msgBufferMu.Lock()
	buf := make([]string, len(msgBuffer))
	copy(buf, msgBuffer)
	msgBuffer = nil
	msgBufferMu.Unlock()
	for _, msg := range buf {
		wsMu.Lock()
		if wsConn != nil {
			wsConn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			wsConn.WriteMessage(websocket.TextMessage, []byte(msg))
		}
		wsMu.Unlock()
	}
	if len(buf) > 0 {
		slog.Info("[bridge] flushed buffered messages", "count", len(buf))
	}
}

func wsWrite(msg string) error {
	wsMu.Lock()
	wsConnLocal := wsConn
	wsMu.Unlock()
	if wsConnLocal == nil {
		bufferOutgoing(msg)
		return nil
	}
	wsMu.Lock()
	wsConn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	err := wsConn.WriteMessage(websocket.TextMessage, []byte(msg))
	if err == nil {
		api.BridgeLastWsPing = time.Now()
	}
	wsMu.Unlock()
	if err != nil {
		bufferOutgoing(msg)
	}
	pendingMu.Lock()
	api.BridgeMsgQueueDepth = int64(len(pendingResp))
	if api.BridgeMsgQueueDepth > 50 {
		slog.Warn("[bridge] message queue depth high", "depth", api.BridgeMsgQueueDepth)
	}
	pendingMu.Unlock()
	return err
}

// sendCmdFn is registered as api.SimplexCmd for HTTP handlers.
func sendCmdFn(cmd string) (map[string]any, error) {
	corr := fmt.Sprintf("go-%d", time.Now().UnixNano())
	ch := make(chan map[string]any, 1)

	pendingMu.Lock()
	pendingResp[corr] = ch
	pendingMu.Unlock()

	defer func() {
		pendingMu.Lock()
		delete(pendingResp, corr)
		pendingMu.Unlock()
	}()

	msg := fmt.Sprintf(`{"corrId":"%s","cmd":%s}`, corr, mustJSON(cmd))
	started := time.Now()
	if err := wsWrite(msg); err != nil {
		api.BridgeLastCmdLatency = time.Since(started)
		api.BridgeCmdCount++
		return nil, err
	}

	select {
	case resp := <-ch:
		api.BridgeLastCmdLatency = time.Since(started)
		api.BridgeCmdCount++
		if api.BridgeMinLatency == 0 || time.Since(started) < api.BridgeMinLatency {
			api.BridgeMinLatency = time.Since(started)
		}
		if time.Since(started) > api.BridgeMaxLatency {
			api.BridgeMaxLatency = time.Since(started)
		}
		return resp, nil
	case <-time.After(15 * time.Second):
		api.BridgeLastCmdLatency = time.Since(started)
		api.BridgeCmdCount++
		return nil, fmt.Errorf("command timeout")
	}
}

func mustJSON(v string) string {
	b, _ := json.Marshal(v)
	return string(b)
}


// RunContext handles the RunContext HTTP request.
func (b *Bridge) RunContext(ctx context.Context) {
	if _, err := os.Stat(b.cliBin); err != nil {
		slog.Warn("[bridge] simplex-chat-island not found, bridge disabled", "path", b.cliBin)
		return
	}

	simplexDir := filepath.Join(b.DataDir, "island-bot")
	if err := os.MkdirAll(simplexDir, 0700); err != nil {
		slog.Error("[bridge] failed to create island-bot dir", "error", err)
		return
	}

	dbFile := filepath.Join(simplexDir, "simplex_v1")
	bridgePort := 17225

	api.SimplexCmd = sendCmdFn

	api.BridgeSendFunc = func(text string, userID, contactID int64) {
		if contactID <= 0 {
			return
		}
		msg := fmt.Sprintf(`{"corrId":"snd-%d","cmd":"/_send @%d json [{\"msgContent\":{\"type\":\"text\",\"text\":%s}}]"}`,
			time.Now().UnixNano(), contactID, mustJSON(text))
		wsWrite(msg)
	}

	backoff := 1 * time.Second
	const maxBackoff = 30 * time.Second

	for {
		select {
		case <-ctx.Done():
			slog.Info("[bridge] context cancelled, stopping")
			return
		case <-reconnectTrigger:
			slog.Info("[bridge] manual reconnect triggered")
		default:
		}
		old := exec.Command("sh", "-c", fmt.Sprintf("lsof -ti :%d 2>/dev/null | xargs -r kill 2>/dev/null", bridgePort))
		old.Run()

		cmd := exec.CommandContext(ctx, b.cliBin,
			"-d", dbFile,
			"-k", "test123",
			"-p", fmt.Sprintf("%d", bridgePort),
			"-x",
			"--create-bot-display-name", "Island Bot",
			"--create-bot-allow-files",
			"-y",
		)
		cmd.Env = append(os.Environ(), "SIMPLEX_AGENT=island-bot")
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			slog.Error("[bridge] stdout pipe", "error", err)
			slog.Info("[bridge] retrying in", "backoff", backoff)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff = minDuration(backoff*2, maxBackoff)
			continue
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			slog.Error("[bridge] stderr pipe", "error", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff = minDuration(backoff*2, maxBackoff)
			continue
		}
		if err := cmd.Start(); err != nil {
			slog.Error("[bridge] start", "error", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff = minDuration(backoff*2, maxBackoff)
			continue
		}

		go io.Copy(os.Stdout, stdout)
		go io.Copy(os.Stderr, stderr)

		time.Sleep(3 * time.Second)

		wsURL := fmt.Sprintf("ws://127.0.0.1:%d", bridgePort)
		c, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			slog.Warn("[bridge] WS dial failed — retrying", "url", wsURL, "error", err)
			cmd.Process.Kill()
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff = minDuration(backoff*2, maxBackoff)
			continue
		}

		wsMu.Lock()
		wsConn = c
		wsMu.Unlock()
		api.BridgeConnected = true
		api.BridgeConnectedSince = time.Now().UTC().Format(time.RFC3339)

		slog.Info("[bridge] connected to simplex-chat CLI via WS")
		backoff = 1 * time.Second

		flushBuffer()

		go b.heartbeatLoop(ctx)

		b.msgLoopCtx(ctx, c)

		wsMu.Lock()
		wsConn = nil
		wsMu.Unlock()
		c.Close()
		cmd.Process.Kill()
		api.BridgeConnected = false
		api.BridgeLastDisconnect = time.Now().UTC().Format(time.RFC3339)
		api.BridgeReconnectCount++

		slog.Warn("[bridge] disconnected, reconnecting", "backoff", backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = minDuration(backoff*2, maxBackoff)
	}
}

func (b *Bridge) msgLoopCtx(ctx context.Context, c *websocket.Conn) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		_, message, err := c.ReadMessage()
		if err != nil {
			return
		}
		var raw struct {
			CorrID json.RawMessage `json:"corrId"`
			Resp   json.RawMessage `json:"resp"`
		}
		if err := json.Unmarshal(message, &raw); err != nil {
			continue
		}
		var corrID string
		if raw.CorrID != nil {
			json.Unmarshal(raw.CorrID, &corrID)
		}
		var respMap map[string]any
		if raw.Resp != nil {
			json.Unmarshal(raw.Resp, &respMap)
		}
		if corrID != "" {
			pendingMu.Lock()
			ch, ok := pendingResp[corrID]
			pendingMu.Unlock()
			if ok && ch != nil && respMap != nil {
				select {
				case ch <- respMap:
				default:
				}
				continue
			}
		}
		if respMap == nil {
			continue
		}
		rType, _ := respMap["type"].(string)
		if rType == "newChatItems" {
			chatItems, _ := respMap["chatItems"].([]any)
			for _, item := range chatItems {
				if itemMap, ok := item.(map[string]any); ok {
					content, _ := itemMap["content"].(map[string]any)
					msgContent, _ := content["msgContent"].(map[string]any)
					text, _ := msgContent["text"].(string)
					chatInfo, _ := respMap["chatInfo"].(map[string]any)
					var contactID int64
					var contactName string
					if chatInfo != nil {
						contact, _ := chatInfo["contact"].(map[string]any)
						if contact != nil {
							if id, ok := contact["contactId"].(float64); ok {
								contactID = int64(id)
							}
							contactName, _ = contact["localDisplayName"].(string)
							if contactName == "" {
								contactName, _ = contact["displayName"].(string)
							}
						}
					}
					if text != "" && contactID > 0 {
						msgID := fmt.Sprintf("in-%d", time.Now().UnixNano())
						api.GlobalChatHub.AddMessage(api.ChatMessage{
							ID:        msgID,
							From:      contactName,
							Text:      text,
							Timestamp: time.Now().UTC().Format(time.RFC3339),
							IsUser:    false,
							ChatID:    fmt.Sprintf("@%d", contactID),
						})
						if transport.GlobalTransport != nil {
							transport.GlobalTransport.BroadcastAll(transport.WSMessage{
								Type:      "message",
								ContactID: contactID,
								Text:      text,
							})
						}
						go b.replyAsync(text, contactID)
					}
				}
			}
		}
		if rType == "userContactLinkCreated" || rType == "userContactLinkUpdated" {
			if clc, ok := respMap["connLinkContact"].(map[string]any); ok {
				if cfl, _ := clc["connFullLink"].(string); cfl != "" {
					go b.updateContact(cfl)
				}
			}
		}
		if rType == "contactRequest" || strings.HasPrefix(corrID, "hello") {
			if chatInfo, ok := respMap["chatInfo"].(map[string]any); ok {
				if contactReq, ok := chatInfo["contactRequest"].(map[string]any); ok {
					if reqID, ok := contactReq["contactReqId"].(float64); ok {
						acceptCmd := fmt.Sprintf(`{"corrId":"acpt-%d","cmd":"/_accept %d"}`, time.Now().UnixNano(), int64(reqID))
						wsWrite(acceptCmd)
					}
				}
			}
		}
	}
}


// Run handles the Run HTTP request.
func (b *Bridge) Run() {
	if _, err := os.Stat(b.cliBin); err != nil {
		slog.Warn("[bridge] simplex-chat-island not found, bridge disabled", "path", b.cliBin)
		return
	}

	simplexDir := filepath.Join(b.DataDir, "island-bot")
	if err := os.MkdirAll(simplexDir, 0700); err != nil {
		slog.Error("[bridge] failed to create island-bot dir", "error", err)
		return
	}

	dbFile := filepath.Join(simplexDir, "simplex_v1")
	bridgePort := 17225

	api.SimplexCmd = sendCmdFn

	api.BridgeSendFunc = func(text string, userID, contactID int64) {
		if contactID <= 0 {
			return
		}
		msg := fmt.Sprintf(`{"corrId":"snd-%d","cmd":"/_send @%d json [{\"msgContent\":{\"type\":\"text\",\"text\":%s}}]"}`,
			time.Now().UnixNano(), contactID, mustJSON(text))
		wsWrite(msg)
	}

	for {
		// Kill any stale simplex-chat-island process on the same port
		old := exec.Command("sh", "-c", fmt.Sprintf("lsof -ti :%d 2>/dev/null | xargs -r kill 2>/dev/null", bridgePort))
		old.Run()

		cmd := exec.Command(b.cliBin,
			"-d", dbFile,
			"-k", "test123",
			"-p", fmt.Sprintf("%d", bridgePort),
			"-x",
			"--create-bot-display-name", "Island Bot",
			"--create-bot-allow-files",
			"-y",
		)
		cmd.Env = append(os.Environ(), "SIMPLEX_AGENT=island-bot")
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			slog.Error("[bridge] stdout pipe", "error", err)
			time.Sleep(15 * time.Second)
			continue
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			slog.Error("[bridge] stderr pipe", "error", err)
			time.Sleep(15 * time.Second)
			continue
		}
		if err := cmd.Start(); err != nil {
			slog.Error("[bridge] start", "error", err)
			time.Sleep(15 * time.Second)
			continue
		}

		go io.Copy(os.Stdout, stdout)
		go io.Copy(os.Stderr, stderr)

		time.Sleep(3 * time.Second)

		wsURL := fmt.Sprintf("ws://127.0.0.1:%d", bridgePort)
		c, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			slog.Warn("[bridge] WS dial failed — retrying", "url", wsURL, "error", err)
			cmd.Process.Kill()
			time.Sleep(15 * time.Second)
			continue
		}

		wsMu.Lock()
		wsConn = c
		wsMu.Unlock()
		api.BridgeConnected = true
		api.BridgeConnectedSince = time.Now().UTC().Format(time.RFC3339)

		slog.Info("[bridge] connected to simplex-chat CLI via WS")

		flushBuffer()

		b.msgLoop(c)

		wsMu.Lock()
		wsConn = nil
		wsMu.Unlock()
		c.Close()
		cmd.Process.Kill()
		api.BridgeConnected = false
		api.BridgeLastDisconnect = time.Now().UTC().Format(time.RFC3339)
		api.BridgeReconnectCount++

		slog.Warn("[bridge] disconnected, reconnecting in 15s")
		time.Sleep(15 * time.Second)
	}
}

func (b *Bridge) msgLoop(c *websocket.Conn) {
	for {
		_, message, err := c.ReadMessage()
		if err != nil {
			return
		}

		var raw struct {
			CorrID json.RawMessage `json:"corrId"`
			Resp   json.RawMessage `json:"resp"`
		}
		if err := json.Unmarshal(message, &raw); err != nil {
			continue
		}

		var corrID string
		if raw.CorrID != nil {
			json.Unmarshal(raw.CorrID, &corrID)
		}

		var respMap map[string]any
		if raw.Resp != nil {
			json.Unmarshal(raw.Resp, &respMap)
		}

		if corrID != "" {
			pendingMu.Lock()
			ch, ok := pendingResp[corrID]
			pendingMu.Unlock()
			if ok && ch != nil && respMap != nil {
				select {
				case ch <- respMap:
				default:
				}
				continue
			}
		}

		if respMap == nil {
			continue
		}

		rType, _ := respMap["type"].(string)

		if rType == "newChatItems" {
			chatItems, _ := respMap["chatItems"].([]any)
			for _, item := range chatItems {
				if itemMap, ok := item.(map[string]any); ok {
					content, _ := itemMap["content"].(map[string]any)
					msgContent, _ := content["msgContent"].(map[string]any)
					text, _ := msgContent["text"].(string)

					chatInfo, _ := respMap["chatInfo"].(map[string]any)
					var contactID int64
					var contactName string
					if chatInfo != nil {
						contact, _ := chatInfo["contact"].(map[string]any)
						if contact != nil {
							if id, ok := contact["contactId"].(float64); ok {
								contactID = int64(id)
							}
							contactName, _ = contact["localDisplayName"].(string)
							if contactName == "" {
								contactName, _ = contact["displayName"].(string)
							}
						}
					}

					if text != "" && contactID > 0 {
						msgID := fmt.Sprintf("in-%d", time.Now().UnixNano())
						api.GlobalChatHub.AddMessage(api.ChatMessage{
							ID:        msgID,
							From:      contactName,
							Text:      text,
							Timestamp: time.Now().UTC().Format(time.RFC3339),
							IsUser:    false,
							ChatID:    fmt.Sprintf("@%d", contactID),
						})
						if transport.GlobalTransport != nil {
							transport.GlobalTransport.BroadcastAll(transport.WSMessage{
								Type:      "message",
								ContactID: contactID,
								Text:      text,
							})
						}
						go b.replyAsync(text, contactID)
					}
				}
			}
		}

		if rType == "userContactLinkCreated" || rType == "userContactLinkUpdated" {
			if clc, ok := respMap["connLinkContact"].(map[string]any); ok {
				if cfl, _ := clc["connFullLink"].(string); cfl != "" {
					go b.updateContact(cfl)
				}
			}
		}

		if rType == "contactRequest" || strings.HasPrefix(corrID, "hello") {
			if chatInfo, ok := respMap["chatInfo"].(map[string]any); ok {
				if contactReq, ok := chatInfo["contactRequest"].(map[string]any); ok {
					if reqID, ok := contactReq["contactReqId"].(float64); ok {
						acceptCmd := fmt.Sprintf(`{"corrId":"acpt-%d","cmd":"/_accept %d"}`, time.Now().UnixNano(), int64(reqID))
						wsWrite(acceptCmd)
					}
				}
			}
		}
	}
}

func (b *Bridge) replyAsync(text string, contactID int64) {
	done := make(chan string, 1)
	go func() {
		done <- processWalletCommand(text, fmt.Sprintf("@%d", contactID), b.DataDir)
	}()
	select {
	case response := <-done:
		reply := fmt.Sprintf(`{"corrId":"rpl-%d","cmd":"/_send @%d json [{\"msgContent\":{\"type\":\"text\",\"text\":%s}}]"}`,
			time.Now().UnixNano(), contactID, mustJSON(response))
		wsWrite(reply)
	case <-time.After(30 * time.Second):
		reply := fmt.Sprintf(`{"corrId":"rpl-%d","cmd":"/_send @%d json [{\"msgContent\":{\"type\":\"text\",\"text\":%s}}]"}`,
			time.Now().UnixNano(), contactID, mustJSON("Command timed out. Please try again."))
		wsWrite(reply)
	}
}

func (b *Bridge) updateContact(link string) {
	if link == "" {
		return
	}
	path := filepath.Join(b.DataDir, "island_contact_link.txt")
	b2, err := os.ReadFile(path)
	current := ""
	if err == nil {
		current = strings.TrimSpace(string(b2))
	}
	if current == link {
		return
	}
	if err := os.WriteFile(path, []byte(strings.TrimSpace(link)+"\n"), 0600); err != nil {
		slog.Error("[bridge] failed to update island contact link", "err", err)
		return
	}
	slog.Info("[bridge] updated island contact link")
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

func (b *Bridge) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			connected := api.BridgeConnected
			reconnects := api.BridgeReconnectCount
			cmdCount := api.BridgeCmdCount
			latency := api.BridgeLastCmdLatency
			score := api.BridgeHealthScore()
			slog.Info("[bridge] heartbeat",
				"connected", connected,
				"reconnects", reconnects,
				"cmd_count", cmdCount,
				"latency", latency.String(),
				"health_score", score["health_score"],
				"status", score["status"],
			)
		}
	}
}
