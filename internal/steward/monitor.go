// Package steward implements the Steward AI service
package steward

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"ParanoidX/internal/economy"
)

// StewardMetrics holds all collected economy metrics.
type StewardMetrics struct {
	mu                   sync.RWMutex
	CollectedAt          time.Time `json:"collected_at"`
	SilverReserveNg      int64     `json:"silver_reserve_ng"`
	TotalIssuedNg        int64     `json:"total_issued_ng"`
	ReserveRatio         float64   `json:"reserve_ratio"`
	TreasuryBalanceNg    int64     `json:"treasury_balance_ng"`
	ActiveBanknotes      int       `json:"active_banknotes"`
	TotalHolders         int       `json:"total_holders"`
	TreasuryTier         string    `json:"treasury_tier"`
	MonthlyOpsNg         int64     `json:"monthly_ops_ng"`
	LedgerTotalSupplyNg  int64     `json:"ledger_total_supply_ng"`
}

// Monitor collects economy metrics on a ticker.
type Monitor struct {
	dataDir string
	metrics *StewardMetrics
	done    chan struct{}
}

// NewMonitor creates a steward monitor.
func NewMonitor(dataDir string) *Monitor {
	return &Monitor{
		dataDir: dataDir,
		metrics: &StewardMetrics{},
		done:    make(chan struct{}),
	}
}

// Start begins the 60s collection loop.
func (m *Monitor) Start() {
	slog.Info("steward monitor started (60s interval)")
	m.Collect()
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				m.Collect()
			case <-m.done:
				return
			}
		}
	}()
}

// Stop halts the monitor.
func (m *Monitor) Stop() {
	close(m.done)
}

// Collect gathers all metrics.
func (m *Monitor) Collect() {
	reserve := int64(0)
	if b, err := os.ReadFile(filepath.Join(m.dataDir, "silver_reserve_ng.txt")); err == nil {
		fmt.Sscanf(string(b), "%d", &reserve)
	}

	totalIssued := int64(0)
	banknotes, _ := economy.LoadBanknotesV2(m.dataDir)
	activeCount := 0
	holders := map[string]bool{}
	for _, b := range banknotes {
		if b.Status == "active" {
			activeCount++
			holders[b.Holder] = true
			totalIssued += b.DenominationNg
		}
	}

	var treasuryBal int64
	if b, err := os.ReadFile(filepath.Join(m.dataDir, "treasury_state.json")); err == nil {
		var state struct{ BalanceNg int64 `json:"balance_ng"` }
		json.Unmarshal(b, &state)
		treasuryBal = state.BalanceNg
	}
	if treasuryBal == 0 {
		treasuryBal = reserve
	}

	ratio := 0.0
	if totalIssued > 0 {
		ratio = float64(reserve) / float64(totalIssued)
	}

	cfg := economy.DefaultTreasuryConfig()
	tier := economy.DetectTier(reserve, cfg)

	ledger := economy.LoadLedger(m.dataDir)

	m.metrics.mu.Lock()
	m.metrics.CollectedAt = time.Now().UTC()
	m.metrics.SilverReserveNg = reserve
	m.metrics.TotalIssuedNg = totalIssued
	m.metrics.ReserveRatio = ratio
	m.metrics.TreasuryBalanceNg = treasuryBal
	m.metrics.ActiveBanknotes = activeCount
	m.metrics.TotalHolders = len(holders)
	m.metrics.TreasuryTier = tier.String()
	m.metrics.MonthlyOpsNg = cfg.MonthlyOpsNg
	m.metrics.LedgerTotalSupplyNg = ledger.TotalSupply
	m.metrics.mu.Unlock()
}

// GetMetrics returns a snapshot of the current metrics.
func (m *Monitor) GetMetrics() *StewardMetrics {
	m.metrics.mu.RLock()
	defer m.metrics.mu.RUnlock()
	return m.metrics
}
