// Package api provides HTTP handlers and API endpoints for the simplex-node server
package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"ParanoidX/internal/crypto/bip39"
)

type accountCreateResp struct {
	Pubkey   string `json:"pubkey"`
	Privkey  string `json:"privkey"`
	Mnemonic string `json:"mnemonic"`
}

type accountRestoreReq struct {
	Mnemonic   string `json:"mnemonic"`
	Passphrase string `json:"passphrase,omitempty"`
}

type accountVerifyReq struct {
	Mnemonic   string `json:"mnemonic"`
	Passphrase string `json:"passphrase,omitempty"`
	Pubkey     string `json:"pubkey"`
}


// AccountCreateHandler handles the AccountCreateHandler HTTP request.
// AccountCreateHandler generates a new BIP39 mnemonic and derives a keypair.
func AccountCreateHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		mnemonic, err := bip39.GenerateMnemonic()
		if err != nil {
			http.Error(w, "generate mnemonic: "+err.Error(), 500)
			return
		}
		pubkey, privkey, err := bip39.KeypairFromMnemonic(mnemonic, "")
		if err != nil {
			http.Error(w, "derive keypair: "+err.Error(), 500)
			return
		}
		writeJSON(w, accountCreateResp{
			Pubkey:   pubkey,
			Privkey:  privkey,
			Mnemonic: mnemonic,
		})
	}
}


// AccountRestoreHandler handles the AccountRestoreHandler HTTP request.
// AccountRestoreHandler derives a keypair from an existing BIP39 mnemonic.
func AccountRestoreHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		var req accountRestoreReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", 400)
			return
		}
		req.Mnemonic = strings.TrimSpace(req.Mnemonic)
		if req.Mnemonic == "" {
			http.Error(w, "mnemonic required", 400)
			return
		}
		_, err := bip39.EntropyFromMnemonic(req.Mnemonic)
		if err != nil {
			http.Error(w, "invalid mnemonic: "+err.Error(), 400)
			return
		}
		pubkey, privkey, err := bip39.KeypairFromMnemonic(req.Mnemonic, req.Passphrase)
		if err != nil {
			http.Error(w, "derive keypair: "+err.Error(), 500)
			return
		}
		writeJSON(w, accountCreateResp{
			Pubkey:  pubkey,
			Privkey: privkey,
		})
	}
}


// AccountVerifyHandler handles the AccountVerifyHandler HTTP request.
// AccountVerifyHandler checks that a mnemonic derives to the given pubkey.
func AccountVerifyHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		var req accountVerifyReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", 400)
			return
		}
		req.Mnemonic = strings.TrimSpace(req.Mnemonic)
		if req.Mnemonic == "" || req.Pubkey == "" {
			http.Error(w, "mnemonic and pubkey required", 400)
			return
		}
		_, err := bip39.EntropyFromMnemonic(req.Mnemonic)
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "invalid mnemonic: " + err.Error()})
			return
		}
		pubkey, _, err := bip39.KeypairFromMnemonic(req.Mnemonic, req.Passphrase)
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		if pubkey != req.Pubkey {
			writeJSON(w, map[string]any{"ok": false, "error": "mnemonic does not match pubkey"})
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	}
}
