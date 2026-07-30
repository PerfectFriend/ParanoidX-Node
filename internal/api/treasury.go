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

func readTrim(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func isRoyalNode(dataDir string) bool {
	b, err := os.ReadFile(filepath.Join(dataDir, "royal.enabled"))
	if err != nil {
		return false
	}
	s := string(b)
	return s != "" && s != "0"
}


// ProofOfReserveHandler handles the ProofOfReserveHandler HTTP request.
func ProofOfReserveHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}

		reserve := int64(0)
		if b, err := os.ReadFile(filepath.Join(dataDir, "silver_reserve_ng.txt")); err == nil {
			fmt.Sscanf(string(b), "%d", &reserve)
		}

		banknotes, _ := economy.LoadBanknotesV2(dataDir)
		totalLiabilities := int64(0)
		activeBanknotes := 0
		for _, b := range banknotes {
			if b.Status == "active" || b.Status == "" {
				totalLiabilities += b.FrozenNg
				activeBanknotes++
			}
		}

		solvent := true
		ratio := 0.0
		if totalLiabilities > 0 {
			ratio = float64(reserve) / float64(totalLiabilities)
			solvent = ratio >= 1.0
		}
		writeJSON(w, map[string]any{
			"reserve_ng":          reserve,
			"reserve_g":           float64(reserve) / 1e9,
			"liabilities_ng":      totalLiabilities,
			"liabilities_g":       float64(totalLiabilities) / 1e9,
			"coverage_ratio":      fmt.Sprintf("%.4f", ratio),
			"solvent":             solvent,
			"active_banknotes":    activeBanknotes,
			"total_banknotes":     len(banknotes),
		})
	}
}


// TreasuryStateHandler handles the TreasuryStateHandler HTTP request.
func TreasuryStateHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !middleware.IsLocalOrOnionAccess(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		reserve := int64(0)
		if b, err := os.ReadFile(filepath.Join(dataDir, "silver_reserve_ng.txt")); err == nil {
			fmt.Sscanf(string(b), "%d", &reserve)
		}
		treasuryCumulative := int64(float64(reserve) * 0.20)
		rounds := ""
		if b, err := os.ReadFile(filepath.Join(dataDir, "silver_rounds.log")); err == nil {
			lines := strings.Split(strings.TrimSpace(string(b)), "\n")
			if len(lines) > 5 {
				rounds = strings.Join(lines[len(lines)-5:], "\n")
			} else {
				rounds = string(b)
			}
		}
		bnk := []map[string]any{}
		fileutil.ReadJSON(filepath.Join(dataDir, "banknotes_registry.json"), &bnk)
		rwas := []map[string]any{}
		fileutil.ReadJSON(filepath.Join(dataDir, "rwa_registry.json"), &rwas)
		writeJSON(w, map[string]any{
			"current_reserve_ng":  reserve,
			"treasury_share_ng":   treasuryCumulative,
			"recent_rounds":       rounds,
			"banknotes":           bnk,
			"rwa":                 rwas,
			"is_royal":            isRoyalNode(dataDir),
		})
	}
}


