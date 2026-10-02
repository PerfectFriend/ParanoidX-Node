package apptransport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestEnvelopeSignVerifyRejectsTamper(t *testing.T) {
	key := []byte("test-secret")
	payload := json.RawMessage(`{"game":"state","tick":1}`)
	env, err := NewEnvelope(PublishRequest{Type: "event", ChannelID: "room-1", Kind: "state", Payload: payload}, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Verify(key, time.Now().UTC(), nil); err != nil {
		t.Fatal(err)
	}
	env.Payload = json.RawMessage(`{"game":"state","tick":2}`)
	if err := env.Verify(key, time.Now().UTC(), nil); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("expected invalid signature, got %v", err)
	}
}

func TestReplayGuardRejectsDuplicateNonce(t *testing.T) {
	now := time.Unix(100, 0)
	guard := NewReplayGuard(time.Minute, func() time.Time { return now })
	env := Envelope{ID: "1", Nonce: "nonce-1"}
	if err := guard.Check(env); err != nil {
		t.Fatal(err)
	}
	if err := guard.Check(env); !errors.Is(err, ErrReplay) {
		t.Fatalf("expected replay error, got %v", err)
	}
	now = now.Add(2 * time.Minute)
	if err := guard.Check(env); err != nil {
		t.Fatalf("expected nonce to expire, got %v", err)
	}
}

func TestQueueHistoryAndBroadcast(t *testing.T) {
	q := NewQueue(2)
	if _, err := q.Enqueue(Envelope{ID: "1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue(Envelope{ID: "2"}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue(Envelope{ID: "3"}); err != nil {
		t.Fatal(err)
	}
	history := q.History(10)
	if len(history) != 2 || history[0].Envelope.ID != "2" || history[1].Envelope.ID != "3" {
		t.Fatalf("unexpected history: %#v", history)
	}
	ch, unsubscribe := q.Subscribe()
	defer unsubscribe()
	if _, err := q.Enqueue(Envelope{ID: "4"}); err != nil {
		t.Fatal(err)
	}
	select {
	case msg := <-ch:
		if msg.Envelope.ID != "4" {
			t.Fatalf("unexpected message id %q", msg.Envelope.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for queued message")
	}
}

func TestPublishHandler(t *testing.T) {
	tr, err := New(Config{Secret: []byte("test-secret"), MaxQueue: 4, MaxPayloadBytes: 1024, ReplayWindow: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	body := bytes.NewBufferString(`{"type":"rpc.request","channel_id":"ai","kind":"prompt","payload":{"text":"hello"}}`)
	req := httptest.NewRequest(http.MethodPost, "/publish", body)
	rec := httptest.NewRecorder()
	tr.PublishHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var env Envelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatal(err)
	}
	if err := env.Verify([]byte("test-secret"), time.Now().UTC(), nil); err != nil {
		t.Fatal(err)
	}
	if env.ChannelID != "ai" || env.Kind != "prompt" {
		t.Fatalf("unexpected envelope: %#v", env)
	}
}

func TestSignalHandler(t *testing.T) {
	tr, err := New(Config{Secret: []byte("test-secret")})
	if err != nil {
		t.Fatal(err)
	}
	body := bytes.NewBufferString(`{"offer":"sdp"}`)
	post := httptest.NewRequest(http.MethodPost, "/signal?room=call-1", body)
	postRec := httptest.NewRecorder()
	tr.SignalHandler(postRec, post)
	if postRec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", postRec.Code, postRec.Body.String())
	}
	get := httptest.NewRequest(http.MethodGet, "/signal?room=call-1", nil)
	getRec := httptest.NewRecorder()
	tr.SignalHandler(getRec, get)
	if getRec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", getRec.Code, getRec.Body.String())
	}
	var state map[string]any
	if err := json.NewDecoder(getRec.Body).Decode(&state); err != nil {
		t.Fatal(err)
	}
	if state["offer"] != "sdp" {
		t.Fatalf("unexpected state: %#v", state)
	}
}

func TestPublishRejectsFullQueue(t *testing.T) {
	tr, err := New(Config{Secret: []byte("test-secret"), MaxQueue: 1, MaxPayloadBytes: 1024, ReplayWindow: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Publish(context.Background(), PublishRequest{Type: "event", Payload: json.RawMessage(`{"first":true}`)}); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Publish(context.Background(), PublishRequest{Type: "event", Payload: json.RawMessage(`{"second":true}`)}); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("expected queue full, got %v", err)
	}
}

func TestReplayForgetRollback(t *testing.T) {
	tr, err := New(Config{Secret: []byte("test-secret"), MaxQueue: 1, MaxPayloadBytes: 1024, ReplayWindow: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	env, err := NewEnvelope(PublishRequest{Type: "event", Payload: json.RawMessage(`{"second":true}`)}, []byte("test-secret"))
	if err != nil {
		t.Fatal(err)
	}
	if err := tr.ReplayGuard().Check(env); err != nil {
		t.Fatal(err)
	}
	tr.ReplayGuard().Forget(env.Nonce)
	if err := tr.ReplayGuard().Check(env); err != nil {
		t.Fatalf("expected nonce rollback, got %v", err)
	}
}

func TestWriteReadEnvelope(t *testing.T) {
	tr, err := New(Config{Secret: []byte("test-secret"), MaxPayloadBytes: 1024, WriteTimeout: time.Second, ReadTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	env, err := NewEnvelope(PublishRequest{Type: "event", Payload: json.RawMessage(`{"tick":1}`)}, []byte("test-secret"))
	if err != nil {
		t.Fatal(err)
	}
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	errCh := make(chan error, 1)
	go func() {
		errCh <- tr.WriteEnvelope(context.Background(), client, env)
	}()
	got, err := tr.ReadEnvelope(server)
	if err != nil {
		t.Fatal(err)
	}
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
	if got.ID != env.ID || got.Signature != env.Signature {
		t.Fatalf("unexpected envelope: %#v", got)
	}
}
