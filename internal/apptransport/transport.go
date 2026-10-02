package apptransport

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ParanoidX/internal/socks5"
)

type Transport struct {
	cfg     Config
	dialer  *net.Dialer
	queue   *Queue
	replay  *ReplayGuard
	signals *SignalState
	started time.Time
}

func New(cfg Config) (*Transport, error) {
	cfg, err := normalizeConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &Transport{
		cfg:     cfg,
		dialer:  &net.Dialer{Timeout: cfg.DialTimeout},
		queue:   NewQueue(cfg.MaxQueue),
		replay:  NewReplayGuard(cfg.ReplayWindow, cfg.Clock),
		signals: NewSignalState(),
		started: time.Now().UTC(),
	}, nil
}

func (t *Transport) Config() Config {
	return t.cfg
}

func (t *Transport) Queue() *Queue {
	return t.queue
}

func (t *Transport) ReplayGuard() *ReplayGuard {
	return t.replay
}

func (t *Transport) SignalState() *SignalState {
	return t.signals
}

func (t *Transport) Dial(ctx context.Context, host string, port int) (net.Conn, error) {
	if port == 0 {
		port = 443
	}
	switch t.cfg.Mode {
	case ModeTor, ModeVPN, ModeVPNTor:
		return t.dialSOCKS5(ctx, host, port)
	default:
		return t.dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	}
}

func (t *Transport) dialSOCKS5(ctx context.Context, host string, port int) (net.Conn, error) {
	conn, err := t.dialer.DialContext(ctx, "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(t.cfg.SocksPort)))
	if err != nil {
		return nil, fmt.Errorf("socks5 connect: %w", err)
	}
	if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		conn.Close()
		return nil, fmt.Errorf("socks5 handshake write: %w", err)
	}
	var resp [2]byte
	if _, err := io.ReadFull(conn, resp[:]); err != nil {
		conn.Close()
		return nil, fmt.Errorf("socks5 handshake read: %w", err)
	}
	if resp[0] != 0x05 || resp[1] != 0x00 {
		conn.Close()
		return nil, fmt.Errorf("socks5 auth failed: %d", resp[1])
	}
	hostBytes := []byte(host)
	if len(hostBytes) > 255 {
		conn.Close()
		return nil, fmt.Errorf("host too long")
	}
	req := make([]byte, 7+len(hostBytes))
	req[0] = 0x05
	req[1] = 0x01
	req[2] = 0x00
	req[3] = 0x03
	req[4] = byte(len(hostBytes))
	copy(req[5:], hostBytes)
	req[5+len(hostBytes)] = byte(port >> 8)
	req[6+len(hostBytes)] = byte(port)
	if _, err := conn.Write(req); err != nil {
		conn.Close()
		return nil, fmt.Errorf("socks5 target write: %w", err)
	}
	if err := socks5.ReadBindReply(conn); err != nil {
		conn.Close()
		return nil, fmt.Errorf("socks5 target: %w", err)
	}
	return conn, nil
}

func (t *Transport) Publish(ctx context.Context, req PublishRequest) (Envelope, error) {
	if len(req.Payload) > t.cfg.MaxPayloadBytes {
		return Envelope{}, ErrPayloadTooLarge
	}
	env, err := NewEnvelope(req, t.cfg.Secret)
	if err != nil {
		return Envelope{}, err
	}
	if !t.queue.HasCapacity() {
		return Envelope{}, ErrQueueFull
	}
	if err := t.replay.Check(env); err != nil {
		return Envelope{}, err
	}
	if _, err := t.queue.Enqueue(env); err != nil {
		t.replay.Forget(env.Nonce)
		return Envelope{}, err
	}
	return env, nil
}

func (t *Transport) PublishEnvelope(ctx context.Context, env Envelope) error {
	if !t.queue.HasCapacity() {
		return ErrQueueFull
	}
	if err := env.Verify(t.cfg.Secret, t.cfg.Clock(), nil); err != nil {
		return err
	}
	if err := t.replay.Check(env); err != nil {
		return err
	}
	if _, err := t.queue.Enqueue(env); err != nil {
		t.replay.Forget(env.Nonce)
		return err
	}
	return nil
}

func (t *Transport) Subscribe() (<-chan QueueMessage, func()) {
	return t.queue.Subscribe()
}