// SubscriptionHandler handles the SubscriptionHandler HTTP request.
func SubscriptionHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}

		sm := economy.LoadSubscriptionManager(dataDir)
		defer sm.Save(dataDir)

		switch r.Method {
		case "GET":
			pubkey := r.URL.Query().Get("pubkey")
			if pubkey == "" {
				http.Error(w, "pubkey required", http.StatusBadRequest)
				return
			}
			s := sm.GetOrCreate(pubkey)
			writeJSON(w, map[string]any{
				"pubkey":          s.Pubkey,
				"tier":            s.Tier,
				"active":          s.IsActive(),
				"days_remaining":  s.DaysRemaining(),
				"auto_renew":      s.AutoRenew,
				"benefits":        s.Tier.Benefits(),
				"monthly_cost_ng": s.Tier.MonthlyCostNg(),
			})

		case "POST":
			var req struct {
				Pubkey       string `json:"pubkey"`
				Tier         string `json:"tier"`
				DurationDays int    `json:"duration_days"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
				return
			}
			if req.Pubkey == "" || req.Tier == "" {
				http.Error(w, "pubkey and tier required", http.StatusBadRequest)
				return
			}
			if req.DurationDays <= 0 {
				req.DurationDays = 30
			}

			tier := economy.Tier(req.Tier)
			costNg := tier.MonthlyCostNg() * int64(req.DurationDays/30)

			// Process payment: deduct from user, credit treasury + mining pool
			ledger := economy.LoadLedger(dataDir)
			if err := ledger.Transfer(req.Pubkey, "treasury", costNg); err != nil {
				http.Error(w, "payment failed: "+err.Error(), http.StatusPaymentRequired)
				return
			}
			ledger.Save(dataDir)

			// 2.28% to treasury reserve
			treasuryCut := tier.TreasuryCutNg() * int64(req.DurationDays/30)
			reservePath := filepath.Join(dataDir, "silver_reserve_ng.txt")
			reserve := int64(0)
			if b, err := os.ReadFile(reservePath); err == nil {
				fmt.Sscanf(string(b), "%d", &reserve)
			}
			reserve += treasuryCut
			fileutil.WriteFile(reservePath, []byte(fmt.Sprintf("%d", reserve)), 0600)

			// 97.72% to mining pool
			miningPoolShare := tier.MiningPoolNg() * int64(req.DurationDays/30)
			vm := economy.LoadVaultMining(dataDir)
			vm.CreditMiningPool(miningPoolShare)
			vm.Save(dataDir)

			s, err := sm.Subscribe(req.Pubkey, tier, req.DurationDays)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			writeJSON(w, map[string]any{
				"pubkey":                s.Pubkey,
				"tier":                  s.Tier,
				"active_till":           s.ActiveTill,
				"days_remaining":        s.DaysRemaining(),
				"auto_renew":            s.AutoRenew,
				"charged_ng":            costNg,
				"treasury_ng":           treasuryCut,
				"mining_pool_ng":        miningPoolShare,
			})

		default:
			http.Error(w, "GET or POST only", 400)
		}
	}
}


// ClaimDividendsHandler handles the ClaimDividendsHandler HTTP request.
func ClaimDividendsHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}

		serial := r.URL.Query().Get("serial")
		holder := r.URL.Query().Get("holder")
		if serial == "" || holder == "" {
			http.Error(w, "serial and holder required", http.StatusBadRequest)
			return
		}

		banknotes, err := economy.LoadBanknotesV2(dataDir)
		if err != nil {
			slog.Error("claim: load banknotes", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		var target *economy.BanknoteV2
		for i := range banknotes {
			if banknotes[i].Serial == serial && banknotes[i].Holder == holder {
				target = &banknotes[i]
				break
			}
		}
		if target == nil {
			http.Error(w, "banknote not found for this holder", http.StatusNotFound)
			return
		}

		totalNg := int64(0)
		for _, div := range target.DividendHistory {
			totalNg += div.Ng
		}

		if totalNg <= 0 {
			writeJSON(w, map[string]any{
				"serial":  serial,
				"holder":  holder,
				"claimed": 0,
				"note":    "no dividends to claim",
			})
			return
		}

		// Mint to holder's ledger
		ledger := economy.LoadLedger(dataDir)
		ledger.Mint(holder, totalNg)

		// Claim all dividends
		target.DividendHistory = []economy.DividendRecord{}

		for i := range banknotes {
			if banknotes[i].Serial == serial {
				banknotes[i].DividendHistory = target.DividendHistory
				break
			}
		}

		regFile := filepath.Join(dataDir, "banknotes_registry_v2.json")
		fileutil.WriteJSON(regFile, banknotes)

		logFile := filepath.Join(dataDir, "dividend_claims.log")
		if f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
			entry := map[string]any{"serial": serial, "holder": holder, "ng": totalNg}
			enc := json.NewEncoder(f)
			enc.Encode(entry)
			f.Close()
		}

		slog.Info("dividends claimed", "serial", serial, "holder", holder, "ng", totalNg)
		writeJSON(w, map[string]any{
			"serial":  serial,
			"holder":  holder,
			"claimed": totalNg,
			"note":    "dividends minted to holder ledger",
		})
	}
}


// WheelHandler handles the WheelHandler HTTP request.
func WheelHandler() http.HandlerFunc {
	spinner := economy.NewWheelSpinner()

	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}

		pubkey := r.URL.Query().Get("pubkey")
		if pubkey == "" {
			http.Error(w, "pubkey required", http.StatusBadRequest)
			return
		}

		switch r.Method {
		case "GET":
			next := spinner.NextSpinTime(pubkey)
			can := spinner.CanSpin(pubkey)
			resp := map[string]any{
				"can_spin": can,
				"pubkey":   pubkey,
			}
			if !next.IsZero() {
				resp["next_spin_at"] = next.Format(time.RFC3339)
			}
			writeJSON(w, resp)

		case "POST":
			reward, err := spinner.Spin(pubkey)
			if err != nil {
				http.Error(w, err.Error(), http.StatusTooManyRequests)
				return
			}
			writeJSON(w, map[string]any{
				"pubkey":  pubkey,
				"tier":    reward.Tier,
				"label":   reward.Label,
				"ng_award": reward.NgAward,
			})

		default:
			http.Error(w, "GET or POST only", 400)
		}
	}
}


// AutoMintHandler handles the AutoMintHandler HTTP request.
func AutoMintHandler(dataDir string) http.HandlerFunc {
	schedule := economy.LoadOrCreateSchedule(dataDir)

	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}

		reserve := int64(0)
		if b, err := os.ReadFile(filepath.Join(dataDir, "silver_reserve_ng.txt")); err == nil {
			fmt.Sscanf(string(b), "%d", &reserve)
		}

		cfg := economy.DefaultTreasuryConfig()
		current := economy.DetectTier(reserve, cfg)

		switch r.Method {
		case "GET":
			writeJSON(w, map[string]any{
				"current_tier":      current.String(),
				"reserve_ng":        reserve,
				"monthly_ops_ng":    cfg.MonthlyOpsNg,
				"last_tier":         schedule.LastTier.String(),
				"triggered_at":      schedule.TriggeredAt,
				"sets_total":        len(schedule.Sets),
				"sets_minted":       countMinted(schedule.Sets),
			})

		case "POST":
			notes, triggers, err := schedule.CheckAndMint(reserve, cfg, dataDir)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			if len(notes) > 0 {
				economy.MergeMintedBanknotes(dataDir, notes)
				schedule.Save(dataDir)
			}
			writeJSON(w, map[string]any{
				"current_tier":      current.String(),
				"triggers":          triggers,
				"banknotes_minted":  len(notes),
				"reserve_ng":        reserve,
			})

		default:
			http.Error(w, "GET or POST only", 400)
		}
	}
}

func countMinted(sets []economy.BanknoteSet) int {
	n := 0
	for _, s := range sets {
		if s.Minted {
			n++
		}
	}
	return n
}


// CraftingHandler handles the CraftingHandler HTTP request.
func CraftingHandler(dataDir string) http.HandlerFunc {
	cm := economy.NewCraftingManager()

	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}

		holder := r.URL.Query().Get("holder")
		if holder == "" {
			http.Error(w, "holder required", http.StatusBadRequest)
			return
		}

		switch r.Method {
		case "GET":
			banknotes, err := economy.LoadBanknotesV2(dataDir)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			lb := economy.GetLeaderboard(banknotes, 20)
			writeJSON(w, map[string]any{
				"leaderboard": lb,
				"upgrade_cost": economy.UpgradeInputCount,
			})

		case "POST":
			var req struct {
				Serials []string `json:"serials"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "invalid json", http.StatusBadRequest)
				return
			}
			burnt, upgraded, err := cm.CraftUpgrade(dataDir, holder, req.Serials)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			writeJSON(w, map[string]any{
				"burnt_serials": burnt,
				"upgraded":      upgraded,
			})

		default:
			http.Error(w, "GET or POST only", 400)
		}
	}
}


