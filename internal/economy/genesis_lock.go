// Package economy — Genesis Lock & Deflation Engine.
//
// 9 карт генезиса заморожены до достижения TreasurySurplus >= 12× MonthlyOps.
// Замороженные дивиденды накапливаются с первого минта — чистая дефляция.
// При разморозке выплачиваются держателям genesis-токенов.
//
// Дефляционные драйверы (все работают одновременно):
//   1. Genesis Lock: дивиденды накапливаются, НЕ циркулируют
//   2. ICO: ng за токены уходят из оборота навсегда
//   3. Advertising: 20% цены тега сжигается
//   4. Vault Mining: выплаты отложены на 7 дней
//   5. Treasury Surplus Threshold: ng заморожены в казне
//   6. Deflation at VeryFat: 40% surplus сжигается (oracle.go)
package economy

import (
		"fmt"
	"math"
		"path/filepath"
	"sync"
	"time"

	"ParanoidX/internal/fileutil"
)

const (
	// GenesisSafetyThresholdMonths — сколько месяцев операций нужно для разморозки
	GenesisSafetyThresholdMonths = 12
)

// TreasuryMonthlyOpsNg — оценочные месячные операционные расходы казны.
var TreasuryMonthlyOpsNg int64 = 1000 * NGPerTLR

// GenesisSafetyThreshold возвращает порог профицита для разморозки.
func GenesisSafetyThreshold() int64 {
	return GenesisSafetyThresholdMonths * TreasuryMonthlyOpsNg
}

// GenesisLockState хранит состояние заморозки генезиса.
type GenesisLockState struct {
	mu                    sync.Mutex
	GenesisCards          []string           `json:"genesis_cards"`
	FrozenDividendPoolNg  int64              `json:"frozen_dividend_pool_ng"`
	FrozenAt              string             `json:"frozen_at"`
	UnlockedAt            string             `json:"unlocked_at,omitempty"`
	Unlocked              bool               `json:"unlocked"`
	DividendAccruals      []FrozenAccrual    `json:"dividend_accruals"`
	LastAccrualRound      string             `json:"last_accrual_round"`
	TotalDeflationNg      int64              `json:"total_deflation_ng"`
}

// FrozenAccrual records a single frozen dividend accrual event.
type FrozenAccrual struct {
	Round   string `json:"round"`
	TotalNg int64  `json:"total_ng"`
	Date    string `json:"date"`
}

// NewGenesisLockState creates a genesis lock state frozen from the current time.
func NewGenesisLockState() *GenesisLockState {
	return &GenesisLockState{
		FrozenAt: time.Now().UTC().Format(time.RFC3339),
	}
}

// LoadGenesisLock loads genesis lock state from disk.
func LoadGenesisLock(dataDir string) *GenesisLockState {
	gl := NewGenesisLockState()
	fileutil.ReadJSON(filepath.Join(dataDir, "genesis_lock.json"), gl)
	if gl.GenesisCards == nil {
		gl.GenesisCards = []string{}
	}
	if gl.DividendAccruals == nil {
		gl.DividendAccruals = []FrozenAccrual{}
	}
	return gl
}

// Save persists genesis lock state to JSON.
func (gl *GenesisLockState) Save(dataDir string) {
	p := filepath.Join(dataDir, "genesis_lock.json")
	fileutil.WriteJSON(p, gl)
}

// RegisterGenesisCard adds a genesis banknote serial to the lock list if not already present.
func (gl *GenesisLockState) RegisterGenesisCard(serial string) {
	gl.mu.Lock()
	defer gl.mu.Unlock()
	for _, s := range gl.GenesisCards {
		if s == serial {
			return
		}
	}
	gl.GenesisCards = append(gl.GenesisCards, serial)
}

// IsFrozen returns whether the genesis lock is still active (not yet unlocked).
func (gl *GenesisLockState) IsFrozen() bool {
	gl.mu.Lock()
	defer gl.mu.Unlock()
	return !gl.Unlocked
}

