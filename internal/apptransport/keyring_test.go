package apptransport

// Tests for per-application key isolation - the property KiloParanoidX
// claimed but did not have (one shared secret, so applications were not
// separated at all).
//
// The card-game scenario: two players' envelopes travel over the same
// transport. Each application must be readable only by itself.

import (
	"bytes"
	"testing"
	"time"
)

func TestKeyringGeneratesIndependentKeysPerApp(t *testing.T) {
	kr := NewKeyring()

	cards, err := kr.Register("cards")
	if err != nil {
		t.Fatalf("Register cards: %v", err)
	}
	backgammon, err := kr.Register("backgammon")
	if err != nil {
		t.Fatalf("Register backgammon: %v", err)
	}

	if bytes.Equal(cards.Secret, backgammon.Secret) {
		t.Fatal("ISOLATION BROKEN: two applications share the same master secret")
	}
	if bytes.Equal(cards.EncKey, backgammon.EncKey) {
		t.Fatal("ISOLATION BROKEN: two applications share the encryption key")
	}
	if bytes.Equal(cards.AuthKey, backgammon.AuthKey) {
		t.Fatal("ISOLATION BROKEN: two applications share the auth key")
	}
	if got := kr.List(); len(got) != 2 || got[0] != "backgammon" || got[1] != "cards" {
		t.Fatalf("List wrong: %v", got)
	}
}

func TestKeyringRefusesDuplicateAndInvalidNames(t *testing.T) {
	kr := NewKeyring()
	if _, err := kr.Register("cards"); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if _, err := kr.Register("cards"); err == nil {
		t.Fatal("duplicate application registered silently - keys would rotate under live peers")
	}
	if _, err := kr.Register(""); err == nil {
		t.Fatal("empty name accepted")
	}
	if _, err := kr.Register("cards/../etc"); err == nil {
		t.Fatal("path traversal in application name accepted")
	}
	if _, err := kr.Get("nope"); err == nil {
		t.Fatal("unknown application lookup succeeded")
	}
}

func TestAppCannotReadAnotherAppsTraffic(t *testing.T) {
	kr := NewKeyring()
	cards, _ := kr.Register("cards")
	backgammon, _ := kr.Register("backgammon")

	secret := []byte(`{"hand":["A","K","Q"],"bet":500}`)
	env, err := cards.NewEnvelopeFor(PublishRequest{
		Type:      "message",
		ChannelID: "game-1",
		Payload:   append([]byte(nil), secret...),
	})
	if err != nil {
		t.Fatalf("NewEnvelopeFor: %v", err)
	}

	// The other application must fail on both verification and decryption.
	if _, err := backgammon.Open(env, time.Now().UTC(), nil); err == nil {
		t.Fatal("ISOLATION BROKEN: backgammon opened cards traffic")
	}
	// And must not be able to read the ciphertext even by direct decrypt.
	if _, err := Open(env.Ciphertext, backgammon.EncKey); err == nil {
		t.Fatal("ISOLATION BROKEN: backgammon decrypted cards ciphertext")
	}

	// The rightful owner must succeed.
	got, err := cards.Open(env, time.Now().UTC(), nil)
	if err != nil {
		t.Fatalf("owner could not open own traffic: %v", err)
	}
	if !bytes.Equal(got, secret) {
		t.Fatalf("payload mismatch: got %s want %s", got, secret)
	}
}

func TestAdoptReconstructsKeysOnSecondPeer(t *testing.T) {
	// Onboarding: peer A registers, exports the public half; peer B adopts the
	// same secret+salt and can then read A's traffic.
	krA := NewKeyring()
	appA, err := krA.Register("cards")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	env, err := appA.NewEnvelopeFor(PublishRequest{
		Type: "message", ChannelID: "g", Payload: []byte(`{"move":"check"}`),
	})
	if err != nil {
		t.Fatalf("NewEnvelopeFor: %v", err)
	}

	krB := NewKeyring()
	appB, err := krB.Adopt("cards", appA.Secret, appA.salt)
	if err != nil {
		t.Fatalf("Adopt: %v", err)
	}
	got, err := appB.Open(env, time.Now().UTC(), nil)
	if err != nil {
		t.Fatalf("adopted peer could not read traffic: %v", err)
	}
	if string(got) != `{"move":"check"}` {
		t.Fatalf("payload mismatch: %s", got)
	}

	// Public export must not leak the secret.
	pub := appA.MarshalPublic()
	for k, v := range pub {
		if k == "secret" {
			t.Fatal("MarshalPublic leaked the secret field")
		}
		if s, ok := v.(string); ok && s == string(appA.Secret) {
			t.Fatalf("MarshalPublic leaked secret bytes under %q", k)
		}
	}
}

func TestAdoptRejectsWrongSecretLength(t *testing.T) {
	kr := NewKeyring()
	if _, err := kr.Adopt("cards", []byte("too-short"), nil); err == nil {
		t.Fatal("wrong-length secret accepted")
	}
}

func TestReplayGuardPerApplicationWindow(t *testing.T) {
	kr := NewKeyring()
	cards, _ := kr.Register("cards")
	guard := NewReplayGuard(time.Minute, time.Now)

	env, err := cards.NewEnvelopeFor(PublishRequest{
		Type: "message", ChannelID: "g", Payload: []byte(`{"n":1}`),
	})
	if err != nil {
		t.Fatalf("NewEnvelopeFor: %v", err)
	}
	// First use is fine.
	if _, err := cards.Open(env, time.Now().UTC(), guard); err != nil {
		t.Fatalf("first delivery rejected: %v", err)
	}
	// Second delivery of the same envelope must be refused.
	if _, err := cards.Open(env, time.Now().UTC(), guard); err == nil {
		t.Fatal("REPLAY ACCEPTED: same envelope delivered twice")
	}
}