func (t *Transport) WriteEnvelope(ctx context.Context, conn net.Conn, env Envelope) error {
	data, err := json.Marshal(env)
	if err != nil {
		return err
	}
	if len(data) > t.cfg.MaxPayloadBytes {
		return ErrFrameTooLarge
	}
	if conn == nil {
		return errors.New("nil connection")
	}
	if err := conn.SetWriteDeadline(time.Now().Add(t.cfg.WriteTimeout)); err != nil {
		return err
	}
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(data)))
	if _, err := conn.Write(lenBuf[:]); err != nil {
		return err
	}
	_, err = conn.Write(data)
	return err
}

func (t *Transport) ReadEnvelope(conn net.Conn) (Envelope, error) {
	if conn == nil {
		return Envelope{}, errors.New("nil connection")
	}
	if err := conn.SetReadDeadline(time.Now().Add(t.cfg.ReadTimeout)); err != nil {
		return Envelope{}, err
	}
	var lenBuf [4]byte
	if _, err := io.ReadFull(conn, lenBuf[:]); err != nil {
		return Envelope{}, err
	}
	size := binary.BigEndian.Uint32(lenBuf[:])
	if size == 0 || size > uint32(t.cfg.MaxPayloadBytes) {
		return Envelope{}, ErrInvalidFrame
	}
	buf := make([]byte, size)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return Envelope{}, err
	}
	return ParseEnvelope(buf)
}

func (t *Transport) StartReader(ctx context.Context, conn net.Conn) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		env, err := t.ReadEnvelope(conn)
		if err != nil {
			return err
		}
		if err := t.PublishEnvelope(context.Background(), env); err != nil {
			return err
		}
	}
}

func (t *Transport) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/status", t.StatusHandler)
	mux.HandleFunc("/publish", t.PublishHandler)
	mux.HandleFunc("/events", t.EventsHandler)
	mux.HandleFunc("/signal", t.SignalHandler)
	mux.HandleFunc("/health", t.HealthHandler)
	return mux
}

func (t *Transport) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	t.Handler().ServeHTTP(w, r)
}

func (t *Transport) StatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, ErrUnsupportedMethod)
		return
	}
	writeJSON(w, Status{
		Mode:            t.cfg.Mode,
		QueueSize:       t.queue.Len(),
		QueueCapacity:   t.cfg.MaxQueue,
		Subscribers:     t.queue.SubscriberCount(),
		ReplayCacheSize: t.replay.Size(),
		MaxPayloadBytes: t.cfg.MaxPayloadBytes,
		ReplayWindowSec: int64(t.cfg.ReplayWindow.Seconds()),
		UptimeSeconds:   int64(time.Since(t.started).Seconds()),
	})
}

func (t *Transport) HealthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, ErrUnsupportedMethod)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "service": "paranoidx-app-transport", "version": Version})
}

func (t *Transport) PublishHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, ErrUnsupportedMethod)
		return
	}
	defer r.Body.Close()
	var req PublishRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("bad json: %w", err))
		return
	}
	env, err := t.Publish(r.Context(), req)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, ErrPayloadTooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		writeError(w, status, err)
		return
	}
	writeJSON(w, env)
}

func (t *Transport) EventsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, ErrUnsupportedMethod)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, errors.New("streaming not supported"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	ch, unsubscribe := t.Subscribe()
	defer unsubscribe()
	for _, msg := range t.queue.History(256) {
		if err := writeSSE(w, "message", msg); err != nil {
			return
		}
	}
	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-ch:
			if err := writeSSE(w, "message", msg); err != nil {
				return
			}
		case <-ticker.C:
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return
			}
		}
		flusher.Flush()
	}
}

func (t *Transport) SignalHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, ErrUnsupportedMethod)
		return
	}
	room := r.URL.Query().Get("room")
	if room == "" {
		room = "default"
	}
	if r.Method == http.MethodPost {
		defer r.Body.Close()
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("bad json: %w", err))
			return
		}
		t.signals.PostSignal(room, payload)
		writeJSON(w, map[string]any{"ok": true, "room": room})
		return
	}
	writeJSON(w, t.signals.GetState(room))
}

func writeSSE(w http.ResponseWriter, event string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
	return err
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
}

func IsLocalOrOnionAccess(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	if ip != nil && (ip.IsLoopback() || ip.IsPrivate()) {
		return true
	}
	h := strings.ToLower(r.Host)
	return strings.HasSuffix(h, ".onion") || strings.HasSuffix(h, ".local")
}
