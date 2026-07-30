// Package api provides HTTP handlers and API endpoints for the simplex-node server
package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ParanoidX/internal/economy"
	"ParanoidX/internal/fileutil"
	"ParanoidX/internal/middleware"
)

var GlobalOracleRef *economy.SilverSpotOracle

func isRoyalNodePath(dataDir string) bool {
	b, err := os.ReadFile(filepath.Join(dataDir, "royal.enabled"))
	if err != nil {
		return false
	}
	s := strings.TrimSpace(string(b))
	return s != "" && s != "0"
}

func isEmergencyStop(dataDir string) bool {
	_, err := os.Stat(filepath.Join(dataDir, "royal.emergency_stop"))
	return err == nil
}

func checkEmergencyStop(w http.ResponseWriter, dataDir string) bool {
	if isEmergencyStop(dataDir) {
		writeJSON(w, map[string]any{"ok": false, "error": "emergency stop active"})
		return true
	}
	return false
}

// ── Treasury State ──────────────────────────────────────────────────────────

func RoyalTreasuryStateHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) { return }
		if !isRoyalNodePath(dataDir) { http.Error(w, "forbidden", 403); return }

		reserveNg := int64(0)
		if b, _ := os.ReadFile(filepath.Join(dataDir, "silver_reserve_ng.txt")); b != nil {
			fmt.Sscanf(string(b), "%d", &reserveNg)
		}
		ledger := economy.LoadLedger(dataDir)
		supplyNg := ledger.TotalSupply
		accounts := len(ledger.Accounts)

		banknotes, _ := economy.LoadBanknotesV2(dataDir)
		activeBN := 0
		for _, bn := range banknotes {
			if bn.Status == "active" {
				activeBN++
			}
		}

		rwas := []map[string]any{}
		fileutil.ReadJSON(filepath.Join(dataDir, "rwa_registry.json"), &rwas)

		cfg := economy.DefaultTreasuryConfig()
		tier := economy.DetectTier(reserveNg, cfg)

		oraclePrice := economy.DefaultSilverSpotUSDperOZ
		if GlobalOracleRef != nil {
			oraclePrice = GlobalOracleRef.GetPrice()
		}

		writeJSON(w, map[string]any{
			"ok":                true,
			"reserve_ng":        reserveNg,
			"reserve_tlr":       reserveNg / economy.NGPerTLR,
			"supply_ng":         supplyNg,
			"supply_tlr":        supplyNg / economy.NGPerTLR,
			"accounts":          accounts,
			"active_banknotes":  activeBN,
			"total_banknotes":   len(banknotes),
			"rwa_count":         len(rwas),
			"tier":              tier.String(),
			"monthly_ops_ng":    cfg.MonthlyOpsNg,
			"silver_spot_usd":   oraclePrice,
			"is_royal":          true,
		})
	}
}

// ── Reserve ────────────────────────────────────────────────────────────────

func RoyalReserveHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) { return }
		if !isRoyalNodePath(dataDir) { http.Error(w, "forbidden", 403); return }

		switch r.Method {
		case "GET":
			reserveNg := int64(0)
			if b, _ := os.ReadFile(filepath.Join(dataDir, "silver_reserve_ng.txt")); b != nil {
				fmt.Sscanf(string(b), "%d", &reserveNg)
			}
			ledger := economy.LoadLedger(dataDir)
			supplyNg := ledger.TotalSupply
			ratio := 1.0
			if supplyNg > 0 {
				ratio = float64(reserveNg) / float64(supplyNg)
			}
			writeJSON(w, map[string]any{
				"reserve_ng":      reserveNg,
				"reserve_tlr":     reserveNg / economy.NGPerTLR,
				"supply_ng":       supplyNg,
				"supply_tlr":      supplyNg / economy.NGPerTLR,
				"coverage_ratio":  fmt.Sprintf("%.4f", ratio),
				"coverage_pct":    ratio * 100,
			})

		case "POST":
			var req struct {
				AmountNg int64  `json:"amount_ng"`
				Action   string `json:"action"` // "credit" or "debit"
				Reason   string `json:"reason"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
				return
			}
			if req.AmountNg <= 0 {
				writeJSON(w, map[string]any{"ok": false, "error": "amount_ng must be positive"})
				return
			}
			reservePath := filepath.Join(dataDir, "silver_reserve_ng.txt")
			reserveNg := int64(0)
			if b, _ := os.ReadFile(reservePath); b != nil {
				fmt.Sscanf(string(b), "%d", &reserveNg)
			}

			switch req.Action {
			case "credit":
				reserveNg += req.AmountNg
				slog.Info("royal reserve credit", "amount_ng", req.AmountNg, "reason", req.Reason)
			case "debit":
				if req.AmountNg > reserveNg {
					writeJSON(w, map[string]any{"ok": false, "error": "insufficient reserve"})
					return
				}
				reserveNg -= req.AmountNg
				slog.Info("royal reserve debit", "amount_ng", req.AmountNg, "reason", req.Reason)
			default:
				writeJSON(w, map[string]any{"ok": false, "error": "action must be 'credit' or 'debit'"})
				return
			}

			os.WriteFile(reservePath, []byte(fmt.Sprintf("%d\n", reserveNg)), 0600)
			appendRoyalAudit(dataDir, "reserve."+req.Action, map[string]any{
				"amount_ng": req.AmountNg, "reason": req.Reason, "new_reserve": reserveNg,
			})
			writeJSON(w, map[string]any{"ok": true, "reserve_ng": reserveNg})

		default:
			http.Error(w, "GET/POST only", 405)
		}
	}
}

// ── Oracle ─────────────────────────────────────────────────────────────────

func RoyalOracleHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) { return }
		if !isRoyalNodePath(dataDir) { http.Error(w, "forbidden", 403); return }

		switch r.Method {
		case "GET":
			oracle := economy.LoadOracle(dataDir)
			history := oracle.GetHistory()
			writeJSON(w, map[string]any{
				"current_price": oracle.GetPrice(),
				"last_updated":  oracle.GetLastUpdated(),
				"history":       history,
				"default":       economy.DefaultSilverSpotUSDperOZ,
			})

		case "POST":
			var req struct {
				Price float64 `json:"price"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
				return
			}
			oracle := economy.LoadOracle(dataDir)
			if err := oracle.UpdatePrice(req.Price); err != nil {
				writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
				return
			}
			oracle.Save(dataDir)
			if GlobalOracleRef != nil {
				GlobalOracleRef.UpdatePrice(req.Price)
			}
			appendRoyalAudit(dataDir, "oracle.update", map[string]any{"price": req.Price})
			writeJSON(w, map[string]any{
				"ok": true, "current_price": oracle.GetPrice(), "updated_at": time.Now().UTC().Format(time.RFC3339),
			})

		default:
			http.Error(w, "GET/POST only", 405)
		}
	}
}

// ── Deflation ──────────────────────────────────────────────────────────────

func RoyalDeflationHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) { return }
		if !isRoyalNodePath(dataDir) { http.Error(w, "forbidden", 403); return }

		dm := economy.LoadDeflation(dataDir)

		switch r.Method {
		case "GET":
			reserveNg := int64(0)
			if b, _ := os.ReadFile(filepath.Join(dataDir, "silver_reserve_ng.txt")); b != nil {
				fmt.Sscanf(string(b), "%d", &reserveNg)
			}
			cfg := economy.DefaultTreasuryConfig()
			tier := economy.DetectTier(reserveNg, cfg)
			writeJSON(w, map[string]any{
				"total_burned_ng": dm.TotalBurnedNg,
				"last_burn_round": dm.LastBurnRound,
				"threshold_basis": dm.ThresholdBasis,
				"current_tier":    tier.String(),
				"reserve_ng":      reserveNg,
			})

		case "POST":
			reserveNg := int64(0)
			if b, _ := os.ReadFile(filepath.Join(dataDir, "silver_reserve_ng.txt")); b != nil {
				fmt.Sscanf(string(b), "%d", &reserveNg)
			}
			cfg := economy.DefaultTreasuryConfig()
			amt, err := dm.Burn(dataDir, reserveNg, cfg)
			if err != nil {
				writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
				return
			}
			appendRoyalAudit(dataDir, "deflation.burn", map[string]any{"amount_ng": amt})
			writeJSON(w, map[string]any{
				"ok": true, "burned_ng": amt, "total_burned_ng": dm.TotalBurnedNg, "last_burn_round": dm.LastBurnRound,
			})

		default:
			http.Error(w, "GET/POST only", 405)
		}
	}
}

// ── Auto-Mint ──────────────────────────────────────────────────────────────

func RoyalAutoMintHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) { return }
		if !isRoyalNodePath(dataDir) { http.Error(w, "forbidden", 403); return }

		schedule := economy.LoadOrCreateSchedule(dataDir)

		switch r.Method {
		case "GET":
			reserveNg := int64(0)
			if b, _ := os.ReadFile(filepath.Join(dataDir, "silver_reserve_ng.txt")); b != nil {
				fmt.Sscanf(string(b), "%d", &reserveNg)
			}
			cfg := economy.DefaultTreasuryConfig()
			current := economy.DetectTier(reserveNg, cfg)
			writeJSON(w, map[string]any{
				"current_tier":   current.String(),
				"reserve_ng":     reserveNg,
				"monthly_ops_ng": cfg.MonthlyOpsNg,
				"last_tier":      schedule.LastTier.String(),
				"triggered_at":   schedule.TriggeredAt,
				"sets_total":     len(schedule.Sets),
			})

		case "POST":
			reserveNg := int64(0)
			if b, _ := os.ReadFile(filepath.Join(dataDir, "silver_reserve_ng.txt")); b != nil {
				fmt.Sscanf(string(b), "%d", &reserveNg)
			}
			cfg := economy.DefaultTreasuryConfig()
			notes, triggers, err := schedule.CheckAndMint(reserveNg, cfg, dataDir)
			if err != nil {
				writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
				return
			}
			if len(notes) > 0 {
				economy.MergeMintedBanknotes(dataDir, notes)
				schedule.Save(dataDir)
			}
			appendRoyalAudit(dataDir, "auto-mint.check", map[string]any{
				"triggers": triggers, "minted": len(notes),
			})
			writeJSON(w, map[string]any{
				"ok": true, "triggers": triggers, "banknotes_minted": len(notes),
			})

		default:
			http.Error(w, "GET/POST only", 405)
		}
	}
}

// ── Dividend ───────────────────────────────────────────────────────────────

func RoyalDividendHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) { return }
		if !isRoyalNodePath(dataDir) { http.Error(w, "forbidden", 403); return }

		dd := economy.LoadDividendRounds(dataDir)

		switch r.Method {
		case "GET":
			limitStr := r.URL.Query().Get("limit")
			limit := 10
			if limitStr != "" {
				var n int
				if _, err := fmt.Sscanf(limitStr, "%d", &n); err == nil && n > 0 && n <= 100 {
					limit = n
				}
			}
			rounds := dd.History(limit)
			totalDistributed := int64(0)
			for _, r := range rounds {
				totalDistributed += r.TotalNg
			}
			writeJSON(w, map[string]any{
				"ok": true, "rounds": rounds, "total_rounds": len(dd.Rounds),
				"total_distributed": totalDistributed,
			})

		case "POST":
			action := r.URL.Query().Get("action")
			if action == "trigger" {
				poolStr := r.URL.Query().Get("pool_ng")
				pool := int64(1000000000)
				if poolStr != "" {
					var p int
					if _, err := fmt.Sscanf(poolStr, "%d", &p); err == nil && p > 0 {
						pool = int64(p)
					}
				}
				round, err := dd.Distribute(dataDir, pool, "royal-"+time.Now().Format("20060102-150405"))
				if err != nil {
					writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
					return
				}
				appendRoyalAudit(dataDir, "dividend.trigger", map[string]any{"pool_ng": pool, "round_id": round.RoundID})
				writeJSON(w, map[string]any{"ok": true, "round": round})
				return
			}
			writeJSON(w, map[string]any{"ok": false, "error": "unknown action (trigger)"})

		default:
			http.Error(w, "GET/POST only", 405)
		}
	}
}

// ── Mint Silver Asset ──────────────────────────────────────────────────────

func RoyalMintHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) { return }
		if !isRoyalNodePath(dataDir) { http.Error(w, "forbidden", 403); return }
		if r.Method != "POST" { http.Error(w, "POST required", 405); return }

		var req struct {
			Holder   string `json:"holder"`
			AmountNg int64  `json:"amount_ng"`
			Reason   string `json:"reason"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
			return
		}
		if req.Holder == "" || req.AmountNg <= 0 {
			writeJSON(w, map[string]any{"ok": false, "error": "holder and amount_ng required"})
			return
		}

		reserveNg := int64(0)
		if b, _ := os.ReadFile(filepath.Join(dataDir, "silver_reserve_ng.txt")); b != nil {
			fmt.Sscanf(string(b), "%d", &reserveNg)
		}
		if req.AmountNg > reserveNg {
			writeJSON(w, map[string]any{"ok": false, "error": "insufficient silver reserve"})
			return
		}

		assetFile := filepath.Join(dataDir, "silver_assets.json")
		var assets []map[string]any
		if b, _ := os.ReadFile(assetFile); b != nil {
			json.Unmarshal(b, &assets)
		}
		if assets == nil {
			assets = make([]map[string]any, 0)
		}
		asset := map[string]any{
			"id":         fmt.Sprintf("silver-%d", time.Now().UnixNano()),
			"holder":     req.Holder,
			"amount_ng":  req.AmountNg,
			"created_at": time.Now().UTC().Format(time.RFC3339),
			"status":     "active",
		}
		assets = append(assets, asset)
		fileutil.WriteJSON(assetFile, assets)

		newReserve := reserveNg - req.AmountNg
		os.WriteFile(filepath.Join(dataDir, "silver_reserve_ng.txt"), []byte(fmt.Sprintf("%d\n", newReserve)), 0600)
		ledger := economy.LoadLedger(dataDir)
		ledger.EnsureAccount(req.Holder)
		ledger.Mint(req.Holder, req.AmountNg)
		ledger.Save(dataDir)

		appendRoyalAudit(dataDir, "mint.asset", map[string]any{
			"holder": req.Holder, "amount_ng": req.AmountNg, "asset_id": asset["id"], "reason": req.Reason,
		})
		writeJSON(w, map[string]any{
			"ok": true, "asset": asset, "reserve_ng": newReserve,
		})
	}
}

// ── Burn Silver Asset ──────────────────────────────────────────────────────

func RoyalBurnHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) { return }
		if !isRoyalNodePath(dataDir) { http.Error(w, "forbidden", 403); return }
		if r.Method != "POST" { http.Error(w, "POST required", 405); return }

		var req struct {
			AssetID string `json:"asset_id"`
			Holder  string `json:"holder"`
			Reason  string `json:"reason"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
			return
		}
		assetFile := filepath.Join(dataDir, "silver_assets.json")
		assetData, err := os.ReadFile(assetFile)
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "no assets found"})
			return
		}
		var assets []map[string]any
		json.Unmarshal(assetData, &assets)
		var found bool
		var amountNg int64
		remaining := make([]map[string]any, 0, len(assets))
		for _, a := range assets {
			if a["id"] == req.AssetID && a["holder"] == req.Holder && a["status"] == "active" {
				found = true
				a["status"] = "burned"
				a["burned_at"] = time.Now().UTC().Format(time.RFC3339)
				switch v := a["amount_ng"].(type) {
				case int64: amountNg = v
				case float64: amountNg = int64(v)
				case json.Number: amountNg, _ = v.Int64()
				}
			}
			remaining = append(remaining, a)
		}
		if !found {
			writeJSON(w, map[string]any{"ok": false, "error": "asset not found"})
			return
		}
		fileutil.WriteJSON(assetFile, remaining)

		reserveNg := int64(0)
		if b, _ := os.ReadFile(filepath.Join(dataDir, "silver_reserve_ng.txt")); b != nil {
			fmt.Sscanf(string(b), "%d", &reserveNg)
		}
		newReserve := reserveNg + amountNg
		os.WriteFile(filepath.Join(dataDir, "silver_reserve_ng.txt"), []byte(fmt.Sprintf("%d\n", newReserve)), 0600)
		ledger := economy.LoadLedger(dataDir)
		ledger.Transfer(req.Holder, "reserve", amountNg)
		ledger.Save(dataDir)

		appendRoyalAudit(dataDir, "burn.asset", map[string]any{
			"asset_id": req.AssetID, "holder": req.Holder, "amount_ng": amountNg, "reason": req.Reason,
		})
		writeJSON(w, map[string]any{
			"ok": true, "burned": req.AssetID, "returned_ng": amountNg, "reserve_ng": newReserve,
		})
	}
}

// ── Silver Assets List ─────────────────────────────────────────────────────

func RoyalSilverAssetsHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) { return }
		if !isRoyalNodePath(dataDir) { http.Error(w, "forbidden", 403); return }

		assetFile := filepath.Join(dataDir, "silver_assets.json")
		var assets []map[string]any
		if b, _ := os.ReadFile(assetFile); b != nil {
			json.Unmarshal(b, &assets)
		}
		if assets == nil {
			assets = make([]map[string]any, 0)
		}
		writeJSON(w, map[string]any{"ok": true, "assets": assets, "count": len(assets)})
	}
}

