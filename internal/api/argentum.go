// Package api provides HTTP handlers and API endpoints for the simplex-node server
package api

import (
	"encoding/json"
	"net/http"
	"time"

	"ParanoidX/internal/economy"
	"ParanoidX/internal/ton"
)


// ArgentumHandler handles the ArgentumHandler HTTP request.
// ArgentumHandler provides Argentum token rate, market info, swap quotes, and swap execution.
func ArgentumHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		action := r.URL.Query().Get("action")
		switch action {
		case "rate":
			rate := ton.GetRate(75.0, 1.85)
			writeJSON(w, map[string]any{
				"symbol":         ton.ArgentumSymbol,
				"name":           ton.ArgentumName,
				"ng_per_ton":     rate,
				"ton_per_ng":     1.0 / rate,
				"silver_price":   75.0,
				"ton_price_usd":  1.85,
				"swap_fee_bps":   ton.SwapFeeBPS,
				"min_swap_ng":    ton.MinSwapNg,
				"updated_at":     time.Now().UTC().Format(time.RFC3339),
			})

		case "market":
			ledger := economy.LoadLedger(dataDir)
			totalSupply := ledger.Balance("treasury")
			writeJSON(w, map[string]any{
				"symbol":          ton.ArgentumSymbol,
				"name":            ton.ArgentumName,
				"decimals":        ton.ArgentumDecimals,
				"total_supply_ng": totalSupply,
				"circulating_ng":  ledger.TotalSupply,
				"backing_ratio":   0.70,
				"price_ton":       0.0,
				"price_usd":       0.0,
				"status":          "pre-launch",
			})

		case "swap-quote":
			var req struct {
				FromAsset  string `json:"from_asset"`
				FromAmount int64  `json:"from_amount"`
				Pubkey     string `json:"pubkey"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
				return
			}
			fee := req.FromAmount * ton.SwapFeeBPS / 10000
			toAmount := req.FromAmount - fee
			writeJSON(w, map[string]any{
				"from_asset":  req.FromAsset,
				"to_asset":    "argentum",
				"from_amount": req.FromAmount,
				"to_amount":   toAmount,
				"fee":         fee,
				"fee_bps":     ton.SwapFeeBPS,
			})

		case "swap":
			var req struct {
				FromAsset  string `json:"from_asset"`
				FromAmount int64  `json:"from_amount"`
				Pubkey     string `json:"pubkey"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
				return
			}
			if req.FromAmount < ton.MinSwapNg {
				http.Error(w, `{"error":"amount below minimum"}`, http.StatusBadRequest)
				return
			}
			if req.Pubkey == "" {
				http.Error(w, `{"error":"pubkey required"}`, http.StatusBadRequest)
				return
			}
			ledger := economy.LoadLedger(dataDir)

			fee := req.FromAmount * ton.SwapFeeBPS / 10000
			netAmount := req.FromAmount - fee

			swap := &ton.ArgentumSwap{
				ID:          economy.RandomID(),
				FromAsset:   req.FromAsset,
				ToAsset:     "argentum",
				FromAmount:  req.FromAmount,
				ToAmount:    netAmount,
				FeeNg:       fee,
				UserPubkey:  req.Pubkey,
				Status:      "completed",
				CreatedAt:   time.Now().UTC().Format(time.RFC3339),
				CompletedAt: time.Now().UTC().Format(time.RFC3339),
			}

			if req.FromAsset == "ton" {
				// TON → ARGENTUM: mint ng to user
				ledger.Mint(req.Pubkey, netAmount)
			} else if req.FromAsset == "ng" || req.FromAsset == "liquid_taler" {
				// ng → ARGENTUM: lock ng (virtual swap, ng stays in ledger)
				if err := ledger.Transfer(req.Pubkey, "treasury", req.FromAmount); err != nil {
					http.Error(w, `{"error":"insufficient balance"}`, http.StatusBadRequest)
					return
				}
			}
			ledger.Save(dataDir)

			writeJSON(w, swap)

		default:
			http.Error(w, `{"error":"unknown action"}`, http.StatusBadRequest)
		}
	}
}
