// Package api provides HTTP handlers and API endpoints for the simplex-node server
package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"ParanoidX/internal/fileutil"
)

// ICOTier represents a tier level in the ICO with specific min investment and bonus.
type ICOTier string

const (
	TierAngel ICOTier = "angel"
	TierMajor ICOTier = "major"
	TierMinor ICOTier = "minor"
	TierCitizen ICOTier = "citizen"
)

var tierLimits = map[ICOTier]struct {
	MinUSD int64
	VestingMonths int
	BonusPct      int64
}{
	TierAngel:  {MinUSD: 100000, VestingMonths: 24, BonusPct: 30},
	TierMajor:  {MinUSD: 10000, VestingMonths: 12, BonusPct: 20},
	TierMinor:  {MinUSD: 1000, VestingMonths: 6, BonusPct: 10},
	TierCitizen: {MinUSD: 100, VestingMonths: 3, BonusPct: 5},
}

// ICOState represents the current state of the initial coin offering.
type ICOState struct {
	Active        bool      `json:"active"`
	StartTime     string    `json:"start_time"`
	EndTime       string    `json:"end_time"`
	TotalRaisedNg int64     `json:"total_raised_ng"`
	TokenPriceNg  int64     `json:"token_price_ng"`
	Investors     int       `json:"investors"`
	HardCapNg     int64     `json:"hard_cap_ng"`
}

// ICOInvestment records a single investor's contribution to the ICO.
type ICOInvestment struct {
	Investor      string   `json:"investor"`
	Tier          ICOTier  `json:"tier"`
	AmountNg      int64    `json:"amount_ng"`
	TokenAllocNg  int64    `json:"token_alloc_ng"`
	VestingMonths int      `json:"vesting_months"`
	BonusPct      int64    `json:"bonus_pct"`
	Time          string   `json:"time"`
}

func loadICOState(dataDir string) *ICOState {
	p := filepath.Join(dataDir, "ico_state.json")
	var s ICOState
	if b, err := os.ReadFile(p); err == nil {
		json.Unmarshal(b, &s)
	}
	if s.TokenPriceNg == 0 {
		s.TokenPriceNg = 1000000000 // 1B ng per token (~$2.41)
	}
	if s.HardCapNg == 0 {
		s.HardCapNg = 100000000000000 // 100T ng hard cap
	}
	return &s
}

func saveICOState(dataDir string, s *ICOState) {
	p := filepath.Join(dataDir, "ico_state.json")
	fileutil.WriteJSON(p, s)
}

func loadICOInvestments(dataDir string) []ICOInvestment {
	p := filepath.Join(dataDir, "ico_investments.json")
	var invs []ICOInvestment
	if b, err := os.ReadFile(p); err == nil {
		json.Unmarshal(b, &invs)
	}
	return invs
}

func saveICOInvestments(dataDir string, invs []ICOInvestment) {
	p := filepath.Join(dataDir, "ico_investments.json")
	fileutil.WriteJSON(p, invs)
}


// ICOInfoHandler returns ICO state and available tier details.
func ICOInfoHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s := loadICOState(dataDir)
		tiers := make([]map[string]any, 0)
		for tier, info := range tierLimits {
			tiers = append(tiers, map[string]any{
				"tier":           string(tier),
				"min_usd":        info.MinUSD,
				"vesting_months": info.VestingMonths,
				"bonus_pct":      info.BonusPct,
			})
		}
		writeJSON(w, map[string]any{"ico": s, "tiers": tiers})
	}
}


// ICOStatusHandler returns the current ICO status (active, raised, investors).
func ICOStatusHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s := loadICOState(dataDir)
		invs := loadICOInvestments(dataDir)
		writeJSON(w, map[string]any{
			"active":          s.Active,
			"total_raised_ng": s.TotalRaisedNg,
			"investors":       len(invs),
			"hard_cap_ng":     s.HardCapNg,
			"token_price_ng":  s.TokenPriceNg,
		})
	}
}


// ICOInvestHandler processes an ICO investment with tier-based bonus calculation.
func ICOInvestHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		var req struct {
			Investor string `json:"investor"`
			Tier     string `json:"tier"`
			AmountNg int64  `json:"amount_ng"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", 400)
			return
		}
		tier := ICOTier(req.Tier)
		tierInfo, ok := tierLimits[tier]
		if !ok {
			http.Error(w, "invalid tier", 400)
			return
		}
		s := loadICOState(dataDir)
		if !s.Active {
			http.Error(w, "ICO not active", 400)
			return
		}
		if s.TotalRaisedNg+req.AmountNg > s.HardCapNg {
			http.Error(w, "hard cap reached", 400)
			return
		}
		bonus := req.AmountNg * tierInfo.BonusPct / 100
		totalAlloc := req.AmountNg + bonus
		inv := ICOInvestment{
			Investor:      req.Investor,
			Tier:          tier,
			AmountNg:      req.AmountNg,
			TokenAllocNg:  totalAlloc,
			VestingMonths: tierInfo.VestingMonths,
			BonusPct:      tierInfo.BonusPct,
			Time:          time.Now().Format(time.RFC3339),
		}
		invs := loadICOInvestments(dataDir)
		invs = append(invs, inv)
		saveICOInvestments(dataDir, invs)
		s.TotalRaisedNg += req.AmountNg
		s.Investors = len(invs)
		saveICOState(dataDir, s)
		writeJSON(w, map[string]any{"ok": true, "investment": inv})
	}
}