// AccrueFrozenDividend начисляет frozen dividends genesis-картам.
// Возвращает сколько ng заморожено (не выплачено).
func (gl *GenesisLockState) AccrueFrozenDividend(dataDir string, dividendNg int64, roundID string) (int64, error) {
	gl.mu.Lock()
	defer gl.mu.Unlock()
	if gl.Unlocked {
		return 0, nil
	}

	banknotes, err := LoadBanknotesV2(dataDir)
	if err != nil {
		return 0, fmt.Errorf("load banknotes: %w", err)
	}

	totalWeight := int64(0)
	genesisWeight := int64(0)
	for _, b := range banknotes {
		if b.Status != "active" && b.Status != "genesis_locked" {
			continue
		}
		w := DividendWeight(b)
		totalWeight += w
		if b.Rarity == "genesis" {
			genesisWeight += w
		}
	}
	if totalWeight <= 0 {
		return 0, nil
	}

	share := float64(genesisWeight) / float64(totalWeight)
	frozen := int64(float64(dividendNg) * share)
	if frozen <= 0 {
		return 0, nil
	}

	gl.FrozenDividendPoolNg += frozen
	gl.TotalDeflationNg += frozen
	gl.DividendAccruals = append(gl.DividendAccruals, FrozenAccrual{
		Round:   roundID,
		TotalNg: frozen,
		Date:    time.Now().UTC().Format(time.RFC3339),
	})
	gl.LastAccrualRound = roundID
	return frozen, nil
}

// CheckSurplus проверяет профицит казны. Если порог достигнут — разморозка.
func (gl *GenesisLockState) CheckSurplus(treasuryNg int64) bool {
	gl.mu.Lock()
	defer gl.mu.Unlock()
	if gl.Unlocked {
		return true
	}
	threshold := GenesisSafetyThreshold()
	if treasuryNg >= threshold {
		gl.Unlocked = true
		gl.UnlockedAt = time.Now().UTC().Format(time.RFC3339)
		return true
	}
	return false
}

// DistributeFrozenDividends выплачивает frozen dividends держателям genesis.
func (gl *GenesisLockState) DistributeFrozenDividends(dataDir string) (int64, error) {
	gl.mu.Lock()
	defer gl.mu.Unlock()
	if !gl.Unlocked || gl.FrozenDividendPoolNg <= 0 {
		return 0, nil
	}

	banknotes, err := LoadBanknotesV2(dataDir)
	if err != nil {
		return 0, fmt.Errorf("load banknotes: %w", err)
	}

	type holderInfo struct {
		holder string
		weight int64
	}
	genesis := map[string]holderInfo{}
	totalWeight := int64(0)
	for _, b := range banknotes {
		if b.Rarity == "genesis" && (b.Status == "active" || b.Status == "genesis_locked") {
			w := DividendWeight(b)
			totalWeight += w
			genesis[b.Serial] = holderInfo{holder: b.Holder, weight: w}
		}
	}
	if totalWeight <= 0 {
		return 0, nil
	}

	ledger := LoadLedger(dataDir)
	distributed := int64(0)
	for _, info := range genesis {
		amount := int64(float64(gl.FrozenDividendPoolNg) * float64(info.weight) / float64(totalWeight))
		if amount > 0 {
			ledger.Mint(info.holder, amount)
			distributed += amount
		}
	}
	ledger.Save(dataDir)
	gl.FrozenDividendPoolNg -= distributed
	return distributed, nil
}

// GenesisUnlockSummary — статус системы генезиса.
type GenesisUnlockSummary struct {
	Frozen                bool    `json:"frozen"`
	GenesisCardCount      int     `json:"genesis_card_count"`
	FrozenDividendPoolNg  int64   `json:"frozen_dividend_pool_ng"`
	TreasuryNg            int64   `json:"treasury_ng"`
	SafetyThresholdNg     int64   `json:"safety_threshold_ng"`
	SurplusNg             int64   `json:"surplus_ng"`
	ProgressPct           float64 `json:"progress_pct"`
	TotalDeflationNg      int64   `json:"total_deflation_ng"`
	UnlockedAt            string  `json:"unlocked_at,omitempty"`
	LastAccrualRound      string  `json:"last_accrual_round"`
}

// Summary returns a snapshot of the genesis lock system state and progress toward unlock.
func (gl *GenesisLockState) Summary(treasuryNg int64) GenesisUnlockSummary {
	gl.mu.Lock()
	defer gl.mu.Unlock()
	threshold := GenesisSafetyThreshold()
	surplus := treasuryNg - threshold
	progress := 0.0
	if threshold > 0 {
		progress = math.Min(100.0, float64(treasuryNg)/float64(threshold)*100.0)
	}
	return GenesisUnlockSummary{
		Frozen:               !gl.Unlocked,
		GenesisCardCount:     len(gl.GenesisCards),
		FrozenDividendPoolNg: gl.FrozenDividendPoolNg,
		TreasuryNg:           treasuryNg,
		SafetyThresholdNg:    threshold,
		SurplusNg:            surplus,
		ProgressPct:          progress,
		TotalDeflationNg:     gl.TotalDeflationNg,
		UnlockedAt:           gl.UnlockedAt,
		LastAccrualRound:     gl.LastAccrualRound,
	}
}
