// Package economy implements the island economy system
package economy

import (
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"ParanoidX/internal/fileutil"
)

// Tier defines a subscription tier for citizens.
type Tier string

const (
	TierColonist   Tier = "colonist"    // free, basic access
	TierCitizen    Tier = "citizen"     // 2B ng/mo ($4.82) — vault 2GB, 0% P2P fee, voting, POS
	TierAristocrat Tier = "aristocrat"  // 20B ng/mo ($48.20) — x4 dividends, auto-bid, priority POS
)

// MonthlyCostNg returns the monthly cost in ng for each tier.
func (t Tier) MonthlyCostNg() int64 {
	switch t {
	case TierColonist:
		return 0
	case TierCitizen:
		return 2_000_000_000 // $4.82/mo — 2GB vault, POS, voting
	case TierAristocrat:
		return 20_000_000_000 // $48.20/mo — 8GB vault, x4 dividends, priority POS
	default:
		return 0
	}
}

// TreasuryCutNg возвращает долю казны (2.28%) от суммы подписки.
func (t Tier) TreasuryCutNg() int64 {
	return t.MonthlyCostNg() * GetTreasuryCommissionBPS() / 10000
}

// MiningPoolNg возвращает долю майнерам (97.72%) от суммы подписки.
func (t Tier) MiningPoolNg() int64 {
	return t.MonthlyCostNg() - t.TreasuryCutNg()
}

// Benefits describes what a tier provides.
type Benefits struct {
	VaultQuotaMB  int     `json:"vault_quota_mb"`
	P2pFeePercent float64 `json:"p2p_fee_percent"`
	DividendMult  int     `json:"dividend_multiplier"`
	EarlyAccess   bool    `json:"early_access"`
	CanVote       bool    `json:"can_vote"`
	AutoBid       bool    `json:"auto_bid"`
}

// Benefits returns the feature set enabled by this tier.
func (t Tier) Benefits() Benefits {
	switch t {
	case TierColonist:
		return Benefits{VaultQuotaMB: 512, P2pFeePercent: 0.0228, DividendMult: 1, CanVote: false}
	case TierCitizen:
		return Benefits{VaultQuotaMB: 2048, P2pFeePercent: 0.0, DividendMult: 1, CanVote: true}
	case TierAristocrat:
		return Benefits{VaultQuotaMB: 8192, P2pFeePercent: 0.0, DividendMult: 4, EarlyAccess: true, CanVote: true, AutoBid: true}
	default:
		return Benefits{}
	}
}

// Subscription holds the current subscription state for a pubkey.
type Subscription struct {
	Pubkey     string `json:"pubkey"`
	Tier       Tier   `json:"tier"`
	ActiveTill int64  `json:"active_till"` // unix timestamp
	AutoRenew  bool   `json:"auto_renew"`
}

// IsActive returns true if the subscription is current.
func (s *Subscription) IsActive() bool {
	return s.ActiveTill > time.Now().Unix()
}

// DaysRemaining returns how many days until expiry.
func (s *Subscription) DaysRemaining() int {
	remaining := s.ActiveTill - time.Now().Unix()
	if remaining <= 0 {
		return 0
	}
	return int(remaining / 86400)
}

// SubscriptionManager manages citizen subscriptions.
type SubscriptionManager struct {
	mu   sync.Mutex
	Subs map[string]*Subscription `json:"subscriptions"`
}

// NewSubscriptionManager creates a subscription manager with an empty subscriptions map.
func NewSubscriptionManager() *SubscriptionManager {
	return &SubscriptionManager{Subs: make(map[string]*Subscription)}
}

// LoadSubscriptionManager loads subscriptions from disk.
func LoadSubscriptionManager(dataDir string) *SubscriptionManager {
	sm := NewSubscriptionManager()
	fileutil.ReadJSON(filepath.Join(dataDir, "subscriptions.json"), sm)
	if sm.Subs == nil {
		sm.Subs = make(map[string]*Subscription)
	}
	return sm
}

// Save persists subscriptions to JSON.
func (m *SubscriptionManager) Save(dataDir string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := filepath.Join(dataDir, "subscriptions.json")
	fileutil.WriteJSON(p, m)
}

// GetOrCreate returns the subscription for pubkey, creating a free Colonist one if missing.
func (m *SubscriptionManager) GetOrCreate(pubkey string) *Subscription {
	if s, ok := m.Subs[pubkey]; ok {
		return s
	}
	s := &Subscription{
		Pubkey:     pubkey,
		Tier:       TierColonist,
		ActiveTill: time.Now().Add(365 * 24 * time.Hour).Unix(), // 1 year free
		AutoRenew:  false,
	}
	m.Subs[pubkey] = s
	return s
}

// Subscribe upgrades a subscription to the given tier for the given duration.
func (m *SubscriptionManager) Subscribe(pubkey string, tier Tier, durationDays int) (*Subscription, error) {
	if tier == TierColonist {
		return nil, fmt.Errorf("colonist is the free tier, use upgrade for paid tiers")
	}
	costNg := tier.MonthlyCostNg() * int64(durationDays/30)
	if costNg <= 0 {
		return nil, fmt.Errorf("invalid duration or tier cost")
	}

	s := m.GetOrCreate(pubkey)

	// Extend from current expiry or from now
	start := s.ActiveTill
	if start < time.Now().Unix() {
		start = time.Now().Unix()
	}
	s.Tier = tier
	s.ActiveTill = start + int64(durationDays*86400)
	s.AutoRenew = true
	return s, nil
}

// GetTier returns the effective tier for a pubkey.
func (m *SubscriptionManager) GetTier(pubkey string) Tier {
	s := m.GetOrCreate(pubkey)
	if !s.IsActive() {
		return TierColonist
	}
	return s.Tier
}
