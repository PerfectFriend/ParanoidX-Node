// Package api provides HTTP handlers and API endpoints for the simplex-node server
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"ParanoidX/internal/economy"
	"ParanoidX/internal/middleware"
)


// GenesisIcoHandler handles the GenesisIcoHandler HTTP request.
func GenesisIcoHandler(dataDir string) http.HandlerFunc {
	ico := economy.LoadGenesisICO(dataDir)

	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}

		action := r.URL.Query().Get("action")

		switch r.Method {
		case "GET":
			switch action {
			case "status":
				status := ico.Status()
				writeJSON(w, status)

			case "holder":
				holder := r.URL.Query().Get("holder")
				if holder == "" {
					http.Error(w, "holder required", http.StatusBadRequest)
					return
				}
				tokens := ico.HolderTokens(holder)
				writeJSON(w, map[string]any{
					"holder": holder,
					"tokens": tokens,
				})

			default:
				writeJSON(w, ico.Status())
			}

		case "POST":
			switch action {
			case "buy":
				var req struct {
					Buyer  string `json:"buyer"`
					Amount int64  `json:"amount"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					http.Error(w, "invalid json", http.StatusBadRequest)
					return
				}
				if req.Buyer == "" || req.Amount <= 0 {
					http.Error(w, "buyer and amount required", http.StatusBadRequest)
					return
				}
				ledger := economy.LoadLedger(dataDir)
				tokens, err := ico.BuyTokens(dataDir, req.Buyer, req.Amount, ledger)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				ledger.Save(dataDir)
				ico.Save(dataDir)
				writeJSON(w, map[string]any{
					"buyer":       req.Buyer,
					"tokens":      len(tokens),
					"total_ng":    req.Amount,
					"token_ids":   tokens,
				})

			case "start":
				err := ico.StartICO(dataDir)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				ico.Save(dataDir)
				writeJSON(w, map[string]any{
					"status":  ico.Status(),
					"message": "ICO started",
				})

			default:
				http.Error(w, "unknown action", http.StatusBadRequest)
			}

		default:
			http.Error(w, "GET or POST only", 400)
		}
	}
}


// GenesisLockHandler handles the GenesisLockHandler HTTP request.
func GenesisLockHandler(dataDir string) http.HandlerFunc {
	lock := economy.LoadGenesisLock(dataDir)

	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}

		switch r.Method {
		case "GET":
			reserve := int64(0)
			if b, err := os.ReadFile(filepath.Join(dataDir, "silver_reserve_ng.txt")); err == nil {
				fmt.Sscanf(string(b), "%d", &reserve)
			}
			summary := lock.Summary(reserve)
			threshold := economy.GenesisSafetyThreshold()
			surplusOk := reserve >= 12*threshold
			unlocked := lock.Unlocked || surplusOk

			writeJSON(w, map[string]any{
				"frozen":                  summary.Frozen,
				"genesis_card_count":      summary.GenesisCardCount,
				"frozen_dividend_pool_ng": summary.FrozenDividendPoolNg,
				"safety_threshold_ng":     summary.SafetyThresholdNg,
				"treasury_surplus_ng":     summary.TreasuryNg,
				"surplus_ng":              summary.SurplusNg,
				"progress_pct":            summary.ProgressPct,
				"unlock_condition_met":    surplusOk,
				"unlocked":                unlocked,
				"twelve_times_threshold":  12 * threshold,
				"total_deflation_ng":      summary.TotalDeflationNg,
				"last_accrual_round":      summary.LastAccrualRound,
			})

		case "POST":
			action := r.URL.Query().Get("action")
			switch action {
			case "accrue":
				var req struct {
					AmountNg int64  `json:"amount_ng"`
					RoundID  string `json:"round_id"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					http.Error(w, "invalid json", http.StatusBadRequest)
					return
				}
				if req.AmountNg <= 0 {
					http.Error(w, "amount_ng > 0 required", http.StatusBadRequest)
					return
				}
				if req.RoundID == "" {
					req.RoundID = time.Now().UTC().Format(time.RFC3339)
				}
				frozen, err := lock.AccrueFrozenDividend(dataDir, req.AmountNg, req.RoundID)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				lock.Save(dataDir)
				writeJSON(w, map[string]any{
					"frozen_dividend_pool_ng": lock.FrozenDividendPoolNg,
					"accrued":                 frozen,
				})

			case "unlock":
				if lock.Unlocked {
					writeJSON(w, map[string]any{"unlocked": true, "note": "already unlocked"})
					return
				}
				reserve := int64(0)
				if b, err := os.ReadFile(filepath.Join(dataDir, "silver_reserve_ng.txt")); err == nil {
					fmt.Sscanf(string(b), "%d", &reserve)
				}
				threshold := economy.GenesisSafetyThreshold()
				if reserve < 12*threshold {
					http.Error(w, "unlock condition not met: treasury surplus < 12x safety threshold", http.StatusBadRequest)
					return
				}
				lock.CheckSurplus(reserve)
				lock.Save(dataDir)
				writeJSON(w, map[string]any{
					"unlocked":          true,
					"frozen_pool_ng":    lock.FrozenDividendPoolNg,
					"note":              "genesis lock released, dividends claimable",
				})

			case "distribute":
				distributed, err := lock.DistributeFrozenDividends(dataDir)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				lock.Save(dataDir)
				writeJSON(w, map[string]any{
					"distributed_ng": distributed,
					"remaining_ng":   lock.FrozenDividendPoolNg,
				})

			default:
				http.Error(w, "unknown action", http.StatusBadRequest)
			}

		default:
			http.Error(w, "GET or POST only", 400)
		}
	}
}
