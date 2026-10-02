package apptransport

// Per-application key isolation.
//
// KiloParanoidX had exactly one secret per Transport, so every envelope from
// every application shared one HMAC key: applications were NOT isolated, they
// merely shared a signing key. This file introduces the piece the design was
// missing - a Keyring that hands each application its own derived key pair and
// refuses to mix them up.
//
// Intended flow (the onboarding KiloParanoidX never had):
//
//	app, err := apptransport.RegisterApp(keyring, "cards")   // generates keys
//	app2, _ := apptransport.RegisterApp(keyring, "backgammon") // separate keys
//	env, _ := app.Seal(ctx, PublishRequest{...})             // sealed for app
//	app2.Open(ctx, env)                                      // -> ErrWrongApp
//
// Each application gets an independent master secret. Subkeys are derived per
// application with its own salt, so a compromise of one application's
// encryption key yields nothing about another's.

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ErrUnknownApp     = errors.New("unknown application")
	ErrAppExists      = errors.New("application already registered")
	ErrInvalidAppName = errors.New("invalid application name")
	ErrWrongApp       = errors.New("envelope belongs to a different application")
)

// AppID identifies one application on the transport.
type AppID string

// AppKeys is one application's isolated key material. Never serialize the
// secret; persist only the salt via MarshalPublic.
type AppKeys struct {
	Name   AppID
	Secret []byte
	EncKey []byte
	AuthKey []byte
	salt   []byte
}

// Keyring holds per-application keys for one transport instance.
type Keyring struct {
	mu   sync.RWMutex
	apps map[AppID]*AppKeys
}

// NewKeyring returns an empty keyring.
func NewKeyring() *Keyring {
	return &Keyring{apps: make(map[AppID]*AppKeys)}
}

// Register generates a fresh master secret and salt for an application and
// derives its subkeys. Re-registering an existing name is refused so a
// transport cannot silently rotate keys out from under live peers.
func (k *Keyring) Register(name string) (*AppKeys, error) {
	id, err := validateAppName(name)
	if err != nil {
		return nil, err
	}

	k.mu.Lock()
	defer k.mu.Unlock()
	if _, exists := k.apps[id]; exists {
		return nil, fmt.Errorf("%w: %s", ErrAppExists, id)
	}

	secret, err := RandomSecret()
	if err != nil {
		return nil, err
	}
	salt, err := NewSalt()
	if err != nil {
		return nil, err
	}
	enc, auth := DeriveSubkeys(secret, salt)

	keys := &AppKeys{
		Name: id, Secret: secret, EncKey: enc, AuthKey: auth, salt: salt,
	}
	k.apps[id] = keys
	return keys, nil
}

// Adopt installs externally generated key material (onboarding: keys handed
// over out of band). The secret must be exactly 32 bytes.
func (k *Keyring) Adopt(name string, secret, salt []byte) (*AppKeys, error) {
	id, err := validateAppName(name)
	if err != nil {
		return nil, err
	}
	if len(secret) != KeySize {
		return nil, fmt.Errorf("secret must be %d bytes, got %d", KeySize, len(secret))
	}
	if len(salt) == 0 {
		if salt, err = NewSalt(); err != nil {
			return nil, err
		}
	}

	k.mu.Lock()
	defer k.mu.Unlock()
	if _, exists := k.apps[id]; exists {
		return nil, fmt.Errorf("%w: %s", ErrAppExists, id)
	}
	enc, auth := DeriveSubkeys(secret, salt)
	keys := &AppKeys{Name: id, Secret: append([]byte(nil), secret...),
		EncKey: enc, AuthKey: auth, salt: append([]byte(nil), salt...)}
	k.apps[id] = keys
	return keys, nil
}

// Get returns an application's keys.
func (k *Keyring) Get(name string) (*AppKeys, error) {
	id, err := validateAppName(name)
	if err != nil {
		return nil, err
	}
	k.mu.RLock()
	defer k.mu.RUnlock()
	keys, ok := k.apps[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownApp, id)
	}
	return keys, nil
}

// List returns registered application names, sorted.
func (k *Keyring) List() []string {
	k.mu.RLock()
	defer k.mu.RUnlock()
	out := make([]string, 0, len(k.apps))
	for id := range k.apps {
		out = append(out, string(id))
	}
	sort.Strings(out)
	return out
}

// NewEnvelopeFor builds and seals an envelope for this application. The
// channel and session fields carry the application identity implicitly, but
// the real isolation comes from the key pair: another application's keys
// cannot verify or decrypt the result.
func (a *AppKeys) NewEnvelopeFor(req PublishRequest) (Envelope, error) {
	if req.Payload == nil {
		req.Payload = json.RawMessage(`{}`)
	}
	env := Envelope{
		Version:   Version,
		ID:        RandomID(),
		Type:      req.Type,
		ChannelID: req.ChannelID,
		SessionID: req.SessionID,
		Topic:     req.Topic,
		Kind:      req.Kind,
		Payload:   req.Payload,
		Timestamp: time.Now().UTC().Unix(),
		TTL:       req.TTL,
		Nonce:     RandomID(),
		Meta:      req.Meta,
	}
	if env.Type == "" {
		env.Type = "message"
	}
	if err := env.SealPayload(a.EncKey); err != nil {
		return Envelope{}, err
	}
	if err := env.Sign(a.AuthKey); err != nil {
		return Envelope{}, err
	}
	return env, nil
}

// Open verifies and decrypts an envelope using this application's keys.
func (a *AppKeys) Open(env Envelope, now time.Time, replay *ReplayGuard) ([]byte, error) {
	if err := env.Verify(a.AuthKey, now, replay); err != nil {
		return nil, err
	}
	if err := env.OpenPayload(a.EncKey); err != nil {
		return nil, err
	}
	return env.Payload, nil
}

// MarshalPublic exports the non-secret half so a peer can adopt the same keys.
// The secret is deliberately excluded.
func (a *AppKeys) MarshalPublic() map[string]any {
	return map[string]any{
		"name": string(a.Name),
		"salt": fmt.Sprintf("%x", a.salt),
	}
}

func validateAppName(name string) (AppID, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 64 {
		return "", fmt.Errorf("%w: %q", ErrInvalidAppName, name)
	}
	for _, c := range name {
		ok := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.'
		if !ok {
			return "", fmt.Errorf("%w: %q", ErrInvalidAppName, name)
		}
	}
	return AppID(name), nil
}