// AutoReinvestHandler handles the AutoReinvestHandler HTTP request.
func AutoReinvestHandler(dataDir string) http.HandlerFunc {
	ar := economy.NewAutoReinvestManager()

	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}

		pubkey := r.URL.Query().Get("pubkey")
		if pubkey == "" {
			http.Error(w, "pubkey required", http.StatusBadRequest)
			return
		}

		switch r.Method {
		case "GET":
			writeJSON(w, map[string]any{
				"pubkey":         pubkey,
				"auto_reinvest":  ar.IsEnabled(pubkey),
			})

		case "POST":
			var req struct {
				Enabled  bool  `json:"enabled"`
				AmountNg int64 `json:"amount_ng"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
				return
			}

			if req.Enabled {
				ar.SetEnabled(pubkey, true)
				if req.AmountNg > 0 {
					n, err := ar.Reinvest(dataDir, pubkey, req.AmountNg)
					if err != nil {
						http.Error(w, err.Error(), http.StatusBadRequest)
						return
					}
					writeJSON(w, map[string]any{
						"pubkey":            pubkey,
						"auto_reinvest":     true,
						"banknotes_created": n,
						"amount_ng":         req.AmountNg,
					})
					return
				}
				writeJSON(w, map[string]any{
					"pubkey":        pubkey,
					"auto_reinvest": true,
				})
			} else {
				ar.SetEnabled(pubkey, false)
				writeJSON(w, map[string]any{
					"pubkey":        pubkey,
					"auto_reinvest": false,
				})
			}

		default:
			http.Error(w, "GET or POST only", 400)
		}
	}
}


// OracleHandler handles the OracleHandler HTTP request.
func OracleHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}

		oracle := economy.LoadOracle(dataDir)

		switch r.Method {
		case "GET":
			writeJSON(w, map[string]any{
				"current_price": oracle.GetPrice(),
				"last_updated":  oracle.GetLastUpdated(),
				"default":       economy.DefaultSilverSpotUSDperOZ,
			})

		case "POST":
			var req struct {
				Price float64 `json:"price"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "invalid json", http.StatusBadRequest)
				return
			}
			if err := oracle.UpdatePrice(req.Price); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			oracle.Save(dataDir)
			writeJSON(w, map[string]any{
				"current_price": oracle.GetPrice(),
				"updated_at":    time.Now().UTC().Format(time.RFC3339),
			})

		default:
			http.Error(w, "GET or POST only", 400)
		}
	}
}

