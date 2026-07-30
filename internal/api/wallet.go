// Package api provides HTTP handlers and API endpoints for the simplex-node server
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"ParanoidX/internal/crypto/bip39"
	"ParanoidX/internal/economy"
	"ParanoidX/internal/fileutil"
	"ParanoidX/internal/store"
)

// walletTx records a single wallet transaction (send/receive).
type walletTx struct {
	ID        string `json:"id"`
	Type      string `json:"type"` // send, receive
	From      string `json:"from"`
	To        string `json:"to"`
	AmountNg  int64  `json:"amount_ng"`
	FeeNg     int64  `json:"fee_ng"`
	Memo      string `json:"memo,omitempty"`
	Time      string `json:"time"`
	Confirmed bool   `json:"confirmed"`
}

func loadWalletTxs(dataDir string) []walletTx {
	p := filepath.Join(dataDir, "wallet_history.json")
	var txs []walletTx
	if b, err := os.ReadFile(p); err == nil {
		json.Unmarshal(b, &txs)
	}
	return txs
}

func saveWalletTxs(dataDir string, txs []walletTx) {
	p := filepath.Join(dataDir, "wallet_history.json")
	fileutil.WriteJSON(p, txs)
}

// WalletSendHandler processes a wallet send transaction with treasury fee deduction.
func WalletSendHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		var req struct {
			From     string `json:"from"`
			To       string `json:"to"`
			AmountNg int64  `json:"amount_ng"`
			Memo     string `json:"memo,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", 400)
			return
		}
		if req.AmountNg <= 0 {
			http.Error(w, "amount must be positive", 400)
			return
		}
		ledger := economy.LoadLedger(dataDir)
		bal := ledger.Balance(req.From)
		fee := req.AmountNg * 228 / 10000 // 2.28% treasury fee
		total := req.AmountNg + fee
		if bal < total {
			http.Error(w, "insufficient balance", 400)
			return
		}
		ledger.Transfer(req.From, req.To, req.AmountNg)
		ledger.Transfer(req.From, "treasury", fee)
		ledger.Save(dataDir)
		tx := walletTx{
			ID:        "tx-" + fmt.Sprintf("%d", time.Now().UnixNano()),
			Type:      "send",
			From:      req.From,
			To:        req.To,
			AmountNg:  req.AmountNg,
			FeeNg:     fee,
			Memo:      req.Memo,
			Time:      time.Now().Format(time.RFC3339),
			Confirmed: true,
		}
		txs := loadWalletTxs(dataDir)
		txs = append(txs, tx)
		saveWalletTxs(dataDir, txs)
		writeJSON(w, map[string]any{"ok": true, "tx": tx})
	}
}

// WalletReceiveHandler returns the balance and address info for receiving funds.
func WalletReceiveHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			To       string `json:"to"`
			From     string `json:"from"`
			AmountNg int64  `json:"amount_ng"`
			Memo     string `json:"memo,omitempty"`
		}
		if r.Method == "POST" {
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
				return
			}
		} else {
			req.To = r.URL.Query().Get("to")
			req.From = r.URL.Query().Get("from")
		}
		ledger := economy.LoadLedger(dataDir)
		bal := ledger.Balance(req.To)
		writeJSON(w, map[string]any{
			"address":  req.To,
			"balance":  bal,
			"messages": []string{"Send Liquid Taler to this address."},
		})
	}
}

// WalletHistoryHandler returns the wallet transaction history, optionally filtered by pubkey.
func WalletHistoryHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pubkey := r.URL.Query().Get("pubkey")
		txs := loadWalletTxs(dataDir)
		if pubkey != "" {
			filtered := make([]walletTx, 0)
			for _, tx := range txs {
				if tx.From == pubkey || tx.To == pubkey {
					filtered = append(filtered, tx)
				}
			}
			txs = filtered
		}
		if txs == nil {
			txs = []walletTx{}
		}
		writeJSON(w, map[string]any{"transactions": txs})
	}
}

// WalletExportHandler returns encrypted backup with mnemonic recovery.
func WalletExportHandler(ws *store.WalletStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pubkey := r.URL.Query().Get("pubkey")
		if pubkey == "" {
			writeJSON(w, map[string]any{"error": "pubkey required"})
			return
		}
		a, err := ws.GetAccountWithPIN(pubkey, r.URL.Query().Get("pin"))
		if err != nil {
			writeJSON(w, map[string]any{"error": "account not found: " + err.Error()})
			return
		}
		writeJSON(w, map[string]any{
			"pubkey":   a.Pubkey,
			"mnemonic": a.Mnemonic,
			"warning":  "Save mnemonic phrase in a safe place.",
		})
	}
}

// WalletRecoverHandler restores an account from mnemonic.
func WalletRecoverHandler(ws *store.WalletStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		var req struct {
			Mnemonic   string `json:"mnemonic"`
			Passphrase string `json:"passphrase,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]any{"error": "bad request"})
			return
		}
		if req.Mnemonic == "" {
			writeJSON(w, map[string]any{"error": "mnemonic required"})
			return
		}
		pubkey, privkey, err := bip39.KeypairFromMnemonic(req.Mnemonic, req.Passphrase)
		if err != nil {
			writeJSON(w, map[string]any{"error": "invalid mnemonic: " + err.Error()})
			return
		}
		a := &store.Account{
			Pubkey:    pubkey,
			Privkey:   privkey,
			Mnemonic:  req.Mnemonic,
			CreatedAt: time.Now(),
		}
		if err := ws.SaveAccountWithPIN(a, req.Passphrase); err != nil {
			writeJSON(w, map[string]any{"error": "save: " + err.Error()})
			return
		}
		writeJSON(w, map[string]any{
			"ok":      true,
			"pubkey":  pubkey,
			"message": "Wallet recovered.",
		})
	}
}
