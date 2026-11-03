package apptransport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"crypto/rand"
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

// TestSealOpenErrorPaths exercises error paths in crypto.go to catch mutations.
func TestSealOpenErrorPaths(t *testing.T) {
	encKey := make([]byte, KeySize)
	if _, err := rand.Read(encKey); err != nil {
		t.Fatal(err)
	}

	// Open with too-short sealed data (crypto.go:100 - remove-error-check#0)
	shortSealed := make([]byte, NonceSize-1)
	if _, err := Open(shortSealed, encKey); err == nil {
		t.Fatal("expected ErrDecryptFailed for short sealed data")
	}

	// Open with valid length but wrong key should still fail decryption
	sealed, err := Seal([]byte("test"), encKey)
	if err != nil {
		t.Fatal(err)
	}
	wrongKey := make([]byte, KeySize)
	if _, err := rand.Read(wrongKey); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(sealed, wrongKey); err == nil {
		t.Fatal("expected ErrDecryptFailed for wrong key")
	}

	// OpenPayload on non-encrypted envelope (crypto.go:154 - remove-error-check#1)
	env := Envelope{Encrypted: false}
	if err := env.OpenPayload(encKey); err != nil {
		t.Fatalf("expected no error for non-encrypted envelope, got %v", err)
	}
}

// TestEnvelopeSignVerifyErrorPaths exercises error paths in envelope.go.
func TestEnvelopeSignVerifyErrorPaths(t *testing.T) {
	// Sign with empty key (envelope.go:30 - validation check)
	env := Envelope{Payload: json.RawMessage(`{}`), Type: "test"}
	if err := env.Sign([]byte{}); err == nil {
		t.Fatal("expected ErrMissingSecret for empty key")
	}

	// Verify with missing ID (envelope.go:57 - validation check)
	env = Envelope{Type: "test", Nonce: "nonce", Signature: "sig", Timestamp: time.Now().Unix()}
	if err := env.Verify([]byte("key"), time.Now(), nil); err == nil {
		t.Fatal("expected ErrMissingID for missing ID")
	}

	// Verify with missing Nonce (envelope.go:60 - validation check)
	env = Envelope{ID: "id", Type: "test", Signature: "sig", Timestamp: time.Now().Unix()}
	if err := env.Verify([]byte("key"), time.Now(), nil); err == nil {
		t.Fatal("expected ErrMissingNonce for missing Nonce")
	}

	// Verify with missing Signature
	env = Envelope{ID: "id", Nonce: "nonce", Type: "test", Timestamp: time.Now().Unix()}
	if err := env.Verify([]byte("key"), time.Now(), nil); err == nil {
		t.Fatal("expected ErrMissingSignature for missing Signature")
	}

	// Verify with missing Timestamp
	env = Envelope{ID: "id", Nonce: "nonce", Signature: "sig", Type: "test"}
	if err := env.Verify([]byte("key"), time.Now(), nil); err == nil {
		t.Fatal("expected ErrMissingTimestamp for missing Timestamp")
	}

	// Verify with invalid base64 signature (envelope.go:80 - base64.DecodeString error check)
	env = Envelope{
		ID:        "id",
		Nonce:     "nonce",
		Signature: "not-valid-base64!",
		Timestamp: time.Now().Unix(),
	}
	if err := env.Verify([]byte("key"), time.Now(), nil); err == nil {
		t.Fatal("expected ErrInvalidSignature for invalid base64 signature")
	}
}

// TestKeyringAdoptErrorPaths exercises error paths in keyring.go Adopt.
func TestKeyringAdoptErrorPaths(t *testing.T) {
	kr := NewKeyring()

	// Adopt with wrong secret length (keyring.go:102 - remove-error-check#1)
	_, err := kr.Adopt("testapp", make([]byte, KeySize-1), nil)
	if err == nil {
		t.Fatal("expected error for wrong secret length")
	}

	// Adopt with empty salt should auto-generate (keyring.go:105 - remove-error-check#2)
	secret := make([]byte, KeySize)
	if _, err := rand.Read(secret); err != nil {
		t.Fatal(err)
	}
	keys, err := kr.Adopt("testapp2", secret, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys.salt) != SaltSize {
		t.Fatalf("expected auto-generated salt of length %d, got %d", SaltSize, len(keys.salt))
	}
}

