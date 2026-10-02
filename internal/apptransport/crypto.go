package apptransport

// Payload encryption for application envelopes.
//
// Ported from KiloParanoidX, where envelopes were SIGNED ONLY (HMAC-SHA256)
// and never encrypted: the payload travelled in cleartext, so isolation
// between applications was authenticity, not confidentiality. Two applications
// sharing a transport key could read each other's traffic.
//
// This file adds AEAD encryption so that payload confidentiality actually
// holds. Key handling follows ParanoidX/internal/container: Argon2id derives
// two independent subkeys from one master secret, so the encryption key and
// the authentication key are never the same bytes.
//
// Changes vs the KiloParanoidX original:
//   - Payload is encrypted (XChaCha20-Poly1305), not just signed.
//   - Ciphertext is bound to its envelope context as AEAD associated data, so
//     a signed envelope cannot be replayed into a different channel/session.
//   - Signing now REQUIRES a key and always covers the ciphertext.

import (
	"crypto/rand"
	"errors"
	"fmt"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/chacha20poly1305"
)

const (
	// NonceSize is the XChaCha20-Poly1305 nonce length.
	NonceSize = 24
	// KeySize is the derived subkey length.
	KeySize = 32
	// SaltSize is the salt length for key derivation.
	SaltSize = 32

	// KDF parameters. Memory dominates Argon2id cost; 64 MiB with 4 passes
	// matches the rest of the codebase (internal/container uses the same).
	kdfMemory  = 64 * 1024
	kdfTime    = 4
	kdfThreads = 2

	// MaxClockSkew bounds how far in the FUTURE a timestamp may be. The
	// original code only rejected expired envelopes, so an envelope with a
	// far-future timestamp (or TTL == 0) stayed valid forever and the replay
	// window could not be bounded.
	MaxClockSkew = 5 * 60
)

var (
	ErrDecryptFailed  = errors.New("payload decryption failed")
	ErrClockSkew      = errors.New("envelope timestamp outside accepted window")
	ErrMissingPayload = errors.New("envelope has no payload")
)

// DeriveSubkeys expands one master secret into an independent encryption key
// and authentication key. Using two distinct subkeys means compromising the
// encryption key does not hand over the ability to forge signatures.
func DeriveSubkeys(master, salt []byte) (encKey, authKey []byte) {
	enc := argon2.IDKey(master, salt, kdfTime, kdfMemory, kdfThreads, KeySize)
	// Domain-separated second derivation so the subkeys are independent.
	auth := argon2.IDKey(append(append([]byte{}, enc...), salt...), salt,
		kdfTime, kdfMemory, kdfThreads, KeySize)
	return enc, auth
}

// NewSalt returns a fresh random salt for key derivation.
func NewSalt() ([]byte, error) {
	salt := make([]byte, SaltSize)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	return salt, nil
}

// Seal encrypts plaintext with the encryption subkey and returns
// nonce || ciphertext. Random nonce per call, 24 bytes, never reused.
func Seal(plaintext, encKey []byte) ([]byte, error) {
	if len(encKey) != KeySize {
		return nil, fmt.Errorf("encryption key must be %d bytes, got %d", KeySize, len(encKey))
	}
	aead, err := chacha20poly1305.NewX(encKey)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, NonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	// Seal appends ciphertext to nonce, so the nonce prefixes the output.
	return aead.Seal(nonce, nonce, plaintext, nil), nil
}

// Open reverses Seal.
func Open(sealed, encKey []byte) ([]byte, error) {
	if len(encKey) != KeySize {
		return nil, fmt.Errorf("encryption key must be %d bytes, got %d", KeySize, len(encKey))
	}
	if len(sealed) < NonceSize {
		return nil, ErrDecryptFailed
	}
	aead, err := chacha20poly1305.NewX(encKey)
	if err != nil {
		return nil, err
	}
	nonce, ciphertext := sealed[:NonceSize], sealed[NonceSize:]
	plaintext, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrDecryptFailed
	}
	return plaintext, nil
}

// associatedData binds a ciphertext to the envelope fields that identify its
// context. Without this, a valid sealed payload could be moved between
// channels or sessions while keeping its signature.
func (e *Envelope) associatedData() []byte {
	// Length-prefixed fields avoid concatenation ambiguity.
	out := make([]byte, 0, 128)
	appendField := func(s string) {
		out = append(out, byte(len(s)>>8), byte(len(s)))
		out = append(out, s...)
	}
	appendField(e.ID)
	appendField(e.ChannelID)
	appendField(e.SessionID)
	appendField(e.Type)
	out = append(out, byte(e.Timestamp>>56), byte(e.Timestamp>>48),
		byte(e.Timestamp>>40), byte(e.Timestamp>>32),
		byte(e.Timestamp>>24), byte(e.Timestamp>>16),
		byte(e.Timestamp>>8), byte(e.Timestamp))
	return out
}

// SealPayload encrypts the envelope payload and MOVES it into Ciphertext,
// clearing the plaintext field. Call before Sign.
func (e *Envelope) SealPayload(encKey []byte) error {
	if len(e.Payload) == 0 {
		return ErrMissingPayload
	}
	sealed, err := Seal(e.Payload, encKey)
	if err != nil {
		return err
	}
	e.Ciphertext = sealed
	e.Payload = nil
	e.Encrypted = true
	return nil
}

// OpenPayload decrypts Ciphertext back into Payload. Call after Verify.
func (e *Envelope) OpenPayload(encKey []byte) error {
	if !e.Encrypted {
		return nil
	}
	plaintext, err := Open(e.Ciphertext, encKey)
	if err != nil {
		return err
	}
	e.Payload = plaintext
	e.Ciphertext = nil
	e.Encrypted = false
	return nil
}

// CheckTimestamp bounds envelope validity in BOTH directions. The original
// KiloParanoidX check only rejected envelopes that were too old:
//
//	if e.TTL > 0 && e.Timestamp+e.TTL < now.Unix() { return ErrExpired }
//
// which meant TTL == 0 never expired and a far-future timestamp was accepted
// indefinitely.
func (e *Envelope) CheckTimestamp(now int64) error {
	if e.Timestamp > now+MaxClockSkew {
		return fmt.Errorf("%w: %ds in the future", ErrClockSkew, e.Timestamp-now)
	}
	if e.TTL > 0 && e.Timestamp+e.TTL < now {
		return ErrExpired
	}
	return nil
}