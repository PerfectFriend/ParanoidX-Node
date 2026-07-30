// Package api provides HTTP handlers and API endpoints for the simplex-node server
package api

import (
	"encoding/json"
	"net/http"

	"ParanoidX/internal/crypto/eth"
)

var bridgeRegistry = eth.NewBridgeRegistry()

type bridgeCreateReq struct {
	FromChain string `json:"from_chain"`
	ToChain   string `json:"to_chain"`
	Token     string `json:"token"`
	Amount    int64  `json:"amount"`
	Sender    string `json:"sender"`
	Recipient string `json:"recipient"`
}

type bridgeConfirmReq struct {
	ID     string `json:"id"`
	TxHash string `json:"tx_hash"`
}

type bridgeCompleteReq struct {
	ID          string `json:"id"`
	ProofTxHash string `json:"proof_tx_hash"`
}

// BridgeCreateHandler initiates a cross-chain token transfer.
func BridgeCreateHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		var req bridgeCreateReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", 400)
			return
		}
		tf := bridgeRegistry.Create(req.FromChain, req.ToChain, req.Token, req.Sender, req.Recipient, req.Amount)
		writeJSON(w, tf)
	}
}

// BridgeConfirmHandler confirms that a transfer was initiated on the source chain.
func BridgeConfirmHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		var req bridgeConfirmReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", 400)
			return
		}
		tf, err := bridgeRegistry.Confirm(req.ID, req.TxHash)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		writeJSON(w, tf)
	}
}

// BridgeCompleteHandler marks a transfer as complete with proof transaction hash.
func BridgeCompleteHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		var req bridgeCompleteReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", 400)
			return
		}
		tf, err := bridgeRegistry.Complete(req.ID, req.ProofTxHash)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		writeJSON(w, tf)
	}
}

// BridgeListHandler returns all cross-chain transfers.
func BridgeListHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		transfers := bridgeRegistry.List()
		writeJSON(w, map[string]any{"transfers": transfers})
	}
}

// BridgeStatusHandler returns aggregated transfer counts by status (pending, completed, failed).
func BridgeStatusHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "GET required", 405)
			return
		}
		transfers := bridgeRegistry.List()
		pending := 0
		completed := 0
		failed := 0
		for _, t := range transfers {
			switch t.Status {
			case "pending", "verified":
				pending++
			case "complete":
				completed++
			case "failed":
				failed++
			}
		}
		writeJSON(w, map[string]any{
			"ok":              true,
			"total_transfers": len(transfers),
			"pending":         pending,
			"completed":       completed,
			"failed":          failed,
		})
	}
}
