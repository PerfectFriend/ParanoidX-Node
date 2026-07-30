// Package economy implements the island economy system
package economy

import (
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"ParanoidX/internal/fileutil"
)

// DividendWeighted рассчитывает вес банкноты для распределения дивидендов:
// Weight = DenominationNg × RarityWeight.
// Общий вес пула = сумма весов всех активных банкнот.
// Доля держателя = SumWeight(Holder) / SumWeight(All).

// DividendWeight возвращает дивидендный вес банкноты.
func DividendWeight(b BanknoteV2) int64 {
	return b.DenominationNg * int64(RarityWeight(b.Rarity))
}

// HolderDividendShare рассчитывает долю дивидендов для указанного держателя.
// Возвращает долю от 0 до 1 (float64 с точностью до 12 знаков).
func HolderDividendShare(holder string, banknotes []BanknoteV2) float64 {
	totalWeight := int64(0)
	holderWeight := int64(0)
	seen := map[string]bool{}

	for _, b := range banknotes {
		if b.Status != "active" {
			continue
		}
		totalWeight += DividendWeight(b)
		if b.Holder == holder && !seen[b.Serial] {
			holderWeight += DividendWeight(b)
			seen[b.Serial] = true
		}
	}
	if totalWeight <= 0 {
		return 0
	}
	return float64(holderWeight) / float64(totalWeight)
}

// AllHoldersDividendShares возвращает карту держатель → доля дивидендов.
func AllHoldersDividendShares(banknotes []BanknoteV2) map[string]float64 {
	totalWeight := int64(0)
	holderWeights := map[string]int64{}

	for _, b := range banknotes {
		if b.Status != "active" {
			continue
		}
		w := DividendWeight(b)
		totalWeight += w
		holderWeights[b.Holder] += w
	}
	if totalWeight <= 0 {
		return map[string]float64{}
	}

	shares := make(map[string]float64, len(holderWeights))
	for holder, w := range holderWeights {
		shares[holder] = float64(w) / float64(totalWeight)
	}
	return shares
}

// === DividendRound ===

// DividendRound запись о выплате дивидендов.
type DividendRound struct {
	RoundID    string             `json:"round_id"`
	TotalNg    int64              `json:"total_ng"`
	PaidAt     string             `json:"paid_at"`
	Payments   []DividendPayment  `json:"payments"`
	SourceRound string            `json:"source_round"` // ID silver round
}

// DividendPayment выплата одному держателю.
type DividendPayment struct {
	Holder     string `json:"holder"`
	Serial     string `json:"serial"`
	DenomNg    int64  `json:"denom_ng"`
	Rarity     string `json:"rarity"`
	Weight     int64  `json:"weight"`
	DividendNg int64  `json:"dividend_ng"`
}

// DividendDistributor распределяет дивиденды по редкости и номиналу.
type DividendDistributor struct {
	mu     sync.Mutex
	Rounds []DividendRound `json:"rounds"`
	NextID int             `json:"next_id"`
}

// NewDividendDistributor creates a distributor with round counter starting at 1.
func NewDividendDistributor() *DividendDistributor {
	return &DividendDistributor{NextID: 1}
}

// LoadDividendRounds loads dividend round history from disk.
func LoadDividendRounds(dataDir string) *DividendDistributor {
	dd := NewDividendDistributor()
	fileutil.ReadJSON(filepath.Join(dataDir, "dividend_rounds.json"), dd)
	if dd.Rounds == nil {
		dd.Rounds = []DividendRound{}
	}
	return dd
}

// Save persists dividend rounds to JSON.
func (dd *DividendDistributor) Save(dataDir string) {
	fileutil.WriteJSON(filepath.Join(dataDir, "dividend_rounds.json"), dd)
}

// Distribute выплачивает dividendNg из пула всем держателям банкнот
// пропорционально DividendWeight.
// Возвращает DividendRound с разбивкой по каждому держателю.
func (dd *DividendDistributor) Distribute(dataDir string, dividendNg int64, sourceRound string) (*DividendRound, error) {
	dd.mu.Lock()
	defer dd.mu.Unlock()

	if dividendNg <= 0 {
		return nil, fmt.Errorf("dividend amount must be positive")
	}

	banknotes, err := LoadBanknotesV2(dataDir)
	if err != nil {
		return nil, fmt.Errorf("load banknotes: %w", err)
	}

	shares := AllHoldersDividendShares(banknotes)
	if len(shares) == 0 {
		return nil, fmt.Errorf("no active banknotes to distribute dividends")
	}

	roundID := fmt.Sprintf("DIV-%d", dd.NextID)
	dd.NextID++

	categories := map[string]struct {
		denom   int64
		rarity  string
		holder  string
		serial  string
	}{}

	// Build a map of each active banknote
	for _, b := range banknotes {
		if b.Status != "active" {
			continue
		}
		categories[b.Serial] = struct {
			denom   int64
			rarity  string
			holder  string
			serial  string
		}{
			denom:  b.DenominationNg,
			rarity: b.Rarity,
			holder: b.Holder,
			serial: b.Serial,
		}
	}

	// Accumulate per-holder totals
	holderTotals := map[string]int64{}
	for holder, share := range shares {
		holderTotals[holder] = int64(float64(dividendNg) * share)
	}

	var payments []DividendPayment
	totalPaid := int64(0)

	// Create per-banknote payment items
	seenHolders := map[string]bool{}
	for _, cat := range categories {
		if seenHolders[cat.holder] {
			// Only one consolidated payment per holder
			continue
		}
		seenHolders[cat.holder] = true
		amt := holderTotals[cat.holder]
		if amt <= 0 {
			continue
		}
		totalPaid += amt
		weight := DividendWeight(BanknoteV2{
			DenominationNg: cat.denom,
			Rarity:         cat.rarity,
		})
		payments = append(payments, DividendPayment{
			Holder:     cat.holder,
			Serial:     cat.serial,
			DenomNg:    cat.denom,
			Rarity:     cat.rarity,
			Weight:     weight,
			DividendNg: amt,
		})
	}

	round := DividendRound{
		RoundID:     roundID,
		TotalNg:     totalPaid,
		PaidAt:      time.Now().UTC().Format(time.RFC3339),
		Payments:    payments,
		SourceRound: sourceRound,
	}
	dd.Rounds = append(dd.Rounds, round)
	dd.Save(dataDir)

	// Mint dividends to holder ledgers
	ledger := LoadLedger(dataDir)
	for _, p := range payments {
		ledger.Mint(p.Holder, p.DividendNg)
	}
	ledger.Save(dataDir)

	return &round, nil
}

// History возвращает последние N раундов дивидендов.
func (dd *DividendDistributor) History(n int) []DividendRound {
	dd.mu.Lock()
	defer dd.mu.Unlock()
	if n <= 0 || n > len(dd.Rounds) {
		n = len(dd.Rounds)
	}
	return dd.Rounds[len(dd.Rounds)-n:]
}
