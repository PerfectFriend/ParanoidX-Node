// Package economy implements the island economy system
package economy

import (
		"fmt"
	"math"
		"path/filepath"
	"sync"
	"time"

	"ParanoidX/internal/fileutil"
)

// VaultMining — майнинг из подписок, не из инфляции.
//
// Каждая подписка:
//   - 2.28% → Казна (TreasuryCommissionBPS)
//   - 97.72% → Пул майнинга (распределяется по долям storage)
//
// Все комиссии в системе ≤ 4.20% (MaxTotalFeeBPS).
// Всё, что выше 2.28% → в дивидендный фонд.

// MinVaultGB is the minimum storage a vault provider can allocate.
// MaxVaultGB is the maximum storage a vault provider can allocate.
// DeferredRewardDays is the deferral period for mining rewards.
// HeartbeatIntervalSec is the expected heartbeat interval from providers.
// GracePeriodHours is the allowed offline period before penalties apply.
// BaseDowntimePenaltyPct is the starting penalty percentage for downtime.
// NetworkGrowthDenominator is used for growth-based reward calculations.
const (
	MinVaultGB     int64 = 10
	MaxVaultGB     int64 = 100_000
	DeferredRewardDays    = 7
	HeartbeatIntervalSec  = 3600
	GracePeriodHours      = 2
	BaseDowntimePenaltyPct = 2.0
	NetworkGrowthDenominator int64 = 1_000_000
)

// VaultProvider represents a storage provider registered for vault mining rewards.
type VaultProvider struct {
	Pubkey             string  `json:"pubkey"`
	AllocatedGB        int64   `json:"allocated_gb"`
	TotalEarnedNg      int64   `json:"total_earned_ng"`
	PendingNg          int64   `json:"pending_ng"`
	LastSeen           string  `json:"last_seen"`
	RegisteredAt       string  `json:"registered_at"`
	Active             bool    `json:"active"`
	DowntimeHours      int     `json:"downtime_hours"`
	UptimePct          float64 `json:"uptime_pct"`
	TotalHours         int     `json:"total_hours"`
	ConsecutiveDowntime int    `json:"consecutive_downtime"`
}

// VaultMiningManager manages vault provider registration, heartbeats, and deferred payouts.
type VaultMiningManager struct {
	mu               sync.Mutex
	Providers        map[string]*VaultProvider `json:"providers"`
	DeferredPoolNg   int64                     `json:"deferred_pool_ng"`
	TotalAllocated   int64                     `json:"total_allocated_gb"`
	LastPayout       string                    `json:"last_payout,omitempty"`
	LastPayoutAmount int64                     `json:"last_payout_amount"`
	PayoutCycle      int                       `json:"payout_cycle"`
}

// NewVaultMiningManager creates a vault mining manager with an empty provider map.
func NewVaultMiningManager() *VaultMiningManager {
	return &VaultMiningManager{
		Providers: make(map[string]*VaultProvider),
	}
}

// LoadVaultMining loads vault mining state from disk.
func LoadVaultMining(dataDir string) *VaultMiningManager {
	vm := NewVaultMiningManager()

	fileutil.ReadJSON(filepath.Join(dataDir, "vault_mining.json"), vm)
	if vm.Providers == nil {
		vm.Providers = make(map[string]*VaultProvider)
	}
	return vm
}

// Save persists vault mining state to JSON.
func (vm *VaultMiningManager) Save(dataDir string) {
	p := filepath.Join(dataDir, "vault_mining.json")
	fileutil.WriteJSON(p, vm)
}

// CreditMiningPool пополняет пул майнинга из подписок (после вычета 2.28% казны).
func (vm *VaultMiningManager) CreditMiningPool(amountNg int64) {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	vm.DeferredPoolNg += amountNg
}

func growingPenalty(consecutiveOfflineHours int) float64 {
	if consecutiveOfflineHours <= 0 {
		return 0
	}
	penalty := BaseDowntimePenaltyPct * math.Pow(2, float64(consecutiveOfflineHours-1))
	if penalty > 64.0 {
		penalty = 64.0
	}
	return penalty
}