// ── Banknotes List ─────────────────────────────────────────────────────────

func RoyalBanknotesHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) { return }
		if !isRoyalNodePath(dataDir) { http.Error(w, "forbidden", 403); return }

		banknotes, err := economy.LoadBanknotesV2(dataDir)
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error(), "banknotes": []any{}})
			return
		}
		if banknotes == nil {
			banknotes = []economy.BanknoteV2{}
		}
		active := 0
		for _, bn := range banknotes {
			if bn.Status == "active" {
				active++
			}
		}
		writeJSON(w, map[string]any{
			"ok": true, "banknotes": banknotes, "total": len(banknotes), "active": active,
		})
	}
}

// ── Proof of Reserve ───────────────────────────────────────────────────────

func RoyalProofOfReserveHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) { return }
		if !isRoyalNodePath(dataDir) { http.Error(w, "forbidden", 403); return }

		reserveNg := int64(0)
		if b, _ := os.ReadFile(filepath.Join(dataDir, "silver_reserve_ng.txt")); b != nil {
			fmt.Sscanf(string(b), "%d", &reserveNg)
		}

		ledger := economy.LoadLedger(dataDir)
		totalSupply := ledger.TotalSupply

		banknotes, _ := economy.LoadBanknotesV2(dataDir)
		totalLiabilities := int64(0)
		for _, b := range banknotes {
			if b.Status == "active" {
				totalLiabilities += b.FrozenNg
			}
		}

		solvent := true
		ratio := 0.0
		if totalLiabilities > 0 {
			ratio = float64(reserveNg) / float64(totalLiabilities)
			solvent = ratio >= 1.0
		}

		writeJSON(w, map[string]any{
			"ok":                true,
			"reserve_ng":        reserveNg,
			"reserve_g":         float64(reserveNg) / 1e9,
			"liabilities_ng":    totalLiabilities,
			"liabilities_g":     float64(totalLiabilities) / 1e9,
			"coverage_ratio":    fmt.Sprintf("%.4f", ratio),
			"solvent":           solvent,
			"active_banknotes":  len(banknotes),
			"total_supply_ng":   totalSupply,
			"backing_pct":       ratio * 100,
		})
	}
}

// ── Multi-Currency Rates ──────────────────────────────────────────────────

func RoyalRatesHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) { return }
		// Royal read-only, no write needed
		rates := map[string]any{
			"base_currency": "USD",
			"rates": map[string]float64{
				"EUR": 0.92, "GBP": 0.79, "JPY": 157.45, "CHF": 0.89,
				"CAD": 1.37, "AUD": 1.50, "BTC": 0.000015, "XAG": 64.18,
			},
			"updated": time.Now().UTC().Format(time.RFC3339),
		}
		writeJSON(w, map[string]any{"ok": true, "data": rates})
	}
}

// ── Tokenomics ────────────────────────────────────────────────────────────

func RoyalTokenomicsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) { return }
		constantPerNG := 414713066.0
		writeJSON(w, map[string]any{
			"ng_per_tlr":                  economy.NGPerTLR,
			"silver_spot_usd_per_oz":     economy.DefaultSilverSpotUSDperOZ,
			"silver_backing_ratio":        economy.SilverBackingRatio,
			"treasury_commission_bps":     economy.TreasuryCommissionBPS,
			"dividend_share_bps":         economy.DividendShareBPS,
			"max_total_fee_bps":          economy.MaxTotalFeeBPS,
			"rarity_weights": map[string]int64{
				"common": 1, "rare": 2, "epic": 5, "legendary": 10, "genesis": 20,
			},
			"subscription_tiers": []map[string]any{
				{"tier": "citizen",    "monthly_ng": economy.TierCitizen.MonthlyCostNg(),    "monthly_usd": float64(economy.TierCitizen.MonthlyCostNg()) / constantPerNG},
				{"tier": "aristocrat", "monthly_ng": economy.TierAristocrat.MonthlyCostNg(), "monthly_usd": float64(economy.TierAristocrat.MonthlyCostNg()) / constantPerNG},
			},
			"genesis_safety_threshold_months": economy.GenesisSafetyThresholdMonths,
			"treasury_monthly_ops_ng":         economy.TreasuryMonthlyOpsNg,
		})
	}
}

