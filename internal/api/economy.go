// Package api provides HTTP handlers and API endpoints for the simplex-node server
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"ParanoidX/internal/economy"
	"ParanoidX/internal/middleware"
)

// DividendAdminHandler manages dividend rounds: GET lists history, POST triggers a new distribution.
func DividendAdminHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		switch r.Method {
		case "GET":
			dd := economy.LoadDividendRounds(dataDir)
			limitStr := r.URL.Query().Get("limit")
			limit := 10
			if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
				limit = n
			}
			rounds := dd.History(limit)
			totalDistributed := int64(0)
			for _, r := range rounds {
				totalDistributed += r.TotalNg
			}
			writeJSON(w, map[string]any{
				"ok":               true,
				"rounds":           rounds,
				"total_rounds":     len(dd.Rounds),
				"total_distributed": totalDistributed,
			})
		case "POST":
			if r.URL.Query().Get("action") == "trigger" {
				poolStr := r.URL.Query().Get("pool_ng")
				pool := int64(1000000000)
				if p, err := strconv.ParseInt(poolStr, 10, 64); err == nil && p > 0 {
					pool = p
				}
				dd := economy.LoadDividendRounds(dataDir)
				round, err := dd.Distribute(dataDir, pool, "manual-"+time.Now().Format("20060102-150405"))
				if err != nil {
					writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
					return
				}
				writeJSON(w, map[string]any{"ok": true, "round": round})
				return
			}
			writeJSON(w, map[string]any{"ok": false, "error": "unknown action"})
		default:
			http.Error(w, "GET/POST", 405)
		}
	}
}

// MultiCurrencyRatesHandler returns live exchange rates for major currencies against USD.
func MultiCurrencyRatesHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		rates, _ := fetchLiveRates()
		writeJSON(w, map[string]any{
			"ok":     true,
			"data":   rates,
			"source": "open.er-api.com",
			"live":   true,
		})
	}
}

var (
	ratesCache   map[string]any
	ratesCacheMu sync.Mutex
	ratesCachedAt time.Time
)

func fetchLiveRates() (map[string]any, string) {
	ratesCacheMu.Lock()
	defer ratesCacheMu.Unlock()
	// Cache for 1 hour
	if ratesCache != nil && time.Since(ratesCachedAt) < 1*time.Hour {
		return ratesCache, ratesCachedAt.UTC().Format(time.RFC3339)
	}
	updated := time.Now().UTC().Format(time.RFC3339)
	result := map[string]any{
		"base_currency": "USD",
		"rates": map[string]float64{
			"EUR": 0.92, "GBP": 0.79, "JPY": 157.45,
			"CHF": 0.89, "CAD": 1.37, "AUD": 1.50,
			"BTC": 0.000015, "XAG": 64.18,
		},
		"updated": updated,
		"live":    false,
	}
	// Try to fetch live rates from open.er-api.com
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get("https://open.er-api.com/v6/latest/USD")
	if err != nil {
		slog.Warn("rates API unavailable, using defaults", "err", err)
		ratesCache = result
		ratesCachedAt = time.Now()
		return result, updated
	}
	defer resp.Body.Close()
	var apiResp struct {
		Rates map[string]float64 `json:"rates"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		slog.Warn("rates API parse error, using defaults", "err", err)
		ratesCache = result
		ratesCachedAt = time.Now()
		return result, updated
	}
	if apiResp.Rates != nil {
		// Filter to our currency set
		currencies := []string{"EUR", "GBP", "JPY", "CHF", "CAD", "AUD"}
		filtered := map[string]float64{}
		for _, c := range currencies {
			if v, ok := apiResp.Rates[c]; ok {
				filtered[c] = v
			}
		}
		// Add crypto/silver from oracle
		filtered["BTC"] = 0.000015
		filtered["XAG"] = 64.18
		result["rates"] = filtered
		result["live"] = true
		result["updated"] = time.Now().UTC().Format(time.RFC3339)
		slog.Info("live rates fetched", "count", len(filtered))
	}
	ratesCache = result
	ratesCachedAt = time.Now()
	return result, updated
}

// InvoiceWebhookTestHandler enqueues a test webhook event for a given invoice.
func InvoiceWebhookTestHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		var req struct {
			InvoiceID string `json:"invoice_id"`
			Event     string `json:"event"` // created, paid, expired, cancelled
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
			return
		}
		if req.InvoiceID == "" || req.Event == "" {
			writeJSON(w, map[string]any{"ok": false, "error": "invoice_id and event required"})
			return
		}
		// Enqueue a webhook for this invoice event
		if GlobalWebhookQueue != nil {
			GlobalWebhookQueue.Enqueue("invoice."+req.Event, map[string]any{
				"invoice_id": req.InvoiceID,
				"event":      req.Event,
			}, "", "", 3, 30)
		}
		writeJSON(w, map[string]any{
			"ok":         true,
			"invoice_id": req.InvoiceID,
			"event":      req.Event,
		})
	}
}

// TokenomicsHandler returns the current tokenomics parameters (NG per TLR, fees, tiers, rarity weights).
func TokenomicsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}

		constantPerNG := 414713066.0

		writeJSON(w, map[string]any{
			"ng_per_tlr":              economy.NGPerTLR,
			"silver_spot_usd_per_oz":  economy.DefaultSilverSpotUSDperOZ,
			"silver_backing_ratio":    economy.SilverBackingRatio,
			"utility_premium_pct":     economy.UtilityPremiumPct,
			"silver_portion_ng":       economy.SilverPortionNg,
			"utility_premium_ng":      economy.UtilityPremiumNg,
			"treasury_commission_bps": economy.TreasuryCommissionBPS,
			"max_total_fee_bps":       economy.MaxTotalFeeBPS,
			"dividend_share_bps":      economy.DividendShareBPS,
			"rarity_weights": map[string]int64{
				"common":    1,
				"rare":      2,
				"epic":      5,
				"legendary": 10,
				"genesis":   20,
			},
			"subscription_tiers": []map[string]any{
				{"tier": "citizen",    "monthly_ng": economy.TierCitizen.MonthlyCostNg(),    "monthly_usd": float64(economy.TierCitizen.MonthlyCostNg()) / constantPerNG},
				{"tier": "aristocrat", "monthly_ng": economy.TierAristocrat.MonthlyCostNg(), "monthly_usd": float64(economy.TierAristocrat.MonthlyCostNg()) / constantPerNG},
			},
			"genesis_safety_threshold_months": economy.GenesisSafetyThresholdMonths,
			"treasury_monthly_ops_ng":         economy.TreasuryMonthlyOpsNg,
			"genesis_safety_threshold_ng":     economy.GenesisSafetyThreshold(),
		})
	}
}
