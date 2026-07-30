// Package api provides HTTP handlers and API endpoints for the simplex-node server
package api

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strconv"
	"strings"

	"ParanoidX/internal/crypto/bip39"
)

// BIP39GenerateHandler creates a new BIP39 mnemonic.
// POST /api/crypto/bip39/generate
// Optional JSON body: {"bits": 128|160|192|224|256} (default 256 → 24 words)
func BIP39GenerateHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			writeJSON(w, map[string]any{"error": "POST required"})
			return
		}

		bits := 256
		if r.Body != nil && r.ContentLength > 0 {
			var req struct {
				Bits int `json:"bits"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err == nil && req.Bits > 0 {
				bits = req.Bits
			}
		}

		validBits := map[int]bool{128: true, 160: true, 192: true, 224: true, 256: true}
		if !validBits[bits] {
			writeJSON(w, map[string]any{"error": fmt.Sprintf("invalid bits: %d (must be 128, 160, 192, 224, or 256)", bits)})
			return
		}

		entropy := make([]byte, bits/8)
		if _, err := rand.Read(entropy); err != nil {
			writeJSON(w, map[string]any{"error": "entropy generation failed: " + err.Error()})
			return
		}

		mnemonic, err := bip39.MnemonicFromEntropy(entropy)
		if err != nil {
			writeJSON(w, map[string]any{"error": "mnemonic generation failed: " + err.Error()})
			return
		}

		writeJSON(w, map[string]any{
			"mnemonic": mnemonic,
			"bits":     bits,
			"words":    strings.Fields(mnemonic),
			"count":    len(strings.Fields(mnemonic)),
		})
	}
}

// BIP39ValidateHandler validates a BIP39 mnemonic phrase.
// POST /api/crypto/bip39/validate
// JSON body: {"mnemonic": "..."}
func BIP39ValidateHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			writeJSON(w, map[string]any{"ok": false, "error": "POST required"})
			return
		}

		var req struct {
			Mnemonic string `json:"mnemonic"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "bad request: " + err.Error()})
			return
		}

		req.Mnemonic = strings.TrimSpace(req.Mnemonic)
		if req.Mnemonic == "" {
			writeJSON(w, map[string]any{"ok": false, "error": "mnemonic required"})
			return
		}

		_, err := bip39.EntropyFromMnemonic(req.Mnemonic)
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}

		words := strings.Fields(req.Mnemonic)
		wordCount := len(words)
		bits := wordCount / 3 * 32

		writeJSON(w, map[string]any{
			"ok":         true,
			"words":      wordCount,
			"bits":       bits,
			"entropy":    fmt.Sprintf("%d bits", bits),
			"checksum":   "valid",
			"wordlist":   "BIP39 English",
		})
	}
}

// BIP39WordlistHandler returns the full BIP39 English wordlist.
// GET /api/crypto/bip39/wordlist
func BIP39WordlistHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			writeJSON(w, map[string]any{"error": "GET required"})
			return
		}

		// Support pagination with offset/limit query params
		offset := 0
		limit := len(bip39.WordList)
		if o := r.URL.Query().Get("offset"); o != "" {
			if n, err := strconv.Atoi(o); err == nil && n >= 0 && n < len(bip39.WordList) {
				offset = n
			}
		}
		if l := r.URL.Query().Get("limit"); l != "" {
			if n, err := strconv.Atoi(l); err == nil && n > 0 {
				limit = n
			}
		}
		if offset+limit > len(bip39.WordList) {
			limit = len(bip39.WordList) - offset
		}

		page := bip39.WordList[offset : offset+limit]
		writeJSON(w, map[string]any{
			"words":  page,
			"total":  len(bip39.WordList),
			"offset": offset,
			"limit":  limit,
		})
	}
}

// randomInt returns a cryptographically secure random integer in [0, max).
func randomInt(max int) (int, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return 0, err
	}
	return int(n.Int64()), nil
}