// TestTransportPublishErrorPaths exercises error paths in transport.go Publish/PublishEnvelope/WriteEnvelope.
func TestTransportPublishErrorPaths(t *testing.T) {
	// Publish with payload too large (transport.go:115 - remove-error-check#0)
	tr, err := New(Config{Secret: []byte("test-secret"), MaxPayloadBytes: 100})
	if err != nil {
		t.Fatal(err)
	}
	largePayload := json.RawMessage(strings.Repeat("x", 200))
	_, err = tr.Publish(context.Background(), PublishRequest{Type: "event", Payload: largePayload})
	if err == nil {
		t.Fatal("expected ErrPayloadTooLarge for oversized payload")
	}

	// PublishEnvelope with full queue (transport.go:122 - remove-error-check#1)
	tr, err = New(Config{Secret: []byte("test-secret"), MaxQueue: 1, MaxPayloadBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	env1, _ := NewEnvelope(PublishRequest{Type: "event", Payload: json.RawMessage(`{"a":1}`)}, []byte("test-secret"))
	if err := tr.PublishEnvelope(context.Background(), env1); err != nil {
		t.Fatal(err)
	}
	env2, _ := NewEnvelope(PublishRequest{Type: "event", Payload: json.RawMessage(`{"b":2}`)}, []byte("test-secret"))
	if err := tr.PublishEnvelope(context.Background(), env2); err == nil {
		t.Fatal("expected ErrQueueFull when queue is full")
	}

	// PublishEnvelope with replay (transport.go:125 - remove-error-check#1 replay check)
	tr, err = New(Config{Secret: []byte("test-secret"), MaxQueue: 10, MaxPayloadBytes: 1024, ReplayWindow: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	env, _ := NewEnvelope(PublishRequest{Type: "event", Payload: json.RawMessage(`{"replay":1}`)}, []byte("test-secret"))
	if err := tr.PublishEnvelope(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	// Same nonce should be rejected by replay check
	if err := tr.PublishEnvelope(context.Background(), env); err == nil {
		t.Fatal("expected ErrReplay for duplicate nonce in PublishEnvelope")
	}

	// WriteEnvelope with frame too large (transport.go:161 - remove-error-check#2)
	tr, err = New(Config{Secret: []byte("test-secret"), MaxPayloadBytes: 50})
	if err != nil {
		t.Fatal(err)
	}
	env, _ = NewEnvelope(PublishRequest{Type: "event", Payload: json.RawMessage(`{"large":"payloadthatwillbetoolargeforfiftybytes"}`)}, []byte("test-secret"))
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	err = tr.WriteEnvelope(context.Background(), client, env)
	if err == nil {
		t.Fatal("expected ErrFrameTooLarge for oversized frame")
	}
}

// TestNormalizeConfigInvalidMode exercises error path in types.go normalizeConfig.
func TestNormalizeConfigInvalidMode(t *testing.T) {
	cfg := Config{Mode: "invalid", Secret: []byte("secret")}
	_, err := normalizeConfig(cfg)
	if err == nil {
		t.Fatal("expected ErrInvalidMode for invalid transport mode")
	}
}

// TestCheckTimestampBoundaries exercises the timestamp boundary check in crypto.go.
func TestCheckTimestampBoundaries(t *testing.T) {
	// Test future timestamp rejection (crypto.go:175 - invert-comparison#0)
	env := Envelope{
		ID:        "id",
		Nonce:     "nonce",
		Signature: "sig",
		Timestamp: time.Now().Unix() + MaxClockSkew + 1, // too far in future
		TTL:       3600,
	}
	if err := env.CheckTimestamp(time.Now().Unix()); err == nil {
		t.Fatal("expected ErrClockSkew for timestamp too far in future")
	}

	// Test expired timestamp rejection
	env = Envelope{
		ID:        "id",
		Nonce:     "nonce",
		Signature: "sig",
		Timestamp: time.Now().Unix() - 7200,
		TTL:       3600, // 1 hour TTL, but timestamp is 2 hours ago
	}
	if err := env.CheckTimestamp(time.Now().Unix()); err == nil {
		t.Fatal("expected ErrExpired for expired envelope")
	}

	// Test valid timestamp (within skew, not expired)
	env = Envelope{
		ID:        "id",
		Nonce:     "nonce",
		Signature: "sig",
		Timestamp: time.Now().Unix() - 100,
		TTL:       3600,
	}
	if err := env.CheckTimestamp(time.Now().Unix()); err != nil {
		t.Fatalf("expected no error for valid timestamp, got %v", err)
	}

	// Test TTL == 0 means no expiry (only future check applies)
	env = Envelope{
		ID:        "id",
		Nonce:     "nonce",
		Signature: "sig",
		Timestamp: time.Now().Unix() - 7200,
		TTL:       0,
	}
	if err := env.CheckTimestamp(time.Now().Unix()); err != nil {
		t.Fatalf("expected no error for TTL=0 (no expiry), got %v", err)
	}
}
