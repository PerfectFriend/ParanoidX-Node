// Package treasury implements treasury round management
package treasury

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"ParanoidX/internal/fileutil"
	"ParanoidX/internal/middleware"
)

// InitSilverRoundHandler handles POST /api/treasury/init-silver-round.
func InitSilverRoundHandler(dataDir, announceDir string, billRecorder func(price int64, action, ref string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !middleware.IsLocalOrOnionAccess(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		usdtStr := r.URL.Query().Get("usdt")
		usdt := 1000.0
		if usdtStr != "" {
			fmt.Sscanf(usdtStr, "%f", &usdt)
		}

		result, err := ExecuteRound(RoundParams{
			DataDir:      dataDir,
			USDT:         usdt,
			AnnounceDir:  announceDir,
		})
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		writeJSON(w, map[string]any{
			"status":            "round processed",
			"usdt_in":           result.USDTIn,
			"new_silver_ng":     result.NewSilverNg,
			"treasury_share_ng": result.TreasuryShareNg,
			"current_reserve_ng": result.CurrentReserve,
			"dividends":         result.Dividends,
		})
	}
}

// USDTDepositsHandler returns the current USDT deposit state from the monitor.
func USDTDepositsHandler(tronMon *Monitor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !middleware.IsLocalOrOnionAccess(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		tronMon.PollNow()
		totalUsdt, count := tronMon.Stats()
		simLog := tronMon.ReadLog()
		deposits := tronMon.AllDeposits()
		writeJSON(w, map[string]any{
			"total_usdt":      totalUsdt,
			"deposit_count":   count,
			"recent_deposits": deposits,
			"sim_log":         simLog,
		})
	}
}

// ProofOfReserveHandler returns the current proof of reserve.
func ProofOfReserveHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		por := GetProofOfReserve(dataDir)
		writeJSON(w, por)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// AutoRoundHandler triggers a silver round if the monitor's total USDT exceeds the threshold.
func AutoRoundHandler(dataDir, announceDir string, tronMon *Monitor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !middleware.IsLocalOrOnionAccess(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		totalUsdt, _ := tronMon.Stats()
		threshold := AutoRoundInitialThreshold
		if t := r.URL.Query().Get("threshold"); t != "" {
			fmt.Sscanf(t, "%f", &threshold)
		}
		if totalUsdt < threshold {
			writeJSON(w, map[string]any{
				"ok": false, "total_usdt": totalUsdt, "threshold": threshold,
				"note": fmt.Sprintf("need %.2f more", threshold-totalUsdt),
			})
			return
		}
		result, err := ExecuteRound(RoundParams{
			DataDir:      dataDir,
			USDT:         totalUsdt,
			AnnounceDir:  announceDir,
		})
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		tronMon.PollNow()
		writeJSON(w, map[string]any{"ok": true, "result": result})
	}
}

// SimulateDepositHandler simulates a USDT deposit (for testing).
func SimulateDepositHandler(dataDir string, tronMon *Monitor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !middleware.IsLocalOrOnionAccess(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		amt := 100.0
		if a := r.URL.Query().Get("amount"); a != "" {
			fmt.Sscanf(a, "%f", &amt)
		}
		from := r.URL.Query().Get("from")
		if from == "" {
			from = "TSimulatedFunder"
		}
		logFile := filepath.Join(dataDir, "treasury_usdt.log")
		if f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
			defer f.Close()
			fmt.Fprintf(f, "SIM %s: %.2f USDT from %s\n", time.Now().Format(time.RFC3339), amt, from)
		}
		slog.Info("simulated USDT deposit", "amount", amt, "from", from)
		writeJSON(w, map[string]any{"ok": true, "simulated_usdt": amt, "from": from})
	}
}

// RegisterBanknoteHandler handles banknote registration.
func RegisterBanknoteHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !middleware.IsLocalOrOnionAccess(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		defer r.Body.Close()
		var req struct {
			Serial string  `json:"serial"`
			Denom  float64 `json:"denomination_tlr"`
			Holder string  `json:"holder"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad json", 400)
			return
		}
		if req.Serial == "" || req.Denom <= 0 {
			http.Error(w, "serial and denomination_tlr required", 400)
			return
		}
		regFile := filepath.Join(dataDir, "banknotes_registry.json")
		var holders []map[string]any
		if b, err := os.ReadFile(regFile); err == nil {
			json.Unmarshal(b, &holders)
		}
		holders = append(holders, map[string]any{
			"serial":            req.Serial,
			"denomination_tlr":  req.Denom,
			"holder":            req.Holder,
			"accrued_ng":        0,
		})
		fileutil.WriteJSON(regFile, holders)
		writeJSON(w, map[string]any{"ok": true, "serial": req.Serial})
	}
}
