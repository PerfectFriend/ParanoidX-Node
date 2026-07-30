// Package steward implements the Steward AI service
package steward

import "ParanoidX/internal/economy"

// ConstitutionRule defines a single parameter bound.
type ConstitutionRule struct {
	Name        string  `json:"name"`
	Min         float64 `json:"min"`
	Max         float64 `json:"max"`
	Target      float64 `json:"target"`
	Unit        string  `json:"unit"`
	Description string  `json:"description"`
}

// Constitution holds all economic rules.
type Constitution struct {
	Rules []ConstitutionRule `json:"rules"`
}

// DefaultConstitution returns the hardcoded constitution.
func DefaultConstitution() *Constitution {
	return &Constitution{
		Rules: []ConstitutionRule{
			{Name: "silver_reserve_ratio", Min: 0.60, Max: 0.80, Target: 0.70, Unit: "ratio", Description: "Silver backing ratio (60-80%)"},
			{Name: "treasury_commission_bps", Min: 200, Max: 250, Target: 228, Unit: "bps", Description: "Treasury commission in basis points"},
			{Name: "max_total_fee_bps", Min: 380, Max: 450, Target: 420, Unit: "bps", Description: "Max total fee cap in basis points"},
			{Name: "ng_per_tlr", Min: 3e10, Max: 3.2e10, Target: float64(economy.NGPerTLR), Unit: "ng", Description: "Nanograms per Troy oz (1 TLR)"},
			{Name: "subscription_citizen_ng", Min: 1e9, Max: 5e9, Target: 2e9, Unit: "ng", Description: "Citizen subscription cost per month"},
			{Name: "subscription_aristocrat_ng", Min: 1e10, Max: 5e10, Target: 2e10, Unit: "ng", Description: "Aristocrat subscription cost per month"},
			{Name: "auction_listing_fee_bps", Min: 20, Max: 100, Target: 50, Unit: "bps", Description: "Auction listing fee in basis points"},
			{Name: "auction_seller_fee_bps", Min: 50, Max: 200, Target: 100, Unit: "bps", Description: "Auction seller fee in basis points"},
			{Name: "auction_buyer_premium_bps", Min: 100, Max: 400, Target: 250, Unit: "bps", Description: "Auction buyer premium in basis points"},
			{Name: "mining_pool_share_pct", Min: 0.90, Max: 0.99, Target: 0.9772, Unit: "ratio", Description: "Share of subscription revenue to mining pool"},
			{Name: "dividend_share_bps", Min: 150, Max: 250, Target: 192, Unit: "bps", Description: "Dividend share for banknote holders"},
			{Name: "treasury_tier_fat_threshold", Min: 5, Max: 8, Target: 6, Unit: "multiples", Description: "Monthly ops multiples for Fat tier"},
			{Name: "treasury_tier_veryfat_threshold", Min: 10, Max: 15, Target: 12, Unit: "multiples", Description: "Monthly ops multiples for Very Fat tier"},
			{Name: "pos_fee_bps", Min: 50, Max: 200, Target: 100, Unit: "bps", Description: "POS processing fee in basis points"},
			{Name: "silver_spot_usd_per_oz", Min: 50, Max: 150, Target: 75, Unit: "usd", Description: "Silver spot price in USD per oz"},
			{Name: "utility_premium_pct", Min: 0.20, Max: 0.40, Target: 0.30, Unit: "ratio", Description: "Utility premium percentage for digital convenience"},
		},
	}
}

// CheckRule returns deviation from target as a fraction of the allowed range.
// Returns 0 = on target, positive = above target, negative = below target.
// Values outside [min, max] are considered severe deviations.
func (c *Constitution) CheckRule(name string, value float64) (deviation float64, severity string) {
	for _, rule := range c.Rules {
		if rule.Name == name {
			if value < rule.Min || value > rule.Max {
				return value - rule.Target, "critical"
			}
			mid := (rule.Min + rule.Max) / 2
			dev := (value - mid) / (rule.Max - rule.Min) * 2
			if dev < -0.5 || dev > 0.5 {
				return dev, "major"
			}
			return dev, "minor"
		}
	}
	return 0, "unknown"
}
