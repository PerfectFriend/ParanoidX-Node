// Package api provides HTTP handlers and API endpoints for the simplex-node server
package api

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)


// TestChatHubMessageLifecycle handles the TestChatHubMessageLifecycle HTTP request.
func TestChatHubMessageLifecycle(t *testing.T) {
	hub := NewChatHub()

	// Add message
	msg := ChatMessage{
		ID:        "msg-test-1",
		From:      "alice",
		Text:      "hello world",
		Timestamp: "2026-01-01T00:00:00Z",
		IsUser:    true,
		ChatID:    "@1",
		Status:    StatusSending,
	}
	hub.AddMessage(msg)

	msgs := hub.GetMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Text != "hello world" {
		t.Fatalf("expected 'hello world', got '%s'", msgs[0].Text)
	}

	// Edit message
	if !hub.EditMessageText("msg-test-1", "edited text") {
		t.Fatal("edit failed")
	}
	msgs = hub.GetMessages()
	if msgs[0].Text != "edited text" {
		t.Fatalf("expected 'edited text', got '%s'", msgs[0].Text)
	}

	// Delete message
	hub.DeleteMessage("msg-test-1")
	msgs = hub.GetMessages()
	if len(msgs) != 0 {
		t.Fatalf("expected 0 messages after delete, got %d", len(msgs))
	}

	hub.ClearMessages()
}


// TestChatHubFilePersistence handles the TestChatHubFilePersistence HTTP request.
func TestChatHubFilePersistence(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/chat_test.json"

	hub := NewChatHub()
	hub.WithFile(path)

	hub.AddMessage(ChatMessage{
		ID:   "msg-persist-1",
		From: "bob",
		Text: "persist test",
	})

	// Create new hub reading same file
	hub2 := NewChatHub()
	hub2.WithFile(path)

	msgs := hub2.GetMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 persisted message, got %d", len(msgs))
	}
	if msgs[0].Text != "persist test" {
		t.Fatalf("expected 'persist test', got '%s'", msgs[0].Text)
	}
}


