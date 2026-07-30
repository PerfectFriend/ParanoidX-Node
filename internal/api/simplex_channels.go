// Package api provides HTTP handlers and API endpoints for the simplex-node server
package api

import (
	"bytes"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"ParanoidX/internal/middleware"
)

// ── DID Key Manager ─────────────────────────────────────────────────────────

type DIDKeyStore struct {
	mu           sync.RWMutex
	path         string
	NodeSeed     []byte `json:"node_seed"`
	NodePubkey   []byte `json:"node_pubkey"`
	StewardSeed  []byte `json:"steward_seed"`
	StewardPubkey []byte `json:"steward_pubkey"`
}

var globalDIDKeys *DIDKeyStore


// InitDIDKeys handles the InitDIDKeys HTTP request.
func InitDIDKeys(dataDir string) {
	globalDIDKeys = &DIDKeyStore{path: filepath.Join(dataDir, "did_keys.json")}
	data, err := os.ReadFile(globalDIDKeys.path)
	if err == nil {
		json.Unmarshal(data, globalDIDKeys)
	}
	// Generate keys if missing
	if len(globalDIDKeys.NodeSeed) == 0 {
		seed := make([]byte, ed25519.SeedSize)
		rand.Read(seed)
		pub := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
		globalDIDKeys.NodeSeed = seed
		globalDIDKeys.NodePubkey = pub
	}
	if len(globalDIDKeys.StewardSeed) == 0 {
		seed := make([]byte, ed25519.SeedSize)
		rand.Read(seed)
		pub := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
		globalDIDKeys.StewardSeed = seed
		globalDIDKeys.StewardPubkey = pub
	}
	globalDIDKeys.save()
	slog.Info("DID keys initialized", "node_pubkey", globalDIDKeys.PubkeyB58())
}

func (k *DIDKeyStore) save() {
	data, _ := json.MarshalIndent(k, "", "  ")
	os.WriteFile(k.path, data, 0600)
}


// PubkeyB58 handles the PubkeyB58 HTTP request.
func (k *DIDKeyStore) PubkeyB58() string {
	return base64.RawURLEncoding.EncodeToString(k.NodePubkey)
}


// StewardPubkeyB58 handles the StewardPubkeyB58 HTTP request.
func (k *DIDKeyStore) StewardPubkeyB58() string {
	return base64.RawURLEncoding.EncodeToString(k.StewardPubkey)
}


// NodeDID handles the NodeDID HTTP request.
func (k *DIDKeyStore) NodeDID() string {
	return "did:island:node-" + k.PubkeyB58()[:16]
}


// StewardDID handles the StewardDID HTTP request.
func (k *DIDKeyStore) StewardDID() string {
	return "did:island:steward-" + k.StewardPubkeyB58()[:16]
}


// ChannelCreateHandler handles the ChannelCreateHandler HTTP request.
func ChannelCreateHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		if SimplexCmd == nil || !BridgeConnected {
			writeJSON(w, map[string]any{"ok": false, "error": "bridge not connected"})
			return
		}
		var req struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
			return
		}
		if req.Name == "" {
			writeJSON(w, map[string]any{"ok": false, "error": "name required"})
			return
		}
		resp, err := SimplexCmd(fmt.Sprintf("/_create_channel %s", req.Name))
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "response": resp})
	}
}


// ChannelListHandler handles the ChannelListHandler HTTP request.
func ChannelListHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		if SimplexCmd == nil || !BridgeConnected {
			writeJSON(w, map[string]any{"ok": false, "channels": []any{}})
			return
		}
		resp, err := SimplexCmd("/_channels")
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "response": resp})
	}
}