// ── Treasury Forecast ─────────────────────────────────────────────────────

func RoyalForecastHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) { return }
		if !isRoyalNodePath(dataDir) { http.Error(w, "forbidden", 403); return }

		reserveNg := int64(0)
		if b, _ := os.ReadFile(filepath.Join(dataDir, "silver_reserve_ng.txt")); b != nil {
			fmt.Sscanf(string(b), "%d", &reserveNg)
		}
		ledger := economy.LoadLedger(dataDir)
		supplyNg := ledger.TotalSupply
		cfg := economy.DefaultTreasuryConfig()
		tier := economy.DetectTier(reserveNg, cfg)

		monthsUntilDepleted := int64(0)
		if cfg.MonthlyOpsNg > 0 {
			monthsUntilDepleted = reserveNg / cfg.MonthlyOpsNg
		}

		oraclePrice := economy.DefaultSilverSpotUSDperOZ
		if GlobalOracleRef != nil {
			oraclePrice = GlobalOracleRef.GetPrice()
		}
		totalUsdValue := float64(reserveNg) / float64(economy.NGPerTLR) * oraclePrice

		health := "healthy"
		if monthsUntilDepleted < 3 {
			health = "critical"
		} else if monthsUntilDepleted < 6 {
			health = "warn"
		}

		writeJSON(w, map[string]any{
			"ok":                    true,
			"reserve_ng":            reserveNg,
			"supply_ng":             supplyNg,
			"tier":                  tier.String(),
			"monthly_ops_ng":        cfg.MonthlyOpsNg,
			"months_until_depleted": monthsUntilDepleted,
			"total_usd_value":       totalUsdValue,
			"silver_spot_usd":       oraclePrice,
			"health":                health,
			"recommendations":       generateTreasuryRecommendations(tier, monthsUntilDepleted),
		})
	}
}

func generateTreasuryRecommendations(tier economy.TreasuryTier, monthsLeft int64) []string {
	recs := []string{}
	switch tier {
	case 0:
		recs = append(recs, "Reserve critically low — reduce monthly ops spending")
		recs = append(recs, "Consider subscription drive to boost revenue")
		recs = append(recs, "Suspend non-essential treasury operations")
	case 1:
		recs = append(recs, "Reserve adequate — maintain current ops level")
		if monthsLeft < 6 {
			recs = append(recs, "Increase reserve buffer to 6+ months")
		}
	case 2:
		recs = append(recs, "Reserve healthy — consider auto-mint triggers")
		recs = append(recs, "Allocate surplus to dividend pool")
	case 3:
		recs = append(recs, "Reserve very fat — trigger deflation burn if appropriate")
		recs = append(recs, "Consider expanding operations or reducing subscription costs")
	}
	return recs
}

// ── Royal Audit Trail ─────────────────────────────────────────────────────

func appendRoyalAudit(dataDir string, action string, details map[string]any) {
	auditFile := filepath.Join(dataDir, "royal_audit.jsonl")
	f, err := os.OpenFile(auditFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		slog.Error("royal audit append", "error", err)
		return
	}
	defer f.Close()
	entry := map[string]any{
		"action":    action,
		"details":   details,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	enc := json.NewEncoder(f)
	enc.Encode(entry)
}

// ── Royal Audit Log Handler ────────────────────────────────────────────────

func RoyalAuditLogHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) { return }
		if !isRoyalNodePath(dataDir) { http.Error(w, "forbidden", 403); return }

		limit := 100
		if l := r.URL.Query().Get("limit"); l != "" {
			fmt.Sscanf(l, "%d", &limit)
		}
		auditFile := filepath.Join(dataDir, "royal_audit.jsonl")
		b, err := os.ReadFile(auditFile)
		if err != nil {
			writeJSON(w, map[string]any{"ok": true, "entries": []any{}})
			return
		}
		lines := strings.Split(strings.TrimSpace(string(b)), "\n")
		var entries []map[string]any
		for i := len(lines) - 1; i >= 0 && len(entries) < limit; i-- {
			var entry map[string]any
			if json.Unmarshal([]byte(lines[i]), &entry) == nil {
				entries = append(entries, entry)
			}
		}
		writeJSON(w, map[string]any{"ok": true, "entries": entries, "count": len(entries)})
	}
}