// TestChatSendHandlerValidation handles the TestChatSendHandlerValidation HTTP request.
func TestChatSendHandlerValidation(t *testing.T) {
	hub := NewChatHub()
	handler := ChatSendHandler(hub)

	// Test with empty body
	req := httptest.NewRequest("POST", "/api/chat/send", nil)
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != 400 {
		t.Fatalf("expected 400 for empty body, got %d", w.Code)
	}

	// Test with missing text
	body := `{"contact_id": 1, "chat_id": "@1"}`
	req = httptest.NewRequest("POST", "/api/chat/send", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	handler(w, req)
	if w.Code != 400 {
		t.Fatalf("expected 400 for missing text, got %d", w.Code)
	}
}


// TestChatEditHandler handles the TestChatEditHandler HTTP request.
func TestChatEditHandler(t *testing.T) {
	hub := NewChatHub()
	hub.AddMessage(ChatMessage{
		ID:   "msg-edit-test",
		From: "admin",
		Text: "original",
	})

	handler := ChatEditHandler(hub)

	body := `{"id": "msg-edit-test", "text": "updated"}`
	req := httptest.NewRequest("POST", "/api/chat/edit", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler(w, req)

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["ok"] != true {
		t.Fatalf("expected ok=true, got %v", resp)
	}

	msgs := hub.GetMessages()
	if msgs[0].Text != "updated" {
		t.Fatalf("expected 'updated', got '%s'", msgs[0].Text)
	}
}


// TestChatDeleteHandler handles the TestChatDeleteHandler HTTP request.
func TestChatDeleteHandler(t *testing.T) {
	hub := NewChatHub()
	hub.AddMessage(ChatMessage{
		ID:   "msg-del-test",
		From: "admin",
		Text: "delete me",
	})

	handler := ChatDeleteHandler(hub)

	body := `{"id": "msg-del-test"}`
	req := httptest.NewRequest("POST", "/api/chat/delete", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler(w, req)

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["ok"] != true {
		t.Fatalf("expected ok=true, got %v", resp)
	}

	msgs := hub.GetMessages()
	if len(msgs) != 0 {
		t.Fatalf("expected 0 messages, got %d", len(msgs))
	}
}


// TestChatClearHandler handles the TestChatClearHandler HTTP request.
func TestChatClearHandler(t *testing.T) {
	hub := NewChatHub()
	hub.AddMessage(ChatMessage{ID: "msg-clear-1", Text: "test"})
	hub.AddMessage(ChatMessage{ID: "msg-clear-2", Text: "test2"})

	handler := ChatClearHandler(hub)
	req := httptest.NewRequest("POST", "/api/chat/clear", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["ok"] != true {
		t.Fatalf("expected ok=true, got %v", resp)
	}

	msgs := hub.GetMessages()
	if len(msgs) != 0 {
		t.Fatalf("expected 0 messages after clear, got %d", len(msgs))
	}
}


// TestChatHistoryHandler handles the TestChatHistoryHandler HTTP request.
func TestChatHistoryHandler(t *testing.T) {
	hub := NewChatHub()
	hub.AddMessage(ChatMessage{ID: "msg-hist-1", Text: "history test", ChatID: "@1"})

	handler := ChatHistoryHandler(hub)
	req := httptest.NewRequest("GET", "/api/chat/history?chat_id=@1", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	msgs, ok := resp["messages"].([]any)
	if !ok || len(msgs) == 0 {
		t.Fatalf("expected messages array, got %v", resp)
	}
}


// TestChatStatsHandler handles the TestChatStatsHandler HTTP request.
func TestChatStatsHandler(t *testing.T) {
	hub := NewChatHub()
	hub.AddMessage(ChatMessage{ID: "msg-stats-1", Text: "test", ChatID: "@1"})
	hub.AddMessage(ChatMessage{ID: "msg-stats-2", Text: "test2", ChatID: "@2"})

	handler := ChatStatsHandler(hub)
	req := httptest.NewRequest("GET", "/api/chat/stats", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["ok"] != true {
		t.Fatalf("expected ok=true, got %v", resp)
	}
}


// TestChatPinHandler handles the TestChatPinHandler HTTP request.
func TestChatPinHandler(t *testing.T) {
	hub := NewChatHub()
	hub.AddMessage(ChatMessage{ID: "msg-pin-1", Text: "pin test", ChatID: "@1"})

	handler := ChatPinHandler(hub)
	body := `{"id": "msg-pin-1"}`
	req := httptest.NewRequest("POST", "/api/chat/pin", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler(w, req)

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["ok"] != true {
		t.Fatalf("expected ok=true, got %v", resp)
	}

	msgs := hub.GetMessages()
	if !msgs[0].Pinned {
		t.Fatal("expected message to be pinned")
	}
}


// TestChatReactHandler handles the TestChatReactHandler HTTP request.
func TestChatReactHandler(t *testing.T) {
	hub := NewChatHub()
	hub.AddMessage(ChatMessage{ID: "msg-react-1", Text: "react test", ChatID: "@1"})

	handler := ChatReactHandler(hub)
	body := `{"id": "msg-react-1", "emoji": "like", "user": "alice"}`
	req := httptest.NewRequest("POST", "/api/chat/react", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler(w, req)

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["ok"] != true {
		t.Fatalf("expected ok=true, got %v", resp)
	}

	msgs := hub.GetMessages()
	if len(msgs[0].Reactions) == 0 {
		t.Fatal("expected reactions")
	}
}


// TestChatContactInfoHandler handles the TestChatContactInfoHandler HTTP request.
func TestChatContactInfoHandler(t *testing.T) {
	hub := NewChatHub()
	hub.AddMessage(ChatMessage{ID: "ci-1", Text: "hello", ChatID: "@1", Timestamp: "2026-07-01T00:00:00Z"})
	hub.AddMessage(ChatMessage{ID: "ci-2", Text: "world", ChatID: "@1", Timestamp: "2026-07-02T00:00:00Z"})

	handler := ChatContactInfoHandler(hub)
	req := httptest.NewRequest("GET", "/api/chat/contact/info?id=@1", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["ok"] != true {
		t.Fatalf("expected ok=true, got %v", resp)
	}
	if resp["count"] != float64(2) {
		t.Fatalf("expected count=2, got %v", resp["count"])
	}
}


// TestChatArchiveLifecycle handles the TestChatArchiveLifecycle HTTP request.
func TestChatArchiveLifecycle(t *testing.T) {
	dir := t.TempDir()
	hub := NewChatHub()
	hub.filePath = dir + "/chat_history.json"

	// Timestamps are derived from time.Now(), not hard-coded. Hard-coded dates
	// made this test rot: "recentTime" was written as 2026-07-01 when the test
	// passed, but 90 days of that window have since elapsed, so the "recent"
	// message crossed the cutoff and began failing (expected 1 archived,
	// got 2). Relative ages keep the intent — one old, one recent — forever.
	now := time.Now()
	oldTime := now.AddDate(0, 0, -200).Format(time.RFC3339)
	recentTime := now.AddDate(0, 0, -10).Format(time.RFC3339)

	hub.AddMessage(ChatMessage{ID: "old-1", Text: "old message", Timestamp: oldTime})
	hub.AddMessage(ChatMessage{ID: "recent-1", Text: "recent message", Timestamp: recentTime})

	archived, err := hub.ArchiveOldMessages(dir, 90)
	if err != nil {
		t.Fatalf("archive error: %v", err)
	}
	if archived != 1 {
		t.Fatalf("expected 1 archived message, got %d (old=%s recent=%s now=%s)",
			archived, oldTime, recentTime, now.Format(time.RFC3339))
	}

	// Check placeholder
	msgs := hub.GetMessages()
	foundPlaceholder := false
	for _, m := range msgs {
		if m.ID == "old-1" && m.Text == "[archived]" {
			foundPlaceholder = true
		}
	}
	if !foundPlaceholder {
		t.Fatal("expected placeholder for archived message")
	}

	// List archives
	archives, err := hub.ListArchives(dir)
	if err != nil {
		t.Fatalf("list archives error: %v", err)
	}
	if len(archives) != 1 {
		t.Fatalf("expected 1 archive file, got %d", len(archives))
	}

	// Restore
	restored, err := hub.RestoreArchive(dir, archives[0])
	if err != nil {
		t.Fatalf("restore error: %v", err)
	}
	if restored < 1 {
		t.Fatalf("expected restored messages, got %d", restored)
	}
}
