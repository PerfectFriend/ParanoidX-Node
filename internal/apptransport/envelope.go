package apptransport

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

type wireEnvelope struct {
	Version   int             `json:"v"`
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	ChannelID string          `json:"channel_id,omitempty"`
	SessionID string          `json:"session_id,omitempty"`
	Topic     string          `json:"topic,omitempty"`
	Kind      string          `json:"kind,omitempty"`
	Payload    json.RawMessage `json:"payload,omitempty"`
	Ciphertext []byte          `json:"ct,omitempty"`
	Encrypted  bool            `json:"enc,omitempty"`
	Timestamp  int64           `json:"ts"`
	TTL        int64           `json:"ttl,omitempty"`
	Nonce      string          `json:"nonce"`
	Meta       jsonMeta        `json:"meta,omitempty"`
}

func (e *Envelope) Sign(key []byte) error {
	if len(key) == 0 {
		return ErrMissingSecret
	}
	if e.Version == 0 {
		e.Version = Version
	}
	if e.ID == "" {
		e.ID = RandomID()
	}
	if e.Nonce == "" {
		e.Nonce = RandomID()
	}
	if e.Timestamp == 0 {
		e.Timestamp = time.Now().UTC().Unix()
	}
	if e.Payload == nil {
		e.Payload = json.RawMessage(`{}`)
	}
	sig, err := signatureFor(e, key)
	if err != nil {
		return err
	}
	e.Signature = base64.RawURLEncoding.EncodeToString(sig)
	return nil
}

func (e Envelope) Verify(key []byte, now time.Time, replay *ReplayGuard) error {
	if e.ID == "" {
		return ErrMissingID
	}
	if e.Nonce == "" {
		return ErrMissingNonce
	}
	if e.Signature == "" {
		return ErrMissingSignature
	}
	if e.Timestamp == 0 {
		return ErrMissingTimestamp
	}
	// Bound validity in BOTH directions. KiloParanoidX only rejected envelopes
	// that were too old, so TTL == 0 never expired and a far-future timestamp
	// was accepted forever, leaving the replay window unbounded.
	if err := e.CheckTimestamp(now.Unix()); err != nil {
		return err
	}
	expected, err := signatureFor(&e, key)
	if err != nil {
		return err
	}
	actual, err := base64.RawURLEncoding.DecodeString(e.Signature)
	if err != nil {
		return ErrInvalidSignature
	}
	if !hmac.Equal(expected, actual) {
		return ErrInvalidSignature
	}
	if replay != nil {
		if err := replay.Check(e); err != nil {
			return err
		}
	}
	return nil
}

func signatureFor(e *Envelope, key []byte) ([]byte, error) {
	data, err := canonicalEnvelope(e)
	if err != nil {
		return nil, err
	}
	mac := hmac.New(sha256.New, key)
	if _, err := mac.Write(data); err != nil {
		return nil, err
	}
	return mac.Sum(nil), nil
}

func canonicalEnvelope(e *Envelope) ([]byte, error) {
	wire := wireEnvelope{
		Version:   e.Version,
		ID:        e.ID,
		Type:      e.Type,
		ChannelID: e.ChannelID,
		SessionID: e.SessionID,
		Topic:     e.Topic,
		Kind:      e.Kind,
		// Both payload AND ciphertext are signed. Signing only the plaintext
		// would let an attacker swap sealed bytes; signing only the ciphertext
		// would let them swap a plaintext payload in its place.
		Payload:    e.Payload,
		Ciphertext: e.Ciphertext,
		Encrypted:  e.Encrypted,
		Timestamp:  e.Timestamp,
		TTL:        e.TTL,
		Nonce:      e.Nonce,
		Meta:       e.Meta,
	}
	return json.Marshal(wire)
}

func NewEnvelope(req PublishRequest, key []byte) (Envelope, error) {
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
	if err := env.Sign(key); err != nil {
		return Envelope{}, err
	}
	return env, nil
}

func ParseEnvelope(data []byte) (Envelope, error) {
	var env Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return Envelope{}, fmt.Errorf("parse envelope: %w", err)
	}
	return env, nil
}
