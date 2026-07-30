// Package economy — Genesis Tokenization & ICO.
//
// Каждая карта генезиса (1 из 9, rarity="genesis") токенизируется в
// фиксированное количество GenesisToken. Эти токены выставляются на ICO.
//
// Механика:
//   - 1 карта генезиса = GenesisTokensPerCard токенов
//   - ICO проходит в несколько раундов с повышением цены
//   - Средства от ICO идут в казну + AuctionPool
//   - После ICO genesis-карты остаются замороженными (см. genesis_lock.go)
//     до достижения профицита казны
//
// Дефляционный эффект: ng, потраченные на ICO, сжигаются (идут в казну/пулы),
// а genesis-токены представляют право на будущие дивиденды.
package economy

import (
		"fmt"
		"path/filepath"
	"sync"
	"time"

	"ParanoidX/internal/fileutil"
)

// GenesisTokensPerCard — количество токенов генезиса на 1 карту.
const GenesisTokensPerCard int64 = 1_000_000

// ICORoundPrice — цена токена в ng на каждом раунде ICO.
// Доступные цены: $0.024 → $0.06 → $0.12 → $0.24 за токен.
var ICORoundPrices = []int64{
	10_000,  // Round 1: $0.024/token — вход для всех
	25_000,  // Round 2: $0.06/token
	50_000,  // Round 3: $0.12/token
	100_000, // Round 4 (public): $0.24/token
}

// GenesisToken представляет один токен генезиса.
type GenesisToken struct {
	TokenID      string `json:"token_id"`
	GenesisSerial string `json:"genesis_serial"` // serial исходной карты
	Holder       string `json:"holder"`
	Round        int    `json:"round"` // ICO round (1-4)
	PriceNg      int64  `json:"price_ng"`
	PurchasedAt  string `json:"purchased_at"`
}

// GenesisICOManager управляет ICO и реестром токенов.
type GenesisICOManager struct {
	mu            sync.Mutex
	Tokens        []GenesisToken `json:"tokens"`
	NextTokenID   int64          `json:"next_token_id"`
	CurrentRound  int            `json:"current_round"`   // 1-4, 0 = not started, 5 = finished
	TotalRaisedNg int64          `json:"total_raised_ng"`
	StartedAt     string         `json:"started_at,omitempty"`
	FinishedAt    string         `json:"finished_at,omitempty"`
}


// NewGenesisICOManager handles the NewGenesisICOManager HTTP request.
func NewGenesisICOManager() *GenesisICOManager {
	return &GenesisICOManager{
		NextTokenID: 1,
	}
}


// LoadGenesisICO handles the LoadGenesisICO HTTP request.
func LoadGenesisICO(dataDir string) *GenesisICOManager {
	ico := NewGenesisICOManager()

	fileutil.ReadJSON(filepath.Join(dataDir, "genesis_ico.json"), ico)
	if ico.Tokens == nil {
		ico.Tokens = []GenesisToken{}
	}
	return ico
}


// Save handles the Save HTTP request.
func (ico *GenesisICOManager) Save(dataDir string) {
	p := filepath.Join(dataDir, "genesis_ico.json")
	fileutil.WriteJSON(p, ico)
}

// StartICO инициализирует ICO с genesis-картами из реестра.
func (ico *GenesisICOManager) StartICO(dataDir string) error {
	ico.mu.Lock()
	defer ico.mu.Unlock()

	if ico.StartedAt != "" {
		return fmt.Errorf("ICO already started at %s", ico.StartedAt)
	}

	banknotes, err := LoadBanknotesV2(dataDir)
	if err != nil {
		return fmt.Errorf("load banknotes: %w", err)
	}

	genesisCount := 0
	for _, b := range banknotes {
		if b.Rarity == "genesis" {
			genesisCount++
		}
	}
	if genesisCount == 0 {
		return fmt.Errorf("no genesis cards found to tokenize")
	}

	ico.StartedAt = time.Now().UTC().Format(time.RFC3339)
	ico.CurrentRound = 1
	return nil
}