// OracleLiveHandler uses a shared oracle instance with live polling.
func OracleLiveHandler(o *economy.SilverSpotOracle) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}

		switch r.Method {
		case "GET":
			history := o.GetHistory()
			writeJSON(w, map[string]any{
				"current_price": o.GetPrice(),
				"last_updated":  o.GetLastUpdated(),
				"default":       economy.DefaultSilverSpotUSDperOZ,
				"history":       history,
			})

		case "POST":
			var req struct {
				Price float64 `json:"price"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "invalid json", http.StatusBadRequest)
				return
			}
			if err := o.UpdatePrice(req.Price); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			writeJSON(w, map[string]any{
				"current_price": o.GetPrice(),
				"updated_at":    time.Now().UTC().Format(time.RFC3339),
			})

		default:
			http.Error(w, "GET or POST only", 400)
		}
	}
}


// DeflationHandler handles the DeflationHandler HTTP request.
func DeflationHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}

		dm := economy.LoadDeflation(dataDir)

		switch r.Method {
		case "GET":
			reserve := int64(0)
			if b, err := os.ReadFile(filepath.Join(dataDir, "silver_reserve_ng.txt")); err == nil {
				fmt.Sscanf(string(b), "%d", &reserve)
			}
			cfg := economy.DefaultTreasuryConfig()
			tier := economy.DetectTier(reserve, cfg)
			writeJSON(w, map[string]any{
				"total_burned_ng": dm.TotalBurnedNg,
				"last_burn_round": dm.LastBurnRound,
				"threshold_basis": dm.ThresholdBasis,
				"current_tier":    tier.String(),
				"reserve_ng":      reserve,
			})

		case "POST":
			reserve := int64(0)
			if b, err := os.ReadFile(filepath.Join(dataDir, "silver_reserve_ng.txt")); err == nil {
				fmt.Sscanf(string(b), "%d", &reserve)
			}
			cfg := economy.DefaultTreasuryConfig()
			amt, err := dm.Burn(dataDir, reserve, cfg)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			writeJSON(w, map[string]any{
				"burned_ng":       amt,
				"total_burned_ng": dm.TotalBurnedNg,
				"last_burn_round": dm.LastBurnRound,
			})

		default:
			http.Error(w, "GET or POST only", 400)
		}
	}
}


// LicenseHandler handles the LicenseHandler HTTP request.
func LicenseHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		lm := economy.LoadLicenses(dataDir)

		switch r.Method {
		case "GET":
			if id := r.URL.Query().Get("id"); id != "" {
				lic, err := lm.Get(id)
				if err != nil {
					http.Error(w, err.Error(), http.StatusNotFound)
					return
				}
				writeJSON(w, lic)
				return
			}
			writeJSON(w, map[string]any{"licenses": lm.List()})

		case "POST":
			var req struct {
				ID         string `json:"id"`
				Name       string `json:"name"`
				Tier       string `json:"tier"`
				RoyaltyBPS int    `json:"royalty_bps"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "invalid json", http.StatusBadRequest)
				return
			}
			if req.ID == "" || req.Name == "" || req.Tier == "" {
				http.Error(w, "id, name, tier required", http.StatusBadRequest)
				return
			}
			lic, err := lm.Create(req.ID, req.Name, req.Tier, req.RoyaltyBPS)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			lm.Save(dataDir)
			writeJSON(w, lic)

		case "PUT":
			var req struct {
				ID        string `json:"id"`
				Name      string `json:"name"`
				OnionAddr string `json:"onion_addr"`
				Status    string `json:"status"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "invalid json", http.StatusBadRequest)
				return
			}
			lic, err := lm.Update(req.ID, req.Name, req.OnionAddr, economy.LicenseStatus(req.Status))
			if err != nil {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			lm.Save(dataDir)
			writeJSON(w, lic)

		case "DELETE":
			id := r.URL.Query().Get("id")
			if id == "" {
				http.Error(w, "id required", http.StatusBadRequest)
				return
			}
			if err := lm.Delete(id); err != nil {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			lm.Save(dataDir)
			writeJSON(w, map[string]any{"deleted": id})

		default:
			http.Error(w, "GET/POST/PUT/DELETE only", 400)
		}
	}
}


// EarmarkHandler handles the EarmarkHandler HTTP request.
func EarmarkHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		em := economy.LoadEarmarks(dataDir)

		switch r.Method {
		case "GET":
			if id := r.URL.Query().Get("id"); id != "" {
				writeJSON(w, map[string]any{
					"remaining": em.Remaining(id),
				})
				return
			}
			writeJSON(w, map[string]any{"accounts": em.List()})

		case "POST":
			var req struct {
				ID          string `json:"id"`
				Holder      string `json:"holder"`
				Purpose     string `json:"purpose"`
				LicenseID   string `json:"license_id"`
				AllocatedNg int64  `json:"allocated_ng"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "invalid json", http.StatusBadRequest)
				return
			}
			if req.ID == "" || req.Holder == "" {
				http.Error(w, "id and holder required", http.StatusBadRequest)
				return
			}
			acct, err := em.Create(dataDir, req.ID, req.Holder, req.Purpose, req.LicenseID, req.AllocatedNg)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			em.Save(dataDir)
			writeJSON(w, acct)

		default:
			http.Error(w, "GET or POST only", 400)
		}
	}
}


