package apptransport

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"
)

const (
	Version = 1
)

var (
	ErrInvalidMode       = errors.New("invalid transport mode")
	ErrQueueFull         = errors.New("queue full")
	ErrMissingSecret     = errors.New("missing signing secret")
	ErrMissingID         = errors.New("missing envelope id")
	ErrMissingNonce      = errors.New("missing envelope nonce")
	ErrMissingSignature  = errors.New("missing envelope signature")
	ErrMissingTimestamp  = errors.New("missing envelope timestamp")
	ErrExpired           = errors.New("envelope expired")
	ErrInvalidSignature  = errors.New("invalid envelope signature")
	ErrReplay            = errors.New("replayed envelope")
	ErrPayloadTooLarge   = errors.New("payload too large")
	ErrFrameTooLarge     = errors.New("frame too large")
	ErrInvalidFrame      = errors.New("invalid frame")
	ErrUnsupportedMethod = errors.New("unsupported method")
)

type Mode string

const (
	ModeDirect Mode = "direct"
	ModeTor    Mode = "tor"
	ModeVPN    Mode = "vpn"
	ModeVPNTor Mode = "vpn+tor"
)

type Config struct {
	Mode            Mode
	SocksPort       int
	DialTimeout     time.Duration
	WriteTimeout    time.Duration
	ReadTimeout     time.Duration
	Secret          []byte
	MaxQueue        int
	MaxPayloadBytes int
	ReplayWindow    time.Duration
	Clock           func() time.Time
}

type Envelope struct {
	Version   int             `json:"v"`
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	ChannelID string          `json:"channel_id,omitempty"`
	SessionID string          `json:"session_id,omitempty"`
	Topic     string          `json:"topic,omitempty"`
	Kind      string          `json:"kind,omitempty"`
	// Payload holds plaintext and is EMPTY whenever Encrypted is true. Keeping
	// plaintext and ciphertext in separate fields makes "encrypted" structural:
	// there is no way to accidentally ship a payload that was never sealed.
	Payload json.RawMessage `json:"payload,omitempty"`
	// Ciphertext holds the AEAD output (nonce || sealed bytes). json encodes
	// []byte as base64, so arbitrary binary ciphertext is wire-safe.
	Ciphertext []byte `json:"ct,omitempty"`
	Encrypted  bool   `json:"enc,omitempty"`
	Timestamp  int64  `json:"ts"`
	TTL        int64  `json:"ttl,omitempty"`
	Nonce      string `json:"nonce"`
	Signature  string `json:"sig,omitempty"`
	Meta       jsonMeta `json:"meta,omitempty"`
}

type jsonMeta map[string]any

type PublishRequest struct {
	Type      string          `json:"type"`
	ChannelID string          `json:"channel_id,omitempty"`
	SessionID string          `json:"session_id,omitempty"`
	Topic     string          `json:"topic,omitempty"`
	Kind      string          `json:"kind,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	TTL       int64           `json:"ttl,omitempty"`
	Meta      jsonMeta        `json:"meta,omitempty"`
}

type QueueMessage struct {
	Envelope   Envelope  `json:"envelope"`
	ReceivedAt time.Time `json:"received_at"`
}

type Status struct {
	Mode            Mode  `json:"mode"`
	QueueSize       int   `json:"queue_size"`
	QueueCapacity   int   `json:"queue_capacity"`
	Subscribers     int   `json:"subscribers"`
	ReplayCacheSize int   `json:"replay_cache_size"`
	MaxPayloadBytes int   `json:"max_payload_bytes"`
	ReplayWindowSec int64 `json:"replay_window_seconds"`
	UptimeSeconds   int64 `json:"uptime_seconds"`
}

func RandomSecret() ([]byte, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func RandomID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return hex.EncodeToString([]byte(time.Now().UTC().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(buf)
}

func normalizeConfig(cfg Config) (Config, error) {
	if cfg.Mode == "" {
		cfg.Mode = ModeDirect
	}
	switch cfg.Mode {
	case ModeDirect, ModeTor, ModeVPN, ModeVPNTor:
	default:
		return cfg, ErrInvalidMode
	}
	if cfg.SocksPort == 0 {
		cfg.SocksPort = 9050
	}
	if cfg.DialTimeout == 0 {
		cfg.DialTimeout = 10 * time.Second
	}
	if cfg.WriteTimeout == 0 {
		cfg.WriteTimeout = 5 * time.Second
	}
	if cfg.ReadTimeout == 0 {
		cfg.ReadTimeout = 30 * time.Second
	}
	if cfg.MaxQueue <= 0 {
		cfg.MaxQueue = 4096
	}
	if cfg.MaxPayloadBytes <= 0 {
		cfg.MaxPayloadBytes = 1 << 20
	}
	if cfg.ReplayWindow == 0 {
		cfg.ReplayWindow = 10 * time.Minute
	}
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	if len(cfg.Secret) == 0 {
		secret, err := RandomSecret()
		if err != nil {
			return cfg, err
		}
		cfg.Secret = secret
	}
	return cfg, nil
}
