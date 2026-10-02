package apptransport

// Tests for the defects found reviewing KiloParanoidX, plus the isolation
// property that makes this usable as an application transport:
//
//   - payload must be encrypted, not merely signed
//   - two applications must NOT be able to read each other's payloads
//   - a signature must not survive payload substitution
//   - a valid signature must not be replayable into another channel
//   - timestamps must be bounded in BOTH directions (KiloParanoidX bug)
//
// Every test below is paired with a negative assertion: it proves the guard
// actually fires. A test that only asserts the happy path proves nothing.

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"
)

func mustMaster(t *testing.T) ([]byte, []byte) {
	t.Helper()
	master, err := RandomSecret()
	if err != nil {
		t.Fatalf("RandomSecret: %v", err)
	}
	salt, err := NewSalt()
	if err != nil {
		t.Fatalf("NewSalt: %v", err)
	}
	enc, auth := DeriveSubkeys(master, salt)
	return enc, auth
}

func TestPayloadIsEncryptedNotPlaintext(t *testing.T) {
	enc, auth := mustMaster(t)
	secret := []byte(`{"move":"ace of spades","private":true}`)

	env := Envelope{
		ID: "env-1", Type: "message", ChannelID: "ch-1",
		Payload:   append([]byte(nil), secret...),
		Timestamp: time.Now().UTC().Unix(),
	}
	if err := env.SealPayload(enc); err != nil {
		t.Fatalf("SealPayload: %v", err)
	}
	if !env.Encrypted {
		t.Fatal("Encrypted flag not set after SealPayload")
	}
	// The plaintext must NOT be visible anywhere in the sealed envelope.
	if bytes.Contains(env.Ciphertext, []byte("ace of spades")) {
		t.Fatal("PAYLOAD LEAKED: plaintext found in ciphertext")
	}
	if bytes.Contains(env.Ciphertext, []byte("private")) {
		t.Fatal("PAYLOAD LEAKED: field name visible")
	}
	if len(env.Payload) != 0 {
		t.Fatal("plaintext payload field not cleared after sealing")
	}

	// Signature must cover the ciphertext and verify.
	if err := env.Sign(auth); err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if err := env.Verify(auth, time.Now().UTC(), nil); err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	if err := env.OpenPayload(enc); err != nil {
		t.Fatalf("OpenPayload: %v", err)
	}
	if !bytes.Equal(env.Payload, secret) {
		t.Fatalf("roundtrip mismatch: got %s want %s", env.Payload, secret)
	}
	if len(env.Ciphertext) != 0 {
		t.Fatal("ciphertext field not cleared after opening")
	}
}

func TestApplicationIsolationDifferentKeysCannotRead(t *testing.T) {
	// The whole point of per-app keys: app A's transport must be useless
	// against app B's traffic.
	encA, authA := mustMaster(t)
	_, authB := mustMaster(t)

	secret := []byte(`{"cards":["K","Q"],"hand":"secret"}`)
	envA := Envelope{
		ID: "env-a", Type: "message", ChannelID: "game-1",
		Payload:   append([]byte(nil), secret...),
		Timestamp: time.Now().UTC().Unix(),
	}
	if err := envA.SealPayload(encA); err != nil {
		t.Fatalf("SealPayload: %v", err)
	}
	if err := envA.Sign(authA); err != nil {
		t.Fatalf("Sign: %v", err)
	}

	// App B with its own key must not even be able to verify the signature.
	if err := envA.Verify(authB, time.Now().UTC(), nil); err == nil {
		t.Fatal("ISOLATION BROKEN: app B verified app A's envelope")
	}
	// And must not be able to decrypt.
	if err := envA.OpenPayload(mustOtherEnc(t)); err == nil {
		t.Fatal("ISOLATION BROKEN: app B decrypted app A's payload")
	}
}

func mustOtherEnc(t *testing.T) []byte {
	t.Helper()
	enc, _ := mustMaster(t)
	return enc
}