// MintAuthHandler handles the MintAuthHandler HTTP request.
func MintAuthHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		m := economy.LoadMintAuths(dataDir)

		switch r.Method {
		case "GET":
			if r.URL.Query().Get("pending") == "true" {
				writeJSON(w, map[string]any{"pending": m.Pending()})
				return
			}
			writeJSON(w, map[string]any{"auths": m.List()})

		case "POST":
			var req struct {
				LicenseID string `json:"license_id"`
				Serial    string `json:"serial"`
				Rarity    string `json:"rarity"`
				DenomNg   int64  `json:"denomination_ng"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "invalid json", http.StatusBadRequest)
				return
			}
			auth := m.Request(req.LicenseID, req.Serial, req.Rarity, req.DenomNg)
			m.Save(dataDir)
			writeJSON(w, auth)

		case "PUT":
			id := r.URL.Query().Get("id")
			if id == "" {
				http.Error(w, "id required", http.StatusBadRequest)
				return
			}
			auth, err := m.Approve(id)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			m.Save(dataDir)
			writeJSON(w, auth)

		default:
			http.Error(w, "GET/POST/PUT only", 400)
		}
	}
}


// ConstitutionHandler handles the ConstitutionHandler HTTP request.
func ConstitutionHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		c := economy.LoadConstitution(dataDir)
		writeJSON(w, map[string]any{
			"version":      c.Version,
			"last_amended": c.LastAmended,
			"articles":     c.Articles,
		})
	}
}


// StewardMonitorHandler handles the StewardMonitorHandler HTTP request.
func StewardMonitorHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		writeJSON(w, map[string]any{
			"status": "active",
		})
	}
}


// SettlementHandler handles the SettlementHandler HTTP request.
func SettlementHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		sm := economy.LoadSettlements(dataDir)

		switch r.Method {
		case "GET":
			writeJSON(w, map[string]any{"settlements": sm.List()})

		case "POST":
			var req struct {
				From        string `json:"from_license"`
				To          string `json:"to_license"`
				AmountNg    int64  `json:"amount_ng"`
				Description string `json:"description"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "invalid json", http.StatusBadRequest)
				return
			}
			s, err := sm.Create(req.From, req.To, req.Description, req.AmountNg)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			sm.Save(dataDir)
			writeJSON(w, s)

		case "PUT":
			id := r.URL.Query().Get("id")
			if id == "" {
				http.Error(w, "id required", http.StatusBadRequest)
				return
			}
			s, err := sm.Complete(id)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			sm.Save(dataDir)
			writeJSON(w, s)

		default:
			http.Error(w, "GET/POST/PUT only", 400)
		}
	}
}


