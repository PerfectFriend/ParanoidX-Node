package api

import (
	"encoding/json"
	"net/http"
	"time"

	"ParanoidX/internal/store"
)

func ExternalWalletListHandler(ews *store.ExternalWalletStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pubkey := r.URL.Query().Get("pubkey")
		if pubkey == "" {
			http.Error(w, "pubkey required", 400)
			return
		}
		wallets, err := ews.List(pubkey)
		if err != nil {
			writeJSON(w, map[string]any{"error": err.Error()})
			return
		}
		if wallets == nil {
			wallets = []store.ExternalWallet{}
		}
		writeJSON(w, map[string]any{"wallets": wallets})
	}
}

func ExternalWalletLinkHandler(ews *store.ExternalWalletStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		var req struct {
			Pubkey        string `json:"pubkey"`
			WalletType    string `json:"wallet_type"`
			WalletAddress string `json:"wallet_address"`
			Label         string `json:"label,omitempty"`
			Chain         string `json:"chain,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]any{"error": "bad request"})
			return
		}
		if req.Pubkey == "" || req.WalletType == "" || req.WalletAddress == "" {
			writeJSON(w, map[string]any{"error": "pubkey, wallet_type, and wallet_address required"})
			return
		}
		if req.Chain == "" {
			req.Chain = "all"
		}
		ew := store.ExternalWallet{
			Pubkey:        req.Pubkey,
			WalletType:    req.WalletType,
			WalletAddress: req.WalletAddress,
			Label:         req.Label,
			Chain:         req.Chain,
			IsVerified:    false,
			CreatedAt:     time.Now(),
		}
		if err := ews.Add(ew); err != nil {
			writeJSON(w, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "wallet": ew})
	}
}

func ExternalWalletUnlinkHandler(ews *store.ExternalWalletStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" && r.Method != "POST" {
			http.Error(w, "DELETE or POST required", 405)
			return
		}
		pubkey := r.URL.Query().Get("pubkey")
		walletType := r.URL.Query().Get("wallet_type")
		if pubkey == "" || walletType == "" {
			var req struct {
				Pubkey     string `json:"pubkey"`
				WalletType string `json:"wallet_type"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
				pubkey, walletType = req.Pubkey, req.WalletType
			}
		}
		if pubkey == "" || walletType == "" {
			writeJSON(w, map[string]any{"error": "pubkey and wallet_type required"})
			return
		}
		if err := ews.Remove(pubkey, walletType); err != nil {
			writeJSON(w, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	}
}

func ExternalWalletSyncHandler(ews *store.ExternalWalletStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		pubkey := r.URL.Query().Get("pubkey")
		if pubkey == "" {
			var req struct{ Pubkey string `json:"pubkey"` }
			if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
				pubkey = req.Pubkey
			}
		}
		if pubkey == "" {
			http.Error(w, "pubkey required", 400)
			return
		}
		wallets, _ := ews.List(pubkey)
		for _, w := range wallets {
			ews.UpdateSyncTime(pubkey, w.WalletType)
		}
		writeJSON(w, map[string]any{
			"ok":      true,
			"synced":  len(wallets),
			"message": "Sync started",
		})
	}
}

func ExternalWalletVerifyHandler(ews *store.ExternalWalletStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		var req struct {
			Pubkey     string `json:"pubkey"`
			WalletType string `json:"wallet_type"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]any{"error": "bad request"})
			return
		}
		if err := ews.Verify(req.Pubkey, req.WalletType); err != nil {
			writeJSON(w, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "message": "Wallet verified"})
	}
}
