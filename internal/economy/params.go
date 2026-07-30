// Package economy implements the island economy system
package economy

import (
	"path/filepath"
	"sync"
	"time"

	"ParanoidX/internal/fileutil"
)

// DynamicParams holds economy parameters that the Steward can adjust at runtime.
type DynamicParams struct {
	mu                    sync.RWMutex
	TreasuryCommissionBPS int     `json:"treasury_commission_bps"`
	MaxTotalFeeBPS        int     `json:"max_total_fee_bps"`
	AuctionListingFeeBPS  int     `json:"auction_listing_fee_bps"`
	AuctionBuyerPremium   int     `json:"auction_buyer_premium_bps"`
	AuctionSellerFeeBPS   int     `json:"auction_seller_fee_bps"`
	UtilityPremiumPct     float64 `json:"utility_premium_pct"`
	SilverBackingRatio    float64 `json:"silver_backing_ratio"`
	PosFeeBPS             int     `json:"pos_fee_bps"`
	MonthlyOpsNg          int64   `json:"monthly_ops_ng"`
	MiningPoolSharePct    float64 `json:"mining_pool_share_pct"`
	UpdatedAt             string  `json:"updated_at"`
}

// CurrentParams is the package-level dynamic params used by all economy code.
// Set by the steward to enable runtime parameter adjustment.
var CurrentParams = DefaultDynamicParams()

// GetTreasuryCommissionBPS returns the current treasury commission in basis points.
func GetTreasuryCommissionBPS() int64 {
	return int64(CurrentParams.TreasuryCommissionBPS)
}

// GetMaxTotalFeeBPS returns the current max fee in basis points.
func GetMaxTotalFeeBPS() int64 {
	return int64(CurrentParams.MaxTotalFeeBPS)
}

// GetAuctionListingFeeBPS returns the auction listing fee in basis points.
func GetAuctionListingFeeBPS() int64 {
	return int64(CurrentParams.AuctionListingFeeBPS)
}

// GetAuctionBuyerPremiumBPS returns the buyer premium in basis points.
func GetAuctionBuyerPremiumBPS() int64 {
	return int64(CurrentParams.AuctionBuyerPremium)
}

// GetAuctionSellerFeeBPS returns the seller fee in basis points.
func GetAuctionSellerFeeBPS() int64 {
	return int64(CurrentParams.AuctionSellerFeeBPS)
}

// GetPOSFeeBPS returns the POS processing fee in basis points.
func GetPOSFeeBPS() int64 {
	return int64(CurrentParams.PosFeeBPS)
}

// DefaultDynamicParams returns default parameter values.
func DefaultDynamicParams() *DynamicParams {
	return &DynamicParams{
		TreasuryCommissionBPS: 228,
		MaxTotalFeeBPS:        420,
		AuctionListingFeeBPS:  50,
		AuctionBuyerPremium:   250,
		AuctionSellerFeeBPS:   100,
		UtilityPremiumPct:     0.30,
		SilverBackingRatio:    0.70,
		PosFeeBPS:             100,
		MonthlyOpsNg:          500_000_000_000,
		MiningPoolSharePct:    0.9772,
		UpdatedAt:             time.Now().UTC().Format(time.RFC3339),
	}
}

// LoadDynamicParams loads or creates params from disk.
func LoadDynamicParams(dataDir string) *DynamicParams {
	p := DefaultDynamicParams()
	fileutil.ReadJSON(filepath.Join(dataDir, "economy_params.json"), p)
	return p
}


// Save handles the Save HTTP request.
func (p *DynamicParams) Save(dataDir string) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	p.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	fileutil.WriteJSON(filepath.Join(dataDir, "economy_params.json"), p)
}

// Adjust safely updates a parameter. Returns true if changed.
func (p *DynamicParams) Adjust(name string, value float64) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	switch name {
	case "treasury_commission_bps":
		v := int(value)
		if v >= 100 && v <= 500 {
			p.TreasuryCommissionBPS = v
			return true
		}
	case "max_total_fee_bps":
		v := int(value)
		if v >= 200 && v <= 1000 {
			p.MaxTotalFeeBPS = v
			return true
		}
	case "auction_listing_fee_bps":
		v := int(value)
		if v >= 10 && v <= 200 {
			p.AuctionListingFeeBPS = v
			return true
		}
	case "auction_buyer_premium_bps":
		v := int(value)
		if v >= 50 && v <= 500 {
			p.AuctionBuyerPremium = v
			return true
		}
	case "auction_seller_fee_bps":
		v := int(value)
		if v >= 20 && v <= 300 {
			p.AuctionSellerFeeBPS = v
			return true
		}
	case "utility_premium_pct":
		if value >= 0.10 && value <= 0.50 {
			p.UtilityPremiumPct = value
			return true
		}
	case "silver_backing_ratio":
		if value >= 0.50 && value <= 0.90 {
			p.SilverBackingRatio = value
			return true
		}
	case "pos_fee_bps":
		v := int(value)
		if v >= 20 && v <= 300 {
			p.PosFeeBPS = v
			return true
		}
	case "mining_pool_share_pct":
		if value >= 0.80 && value <= 0.99 {
			p.MiningPoolSharePct = value
			return true
		}
	}
	return false
}

// Get returns a parameter value by name.
func (p *DynamicParams) Get(name string) float64 {
	p.mu.RLock()
	defer p.mu.RUnlock()

	switch name {
	case "treasury_commission_bps":
		return float64(p.TreasuryCommissionBPS)
	case "max_total_fee_bps":
		return float64(p.MaxTotalFeeBPS)
	case "auction_listing_fee_bps":
		return float64(p.AuctionListingFeeBPS)
	case "auction_buyer_premium_bps":
		return float64(p.AuctionBuyerPremium)
	case "auction_seller_fee_bps":
		return float64(p.AuctionSellerFeeBPS)
	case "utility_premium_pct":
		return p.UtilityPremiumPct
	case "silver_backing_ratio":
		return p.SilverBackingRatio
	case "pos_fee_bps":
		return float64(p.PosFeeBPS)
	case "mining_pool_share_pct":
		return p.MiningPoolSharePct
	}
	return 0
}

// TreasuryConfigFromParams returns a TreasuryConfig using dynamic monthly ops.
func (p *DynamicParams) TreasuryConfigFromParams() TreasuryConfig {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return TreasuryConfig{
		MonthlyOpsNg: p.MonthlyOpsNg,
		Threshold3x:  3,
		Threshold6x:  6,
		Threshold12x: 12,
	}
}

// All returns all params as a flat map.
func (p *DynamicParams) All() map[string]float64 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return map[string]float64{
		"treasury_commission_bps": float64(p.TreasuryCommissionBPS),
		"max_total_fee_bps":       float64(p.MaxTotalFeeBPS),
		"auction_listing_fee_bps": float64(p.AuctionListingFeeBPS),
		"auction_buyer_premium":   float64(p.AuctionBuyerPremium),
		"auction_seller_fee_bps":  float64(p.AuctionSellerFeeBPS),
		"utility_premium_pct":     p.UtilityPremiumPct,
		"silver_backing_ratio":    p.SilverBackingRatio,
		"pos_fee_bps":             float64(p.PosFeeBPS),
		"mining_pool_share_pct":   p.MiningPoolSharePct,
	}
}