// BuyTokens покупает токены генезиса текущего раунда.
// Возвращает купленные токены.
func (ico *GenesisICOManager) BuyTokens(dataDir, buyer string, amount int64, ledger *Ledger) ([]GenesisToken, error) {
	ico.mu.Lock()
	defer ico.mu.Unlock()

	if ico.StartedAt == "" {
		return nil, fmt.Errorf("ICO not started yet")
	}
	if ico.FinishedAt != "" {
		return nil, fmt.Errorf("ICO finished at %s", ico.FinishedAt)
	}
	if amount <= 0 {
		return nil, fmt.Errorf("amount must be positive")
	}

	roundIdx := ico.CurrentRound - 1
	if roundIdx < 0 || roundIdx >= len(ICORoundPrices) {
		return nil, fmt.Errorf("ICO round %d has no price", ico.CurrentRound)
	}

	pricePerToken := ICORoundPrices[roundIdx]
	totalCost := pricePerToken * amount

	if ledger.Balance(buyer) < totalCost {
		return nil, fmt.Errorf("insufficient balance: need %d ng, have %d ng", totalCost, ledger.Balance(buyer))
	}

	// Найти доступные genesis-карты для токенизации
	banknotes, err := LoadBanknotesV2(dataDir)
	if err != nil {
		return nil, fmt.Errorf("load banknotes: %w", err)
	}

	var genesisSerials []string
	for _, b := range banknotes {
		if b.Rarity == "genesis" {
			genesisSerials = append(genesisSerials, b.Serial)
		}
	}
	if len(genesisSerials) == 0 {
		return nil, fmt.Errorf("no genesis cards available")
	}

	// Чередуем genesis-карты между токенами
	var purchased []GenesisToken
	for i := int64(0); i < amount; i++ {
		gs := genesisSerials[i%int64(len(genesisSerials))]
		token := GenesisToken{
			TokenID:       fmt.Sprintf("GT-%d", ico.NextTokenID),
			GenesisSerial: gs,
			Holder:        buyer,
			Round:         ico.CurrentRound,
			PriceNg:       pricePerToken,
			PurchasedAt:   time.Now().UTC().Format(time.RFC3339),
		}
		ico.NextTokenID++
		ico.Tokens = append(ico.Tokens, token)
		purchased = append(purchased, token)
	}

	// Снять средства, распределить
	ledger.Mint(buyer, -totalCost)
	ledger.Mint("treasury", totalCost/2)
	ledger.Mint("auction_pool", totalCost/2)
	ledger.Save(dataDir)

	ico.TotalRaisedNg += totalCost

	// Проверить, не пора ли перейти на следующий раунд
	ico.advanceRound()

	ico.Save(dataDir)
	return purchased, nil
}

func (ico *GenesisICOManager) advanceRound() {
	// Переход на следующий раунд когда продано достаточно токенов
	tokensInRound := int64(0)
	for _, t := range ico.Tokens {
		if t.Round == ico.CurrentRound {
			tokensInRound++
		}
	}
	// Каждый раунд продаёт 25% всех токенов
	expectedPerRound := GenesisTokensPerCard * 9 / 4 // 9 genesis cards, 4 rounds
	if tokensInRound >= expectedPerRound && ico.CurrentRound < len(ICORoundPrices) {
		ico.CurrentRound++
	}
	if ico.CurrentRound > len(ICORoundPrices) {
		ico.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	}
}

// HolderTokens возвращает все токены указанного держателя.
func (ico *GenesisICOManager) HolderTokens(holder string) []GenesisToken {
	ico.mu.Lock()
	defer ico.mu.Unlock()
	var out []GenesisToken
	for _, t := range ico.Tokens {
		if t.Holder == holder {
			out = append(out, t)
		}
	}
	return out
}

// ICOStatus возвращает статус ICO.
type ICOStatus struct {
	Started          bool   `json:"started"`
	Finished         bool   `json:"finished"`
	CurrentRound     int    `json:"current_round"`
	TotalRounds      int    `json:"total_rounds"`
	TotalRaisedNg    int64  `json:"total_raised_ng"`
	TokensSold       int    `json:"tokens_sold"`
	CurrentPriceNg   int64  `json:"current_price_ng"`
	StartedAt        string `json:"started_at,omitempty"`
	FinishedAt       string `json:"finished_at,omitempty"`
}


// Status handles the Status HTTP request.
func (ico *GenesisICOManager) Status() ICOStatus {
	ico.mu.Lock()
	defer ico.mu.Unlock()

	s := ICOStatus{
		Started:      ico.StartedAt != "",
		Finished:     ico.FinishedAt != "",
		CurrentRound: ico.CurrentRound,
		TotalRounds:  len(ICORoundPrices),
		TotalRaisedNg: ico.TotalRaisedNg,
		TokensSold:   len(ico.Tokens),
		StartedAt:    ico.StartedAt,
		FinishedAt:   ico.FinishedAt,
	}
	if ico.CurrentRound > 0 && ico.CurrentRound <= len(ICORoundPrices) {
		s.CurrentPriceNg = ICORoundPrices[ico.CurrentRound-1]
	}
	return s
}