// ChannelJoinHandler handles the ChannelJoinHandler HTTP request.
func ChannelJoinHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		if SimplexCmd == nil || !BridgeConnected {
			writeJSON(w, map[string]any{"ok": false, "error": "bridge not connected"})
			return
		}
		var req struct {
			Link string `json:"link"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
			return
		}
		if req.Link == "" {
			writeJSON(w, map[string]any{"ok": false, "error": "link required"})
			return
		}
		resp, err := SimplexCmd(fmt.Sprintf("/_join_channel %s", req.Link))
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "response": resp})
	}
}


// ChannelPostHandler handles the ChannelPostHandler HTTP request.
func ChannelPostHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		if SimplexCmd == nil || !BridgeConnected {
			writeJSON(w, map[string]any{"ok": false, "error": "bridge not connected"})
			return
		}
		var req struct {
			ChannelID string `json:"channel_id"`
			Text      string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
			return
		}
		if req.ChannelID == "" || req.Text == "" {
			writeJSON(w, map[string]any{"ok": false, "error": "channel_id and text required"})
			return
		}
		resp, err := SimplexCmd(fmt.Sprintf("/_channel %s %s", req.ChannelID, req.Text))
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "response": resp})
	}
}

// ── DID Verification (b110) ───────────────────────────────────────────────────

type DIDDocument struct {
	Context            []string `json:"@context"`
	ID                 string   `json:"id"`
	VerificationMethod []VMethod `json:"verificationMethod"`
	Authentication     []string `json:"authentication"`
	Service            []DIDService `json:"service,omitempty"`
}

type VMethod struct {
	ID              string `json:"id"`
	Type            string `json:"type"`
	Controller      string `json:"controller"`
	PublicKeyBase58 string `json:"publicKeyBase58,omitempty"`
}

type DIDService struct {
	ID              string `json:"id"`
	Type            string `json:"type"`
	ServiceEndpoint string `json:"serviceEndpoint"`
}



// DIDHandler handles the DIDHandler HTTP request.
func DIDHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if globalDIDKeys == nil {
			writeJSON(w, map[string]any{"ok": false, "error": "DID not initialized"})
			return
		}
		did := globalDIDKeys.NodeDID()
		doc := DIDDocument{
			Context: []string{"https://www.w3.org/ns/did/v1"},
			ID:      did,
			VerificationMethod: []VMethod{
				{
					ID:              did + "#keys-1",
					Type:            "Ed25519VerificationKey2020",
					Controller:      did,
					PublicKeyBase58: globalDIDKeys.PubkeyB58(),
				},
			},
			Authentication: []string{did + "#keys-1"},
			Service: []DIDService{
				{
					ID:              did + "#simplex-msg",
					Type:            "SimplexMessaging",
					ServiceEndpoint: "https://" + r.Host + "/api/chat/send",
				},
				{
					ID:              did + "#simplex-relay",
					Type:            "SimplexRelay",
					ServiceEndpoint: "https://" + r.Host + "/api/relay/message",
				},
			},
		}
		writeJSON(w, map[string]any{"ok": true, "did": doc, "pubkey": globalDIDKeys.PubkeyB58()})
	}
}


// ContactDIDHandler handles the ContactDIDHandler HTTP request.
func ContactDIDHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		contactID := r.URL.Query().Get("contact_id")
		if contactID == "" {
			writeJSON(w, map[string]any{"ok": false, "error": "contact_id required"})
			return
		}
		did := fmt.Sprintf("did:island:contact-%s", contactID)

		// Derive unique pubkey per contact via HMAC-SHA256(nodeSeed, "contact:"+contactID)
		pubkey := ""
		if globalDIDKeys != nil && len(globalDIDKeys.NodeSeed) > 0 {
			mac := hmac.New(sha256.New, globalDIDKeys.NodeSeed)
			mac.Write([]byte("contact:" + contactID))
			derived := mac.Sum(nil)
			pubkey = base64.RawURLEncoding.EncodeToString(derived)
		}

		writeJSON(w, map[string]any{
			"ok": true,
			"did": DIDDocument{
				Context: []string{"https://www.w3.org/ns/did/v1"},
				ID:      did,
				VerificationMethod: []VMethod{
					{
						ID:              did + "#keys-1",
						Type:            "Ed25519VerificationKey2020",
						Controller:      did,
						PublicKeyBase58: pubkey,
					},
				},
				Authentication: []string{did + "#keys-1"},
			},
			"derivation": "hmac-sha256(node_seed, contact:" + contactID + ")",
			"pubkey_hex": hex.EncodeToString([]byte(pubkey)),
		})
	}
}

// ── Inter-Node Relay (b112) ───────────────────────────────────────────────────

type RelayMessage struct {
	FromNode    string `json:"from_node"`
	ToNode      string `json:"to_node"`
	ContactID   string `json:"contact_id"`
	Text        string `json:"text"`
	RelayedAt   string `json:"relayed_at,omitempty"`
}

var relayHistory []RelayMessage


// RelayReceiveHandler handles the RelayReceiveHandler HTTP request.
func RelayReceiveHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		var msg RelayMessage
		if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
			return
		}
		msg.RelayedAt = time.Now().UTC().Format(time.RFC3339)
		relayHistory = append(relayHistory, msg)
		if len(relayHistory) > 1000 {
			relayHistory = relayHistory[len(relayHistory)-1000:]
		}
		// Forward to local contact if SimplexCmd available
		if SimplexCmd != nil && BridgeConnected && msg.ContactID != "" {
			cid := int64(0)
			fmt.Sscanf(msg.ContactID, "@%d", &cid)
			if cid > 0 {
				SimplexCmd(fmt.Sprintf("/_send %d %s", cid, msg.Text))
			}
		}
		writeJSON(w, map[string]any{"ok": true, "relayed": msg.RelayedAt})
	}
}


// RelaySendHandler handles the RelaySendHandler HTTP request.
func RelaySendHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		var req struct {
			TargetNode string `json:"target_node"`
			ContactID  string `json:"contact_id"`
			Text       string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
			return
		}
		if req.TargetNode == "" || req.Text == "" {
			writeJSON(w, map[string]any{"ok": false, "error": "target_node and text required"})
			return
		}

		// Forward to target node's /api/relay/receive endpoint
		targetURL := strings.TrimRight(req.TargetNode, "/") + "/api/relay/receive"
		forwardPayload := map[string]any{
			"from_node":  r.Host,
			"to_node":    req.TargetNode,
			"contact_id": req.ContactID,
			"text":       req.Text,
		}
		body, _ := json.Marshal(forwardPayload)
		resp, err := http.Post(targetURL, "application/json", bytes.NewReader(body))
		sent := false
		relayErr := ""
		if err != nil {
			relayErr = err.Error()
			slog.Error("relay forward failed", "target", req.TargetNode, "error", err)
		} else {
			defer resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				sent = true
			} else {
				relayErr = fmt.Sprintf("target returned %d", resp.StatusCode)
			}
		}

		// Record in relay history
		now := time.Now().UTC().Format(time.RFC3339)
		relayHistory = append(relayHistory, RelayMessage{
			FromNode:  r.Host,
			ToNode:    req.TargetNode,
			ContactID: req.ContactID,
			Text:      req.Text,
			RelayedAt: now,
		})
		if len(relayHistory) > 1000 {
			relayHistory = relayHistory[len(relayHistory)-1000:]
		}

		writeJSON(w, map[string]any{
			"ok":         true,
			"sent":       sent,
			"target":     req.TargetNode,
			"contact_id": req.ContactID,
			"error":      relayErr,
		})
	}
}


// RelayHistoryHandler handles the RelayHistoryHandler HTTP request.
func RelayHistoryHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		limitStr := r.URL.Query().Get("limit")
		limit := 20
		fmt.Sscanf(limitStr, "%d", &limit)
		if limit > len(relayHistory) {
			limit = len(relayHistory)
		}
		var out []RelayMessage
		if limit > 0 {
			out = relayHistory[len(relayHistory)-limit:]
		} else {
			out = []RelayMessage{}
		}
		writeJSON(w, map[string]any{"ok": true, "messages": out, "total": len(relayHistory)})
	}
}

// ── Bridge Auto-Config (b111) ─────────────────────────────────────────────────

type BridgeConfig struct {
	CLIBin        string   `json:"cli_bin"`
	DataDir       string   `json:"data_dir"`
	Port          int      `json:"port"`
	AutoAccept    bool     `json:"auto_accept"`
	Reconnect     bool     `json:"reconnect"`
	ReconnectWait int      `json:"reconnect_wait"`
	Features      []string `json:"features"`
}


// BridgeConfigHandler handles the BridgeConfigHandler HTTP request.
func BridgeConfigHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		cfg := BridgeConfig{
			CLIBin:        "/home/tomas/bin/simplex-chat-island",
			DataDir:       r.URL.Query().Get("data_dir"),
			Port:          17225,
			AutoAccept:    true,
			Reconnect:     true,
			ReconnectWait: 15,
			Features:      []string{"auto_accept", "reconnect", "reply_async", "wallet_commands"},
		}
		if cfg.DataDir == "" {
			cfg.DataDir = "/home/tomas/.local/share/simplex-node"
		}
		writeJSON(w, map[string]any{"ok": true, "config": cfg})
	}
}