// RegisterProvider registers a new vault storage provider.
func (vm *VaultMiningManager) RegisterProvider(pubkey string, gb int64) (*VaultProvider, error) {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	if gb < MinVaultGB {
		return nil, fmt.Errorf("minimum allocation is %d GB", MinVaultGB)
	}
	if gb > MaxVaultGB {
		return nil, fmt.Errorf("maximum allocation is %d GB", MaxVaultGB)
	}
	if _, exists := vm.Providers[pubkey]; exists {
		return nil, fmt.Errorf("provider %s already registered", pubkey)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	p := &VaultProvider{
		Pubkey:       pubkey,
		AllocatedGB:  gb,
		LastSeen:     now,
		RegisteredAt: now,
		Active:       true,
		UptimePct:    100.0,
	}
	vm.Providers[pubkey] = p
	vm.TotalAllocated += gb
	return p, nil
}

// Heartbeat records a provider heartbeat, applying downtime penalties if the grace period was exceeded.
func (vm *VaultMiningManager) Heartbeat(pubkey string) error {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	p, ok := vm.Providers[pubkey]
	if !ok {
		return fmt.Errorf("provider %s not registered", pubkey)
	}

	lastSeen, err := time.Parse(time.RFC3339, p.LastSeen)
	if err != nil {
		lastSeen = time.Now().Add(-2 * time.Hour)
	}

	hoursSince := time.Since(lastSeen).Hours()
	hoursInt := int(math.Max(1, hoursSince))
	p.TotalHours += hoursInt

	if hoursSince > GracePeriodHours {
		offlineHours := hoursInt - GracePeriodHours
		if offlineHours > 0 {
			p.ConsecutiveDowntime += offlineHours
			penaltyPct := growingPenalty(p.ConsecutiveDowntime)
			penalty := int64(float64(p.PendingNg) * penaltyPct / 100.0)
			if penalty > p.PendingNg {
				penalty = p.PendingNg
			}
			p.PendingNg -= penalty
			vm.DeferredPoolNg -= penalty
			if vm.DeferredPoolNg < 0 {
				vm.DeferredPoolNg = 0
			}
			p.DowntimeHours += offlineHours
		}
	} else {
		p.ConsecutiveDowntime = 0
	}

	if p.TotalHours > 0 {
		p.UptimePct = float64(p.TotalHours-p.DowntimeHours) / float64(p.TotalHours) * 100.0
	}
	p.Active = p.UptimePct >= 50.0
	p.LastSeen = time.Now().UTC().Format(time.RFC3339)
	return nil
}

// ProcessDeferredPayouts distributes the deferred mining pool to providers proportional to allocation.
func (vm *VaultMiningManager) ProcessDeferredPayouts(dataDir string) (int64, error) {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	if vm.DeferredPoolNg <= 0 || vm.TotalAllocated <= 0 {
		return 0, nil
	}

	ledger := LoadLedger(dataDir)
	totalPaid := int64(0)

	for _, p := range vm.Providers {
		if p.PendingNg <= 0 {
			continue
		}
		share := float64(p.AllocatedGB) / float64(vm.TotalAllocated)
		payout := int64(float64(vm.DeferredPoolNg) * share)
		if !p.Active {
			payout = payout / 2
		}
		if payout > 0 {
			ledger.Mint(p.Pubkey, payout)
			totalPaid += payout
		}
		p.PendingNg = 0
	}

	ledger.Save(dataDir)
	vm.DeferredPoolNg = 0
	vm.LastPayout = time.Now().UTC().Format(time.RFC3339)
	vm.LastPayoutAmount = totalPaid
	vm.PayoutCycle++
	vm.Save(dataDir)
	return totalPaid, nil
}

// MiningBalanceInfo is a summary of the current mining pool balance and profitability.
type MiningBalanceInfo struct {
	DeferredPoolNg    int64   `json:"deferred_pool_ng"`
	TotalAllocatedGB  int64   `json:"total_allocated_gb"`
	UsdValue          float64 `json:"usd_value"`
	TreasuryCommissionPct float64 `json:"treasury_commission_pct"`
}

// MiningProfitability returns the current mining pool balance and estimated USD value.
func (vm *VaultMiningManager) MiningProfitability() MiningBalanceInfo {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	usdValue := float64(vm.DeferredPoolNg) / float64(NGPerTLR) * SilverSpotUSDperOZ
	return MiningBalanceInfo{
		DeferredPoolNg:         vm.DeferredPoolNg,
		TotalAllocatedGB:       vm.TotalAllocated,
		UsdValue:               usdValue,
		TreasuryCommissionPct:  float64(TreasuryCommissionBPS) / 100.0,
	}
}

// VaultMiningStatus is a full snapshot of the vault mining system.
type VaultMiningStatus struct {
	ActiveProviders  int               `json:"active_providers"`
	TotalAllocatedGB int64             `json:"total_allocated_gb"`
	DeferredPoolNg   int64             `json:"deferred_pool_ng"`
	LastPayout       string            `json:"last_payout"`
	LastPayoutAmount int64             `json:"last_payout_amount"`
	PayoutCycle      int               `json:"payout_cycle"`
	Profitability    MiningBalanceInfo `json:"profitability"`
	Providers        []*VaultProvider  `json:"providers"`
}

// Status returns a full snapshot of the vault mining system state.
func (vm *VaultMiningManager) Status() VaultMiningStatus {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	providers := make([]*VaultProvider, 0, len(vm.Providers))
	for _, p := range vm.Providers {
		providers = append(providers, p)
	}

	return VaultMiningStatus{
		ActiveProviders:  len(vm.Providers),
		TotalAllocatedGB: vm.TotalAllocated,
		DeferredPoolNg:   vm.DeferredPoolNg,
		LastPayout:       vm.LastPayout,
		LastPayoutAmount: vm.LastPayoutAmount,
		PayoutCycle:      vm.PayoutCycle,
		Profitability:    MiningBalanceInfo{},
		Providers:        providers,
	}
}