// RoyaltyHandler handles the RoyaltyHandler HTTP request.
func RoyaltyHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		rm := economy.LoadRoyalties(dataDir)

		switch r.Method {
		case "GET":
			if r.URL.Query().Get("pending") == "true" {
				writeJSON(w, map[string]any{"pending": rm.Pending()})
				return
			}
			writeJSON(w, map[string]any{"payments": rm.List()})

		case "POST":
			var req struct {
				LicenseID string `json:"license_id"`
				Period    string `json:"period"`
				AmountNg  int64  `json:"amount_ng"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "invalid json", http.StatusBadRequest)
				return
			}
			p := rm.CreateDue(req.LicenseID, req.Period, req.AmountNg)
			rm.Save(dataDir)
			writeJSON(w, p)

		case "PUT":
			id := r.URL.Query().Get("id")
			if id == "" {
				http.Error(w, "id required", http.StatusBadRequest)
				return
			}
			p, err := rm.Pay(id)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			rm.Save(dataDir)
			writeJSON(w, p)

		default:
			http.Error(w, "GET/POST/PUT only", 400)
		}
	}
}

// TemplateHandler handles the TemplateHandler HTTP request.
func TemplateHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		tm := economy.LoadTemplates(dataDir)

		switch r.Method {
		case "GET":
			if id := r.URL.Query().Get("id"); id != "" {
				tpl, err := tm.Get(id)
				if err != nil {
					http.Error(w, err.Error(), http.StatusNotFound)
					return
				}
				writeJSON(w, tpl)
				return
			}
			writeJSON(w, map[string]any{"templates": tm.List()})

		case "POST":
			var req struct {
				ID         string `json:"id"`
				LicenseID  string `json:"license_id"`
				Name       string `json:"name"`
				DesignJSON string `json:"design_json"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "invalid json", http.StatusBadRequest)
				return
			}
			tpl, err := tm.Create(req.ID, req.LicenseID, req.Name, req.DesignJSON)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			tm.Save(dataDir)
			writeJSON(w, tpl)

		case "DELETE":
			id := r.URL.Query().Get("id")
			if id == "" {
				http.Error(w, "id required", http.StatusBadRequest)
				return
			}
			if err := tm.Delete(id); err != nil {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			tm.Save(dataDir)
			writeJSON(w, map[string]any{"deleted": id})

		default:
			http.Error(w, "GET/POST/DELETE only", 400)
		}
	}
}


