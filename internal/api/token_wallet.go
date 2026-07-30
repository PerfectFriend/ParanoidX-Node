package api

import (
	"encoding/json"
	"net/http"

	"ParanoidX/internal/store"
)

func TokenListHandler(ts *store.TokenStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tokens, err := ts.ListTokens()
		if err != nil {
			writeJSON(w, map[string]any{"error": err.Error()})
			return
		}
		if tokens == nil {
			tokens = []store.Token{}
		}
		writeJSON(w, map[string]any{"tokens": tokens})
	}
}

func TokenBalancesHandler(ts *store.TokenStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pubkey := r.URL.Query().Get("pubkey")
		if pubkey == "" {
			http.Error(w, "pubkey required", 400)
			return
		}
		balances, err := ts.GetBalances(pubkey)
		if err != nil {
			writeJSON(w, map[string]any{"error": err.Error()})
			return
		}
		if balances == nil {
			balances = []store.TokenBalance{}
		}
		writeJSON(w, map[string]any{"balances": balances})
	}
}

func TokenAddCustomHandler(ts *store.TokenStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		var t store.Token
		if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
			writeJSON(w, map[string]any{"error": "bad request"})
			return
		}
		if t.Symbol == "" || t.Name == "" {
			writeJSON(w, map[string]any{"error": "symbol and name required"})
			return
		}
		t.IsCustom = true
		if err := ts.AddToken(t); err != nil {
			writeJSON(w, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "token": t})
	}
}

func TokenRemoveCustomHandler(ts *store.TokenStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		var req struct{ Symbol string `json:"symbol"` }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]any{"error": "bad request"})
			return
		}
		if err := ts.RemoveToken(req.Symbol); err != nil {
			writeJSON(w, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	}
}

func TokenUpdateBalanceHandler(ts *store.TokenStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		var req struct {
			Pubkey  string `json:"pubkey"`
			Symbol  string `json:"symbol"`
			Balance string `json:"balance"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]any{"error": "bad request"})
			return
		}
		if err := ts.SetBalance(req.Pubkey, req.Symbol, req.Balance); err != nil {
			writeJSON(w, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	}
}