func TestSignatureSurvivesPayloadSubstitution(t *testing.T) {
	enc, auth := mustMaster(t)
	env := Envelope{
		ID: "env-s", Type: "message", ChannelID: "ch",
		Payload:   []byte(`{"amount":10}`),
		Timestamp: time.Now().UTC().Unix(),
	}
	if err := env.SealPayload(enc); err != nil {
		t.Fatalf("SealPayload: %v", err)
	}
	if err := env.Sign(auth); err != nil {
		t.Fatalf("Sign: %v", err)
	}

	// Attacker swaps the ciphertext for their own sealed payload.
	other := Envelope{Payload: []byte(`{"amount":99999}`)}
	if err := other.SealPayload(enc); err != nil {
		t.Fatalf("SealPayload: %v", err)
	}
	env.Ciphertext = other.Ciphertext

	if err := env.Verify(auth, time.Now().UTC(), nil); err == nil {
		t.Fatal("FORGERY SUCCEEDED: substituted payload passed verification")
	}
}

func TestReplayIntoDifferentChannelRejected(t *testing.T) {
	enc, auth := mustMaster(t)
	env := Envelope{
		ID: "env-c", Type: "message", ChannelID: "channel-a",
		Payload:   []byte(`{"move":"bet"}`),
		Timestamp: time.Now().UTC().Unix(),
	}
	if err := env.SealPayload(enc); err != nil {
		t.Fatalf("SealPayload: %v", err)
	}
	if err := env.Sign(auth); err != nil {
		t.Fatalf("Sign: %v", err)
	}
	// Happy path first: the original must verify.
	if err := env.Verify(auth, time.Now().UTC(), nil); err != nil {
		t.Fatalf("original should verify: %v", err)
	}
	// Move it to another channel: signature must break, since channel_id is
	// part of the canonical form.
	moved := env
	moved.ChannelID = "channel-b"
	if err := moved.Verify(auth, time.Now().UTC(), nil); err == nil {
		t.Fatal("CROSS-CHANNEL REPLAY SUCCEEDED: envelope verified in another channel")
	}
}

func TestTimestampBoundedBothDirections(t *testing.T) {
	now := time.Now().UTC().Unix()

	// Far-future timestamp must be rejected. KiloParanoidX accepted this
	// forever because it only checked expiry.
	future := &Envelope{Timestamp: now + 86400}
	if err := future.CheckTimestamp(now); err == nil {
		t.Fatal("FUTURE TIMESTAMP ACCEPTED: unbounded validity window")
	}

	// TTL == 0 must not mean "valid forever" when the timestamp is skewed.
	skewed := &Envelope{Timestamp: now + MaxClockSkew + 10}
	if err := skewed.CheckTimestamp(now); err == nil {
		t.Fatal("SKEWED NO-TTL ENVELOPE ACCEPTED")
	}

	// Expired must still be rejected.
	expired := &Envelope{Timestamp: now - 100, TTL: 10}
	if err := expired.CheckTimestamp(now); err == nil {
		t.Fatal("EXPIRED ENVELOPE ACCEPTED")
	}

	// A normal envelope must pass.
	ok := &Envelope{Timestamp: now, TTL: 60}
	if err := ok.CheckTimestamp(now); err != nil {
		t.Fatalf("valid envelope rejected: %v", err)
	}
}

func TestSubkeysAreIndependent(t *testing.T) {
	enc, auth := mustMaster(t)
	if bytes.Equal(enc, auth) {
		t.Fatal("SUBKEY COLLISION: encryption and auth keys are identical")
	}
	if len(enc) != KeySize || len(auth) != KeySize {
		t.Fatalf("bad subkey sizes: enc=%d auth=%d", len(enc), len(auth))
	}
}

func TestWireFormatRoundTripKeepsEncryptionFlag(t *testing.T) {
	enc, auth := mustMaster(t)
	env := Envelope{
		ID: "env-w", Type: "event", ChannelID: "c", SessionID: "s",
		Payload:   []byte(`{"x":1}`),
		Timestamp: time.Now().UTC().Unix(),
	}
	if err := env.SealPayload(enc); err != nil {
		t.Fatalf("SealPayload: %v", err)
	}
	if err := env.Sign(auth); err != nil {
		t.Fatalf("Sign: %v", err)
	}

	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	parsed, err := ParseEnvelope(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !parsed.Encrypted {
		t.Fatal("encryption flag lost in transit")
	}
	if err := parsed.Verify(auth, time.Now().UTC(), nil); err != nil {
		t.Fatalf("parsed envelope failed verification: %v", err)
	}
	if err := parsed.OpenPayload(enc); err != nil {
		t.Fatalf("OpenPayload after parse: %v", err)
	}
	if string(parsed.Payload) != `{"x":1}` {
		t.Fatalf("payload mismatch: %s", parsed.Payload)
	}
}