// ServiceRegistryHandler handles the ServiceRegistryHandler HTTP request.
func ServiceRegistryHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		sr := economy.LoadServiceRegistry(dataDir)

		switch r.Method {
		case "GET":
			svcType := r.URL.Query().Get("type")
			if svcType != "" {
				writeJSON(w, map[string]any{"services": sr.ListByType(economy.ServiceType(svcType))})
				return
			}
			if id := r.URL.Query().Get("id"); id != "" {
				svc, err := sr.Get(id)
				if err != nil {
					http.Error(w, err.Error(), http.StatusNotFound)
					return
				}
				writeJSON(w, svc)
				return
			}
			writeJSON(w, map[string]any{"services": sr.ListAll()})

		case "POST":
			var svc economy.RegisteredService
			if err := json.NewDecoder(r.Body).Decode(&svc); err != nil {
				http.Error(w, "invalid json", http.StatusBadRequest)
				return
			}
			if err := sr.Register(&svc); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			sr.Save(dataDir)
			writeJSON(w, svc)

		case "DELETE":
			id := r.URL.Query().Get("id")
			if id == "" {
				http.Error(w, "id required", http.StatusBadRequest)
				return
			}
			if err := sr.Deregister(id); err != nil {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			sr.Save(dataDir)
			writeJSON(w, map[string]any{"deleted": id})

		default:
			http.Error(w, "GET/POST/DELETE only", 400)
		}
	}
}


// ServiceMarketplaceHandler handles the ServiceMarketplaceHandler HTTP request.
func ServiceMarketplaceHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		sm := economy.LoadMarketplace(dataDir)

		switch r.Method {
		case "GET":
			writeJSON(w, map[string]any{"listings": sm.Search(economy.SvcStorage)})

		case "POST":
			var req struct {
				ServiceID string `json:"service_id"`
				SellerID  string `json:"seller_id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "invalid json", http.StatusBadRequest)
				return
			}
			listing, err := sm.List(req.ServiceID, req.SellerID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			sm.Save(dataDir)
			writeJSON(w, listing)

		case "PUT":
			id := r.URL.Query().Get("id")
			if id == "" {
				http.Error(w, "id required", http.StatusBadRequest)
				return
			}
			listing, err := sm.Buy(id)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			sm.Save(dataDir)
			writeJSON(w, listing)

		default:
			http.Error(w, "GET/POST/PUT only", 400)
		}
	}
}