// Package api provides HTTP handlers and API endpoints for the simplex-node server
package api

import (
	"encoding/json"
	"net/http"

	"ParanoidX/internal/crypto/btc"
)

var swapRegistry = btc.NewRegistry()

type swapRequest struct {
	Initiator    string `json:"initiator"`
	Counterparty string `json:"counterparty"`
	AmountNg     int64  `json:"amount_ng"`
	SecretHash   string `json:"secret_hash"`
}

type claimRequest struct {
	SwapID string `json:"swap_id"`
	Secret string `json:"secret"`
}

type refundRequest struct {
	SwapID string `json:"swap_id"`
}

type confirmRequest struct {
	SwapID string `json:"swap_id"`
}

type cancelRequest struct {
	SwapID string `json:"swap_id"`
}


// SwapCreateHandler initiates an atomic swap between two parties.
func SwapCreateHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		if !requireContentType(r) {
			writeJSON(w, map[string]any{"ok": false, "error": "Content-Type must be application/json"})
			return
		}
		var req swapRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", 400)
			return
		}
		if req.Initiator == "" || req.Counterparty == "" {
			writeJSON(w, map[string]any{"ok": false, "error": "initiator and counterparty required"})
			return
		}
		if req.AmountNg <= 0 {
			writeJSON(w, map[string]any{"ok": false, "error": "amount_ng must be positive"})
			return
		}
		swap, err := swapRegistry.Create(req.Initiator, req.Counterparty, req.AmountNg, req.SecretHash)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		// Return pending — requires confirmation via swap/confirm
		writeJSON(w, map[string]any{
			"ok":         true,
			"swap_id":    swap.ID,
			"status":     swap.Status,
			"message":    "swap created, confirm via POST /api/swap/confirm",
			"expires_at": swap.LockTime,
		})
	}
}


// SwapConfirmHandler handles the SwapConfirmHandler HTTP request.
// SwapConfirmHandler confirms a pending atomic swap.
func SwapConfirmHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		if !requireContentType(r) {
			writeJSON(w, map[string]any{"ok": false, "error": "Content-Type must be application/json"})
			return
		}
		var req confirmRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", 400)
			return
		}
		if req.SwapID == "" {
			writeJSON(w, map[string]any{"ok": false, "error": "swap_id required"})
			return
		}
		swap, err := swapRegistry.Confirm(req.SwapID)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "swap": swap})
	}
}


// SwapCancelHandler handles the SwapCancelHandler HTTP request.
// SwapCancelHandler cancels a swap before it is confirmed.
func SwapCancelHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		if !requireContentType(r) {
			writeJSON(w, map[string]any{"ok": false, "error": "Content-Type must be application/json"})
			return
		}
		var req cancelRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", 400)
			return
		}
		if req.SwapID == "" {
			writeJSON(w, map[string]any{"ok": false, "error": "swap_id required"})
			return
		}
		swap, err := swapRegistry.Cancel(req.SwapID)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "swap": swap})
	}
}


// SwapClaimHandler handles the SwapClaimHandler HTTP request.
// SwapClaimHandler claims a swap using the secret preimage.
func SwapClaimHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		if !requireContentType(r) {
			writeJSON(w, map[string]any{"ok": false, "error": "Content-Type must be application/json"})
			return
		}
		var req claimRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", 400)
			return
		}
		swap, err := swapRegistry.Claim(req.SwapID, req.Secret)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		writeJSON(w, swap)
	}
}


// SwapRefundHandler handles the SwapRefundHandler HTTP request.
// SwapRefundHandler refunds an expired swap to the initiator.
func SwapRefundHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		if !requireContentType(r) {
			writeJSON(w, map[string]any{"ok": false, "error": "Content-Type must be application/json"})
			return
		}
		var req refundRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", 400)
			return
		}
		swap, err := swapRegistry.Refund(req.SwapID)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		writeJSON(w, swap)
	}
}


// SwapListHandler handles the SwapListHandler HTTP request.
// SwapListHandler returns all swaps in the registry.
func SwapListHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		swaps := swapRegistry.List()
		writeJSON(w, map[string]any{"swaps": swaps})
	}
}

// SwapExpiryCleaner runs in a goroutine to auto-expire swaps after 24h.
func SwapExpiryCleaner() {
	swapRegistry.CleanExpired()
}
