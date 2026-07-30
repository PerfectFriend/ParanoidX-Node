// Package economy — Buyback Manager: выкуп банкнот казной.
// Common: −2.28% дисконт (9772/10000 от номинала).
// Rare/Epic/Legendary: по номиналу (par).
// Golden/Special/Genesis: только через аукцион (price = 0).
// После выкупа банкнота сжигается (burned_serials.txt) и перечеканивается (re-mint).
package economy

import (
		"fmt"
		"path/filepath"
	"time"

	"ParanoidX/internal/fileutil"
)

// BuybackQuote is the price quote for buying back a single banknote.
type BuybackQuote struct {
	Serial        string `json:"serial"`
	FaceValueNG   int64  `json:"face_value_ng"`
	Rarity        string `json:"rarity"`
	BuybackPriceNG int64 `json:"buyback_price_ng"`
	Discount      string `json:"discount"`
}

// BuybackRecord logs a completed buyback transaction.
type BuybackRecord struct {
	Serial      string `json:"serial"`
	Holder      string `json:"holder"`
	PriceNG     int64  `json:"price_ng"`
	BurnedAt    string `json:"burned_at"`
	ReMintSerial string `json:"re_mint_serial,omitempty"`
}

// BuybackManager handles banknote buyback quoting and execution.
type BuybackManager struct{}

// NewBuybackManager создаёт новый BuybackManager.
func NewBuybackManager() *BuybackManager {
	return &BuybackManager{}
}

// Quote рассчитывает цену выкупа банкноты казной. Common: −2.28% дисконт от номинала.
// Rare/Epic/Legendary: по номиналу (par). Golden/Genesis/Special: только через аукцион.
func (bm *BuybackManager) Quote(banknote BanknoteV2) BuybackQuote {
	discount := "0%"
	price := banknote.DenominationNg

	switch banknote.Rarity {
	case "common", "rare", "epic", "legendary":
		// Все редкости: казна удерживает 2.28%
		price = banknote.DenominationNg * (10000 - TreasuryCommissionBPS) / 10000
		discount = "-2.28%"
	case "golden":
		discount = "only auction"
		price = 0
	case "genesis":
		discount = "only auction"
		price = 0
	default:
		if banknote.SpecialSeries != "" {
			discount = "only auction"
			price = 0
		}
	}

	return BuybackQuote{
		Serial:         banknote.Serial,
		FaceValueNG:    banknote.DenominationNg,
		Rarity:         banknote.Rarity,
		BuybackPriceNG: price,
		Discount:       discount,
	}
}

// Execute выполняет выкуп банкноты: сжигает её (burned_serials.txt), создаёт re-mint копию
// в статусе "pre_mint", списывает цену выкупа со счёта холдера в казну, сохраняет запись о выкупе.
func (bm *BuybackManager) Execute(dataDir string, banknote BanknoteV2, holderPubkey string) (*BuybackRecord, error) {
	quote := bm.Quote(banknote)
	if quote.BuybackPriceNG <= 0 {
		return nil, fmt.Errorf("banknote %s not eligible for buyback (%s)", banknote.Serial, quote.Discount)
	}

	ledger := LoadLedger(dataDir)
	bal := ledger.Balance(holderPubkey)
	if bal < quote.BuybackPriceNG {
		return nil, fmt.Errorf("insufficient balance: have %d, need %d", bal, quote.BuybackPriceNG)
	}

	banknotes, err := LoadBanknotesV2(dataDir)
	if err != nil {
		return nil, fmt.Errorf("load banknotes: %w", err)
	}

	found := false
	for i := range banknotes {
		if banknotes[i].Serial == banknote.Serial && banknotes[i].Holder == holderPubkey && banknotes[i].Status == "active" {
			banknotes[i].Status = "burned"
			banknotes[i].Holder = ""
			banknotes[i].FrozenNg = 0
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("banknote not found or not owned by holder")
	}

	reMintSerial := fmt.Sprintf("RM-%s-%d", time.Now().Format("20060102"), time.Now().UnixMilli()%100000)
	newBanknote := BanknoteV2{
		Serial:         reMintSerial,
		DenominationNg: banknote.DenominationNg,
		Rarity:         banknote.Rarity,
		Multiplier:     banknote.Multiplier,
		Holder:         "",
		Status:         "pre_mint",
		IsGolden:       banknote.IsGolden,
		SpecialSeries:  banknote.SpecialSeries,
		MintedAt:       time.Now().UTC().Format(time.RFC3339),
	}
	banknotes = append(banknotes, newBanknote)
	SaveBanknotesV2(dataDir, banknotes)

	ledger.Transfer(holderPubkey, "treasury", quote.BuybackPriceNG)
	ledger.Save(dataDir)

	burned := LoadBurnedSerials(dataDir)
	burned[banknote.Serial] = true
	SaveBurnedSerials(dataDir, burned)

	record := &BuybackRecord{
		Serial:       banknote.Serial,
		Holder:       holderPubkey,
		PriceNG:      quote.BuybackPriceNG,
		BurnedAt:     time.Now().UTC().Format(time.RFC3339),
		ReMintSerial: reMintSerial,
	}

	records := LoadBuybackRecords(dataDir)
	records = append(records, *record)
	SaveBuybackRecords(dataDir, records)

	return record, nil
}

// LoadBuybackRecords загружает историю выкупов из JSON-файла.
func LoadBuybackRecords(dataDir string) []BuybackRecord {
	var records []BuybackRecord
	fileutil.ReadJSON(filepath.Join(dataDir, "buyback_records.json"), &records)
	if records == nil {
		records = []BuybackRecord{}
	}
	return records
}

// SaveBuybackRecords сохраняет историю выкупов в JSON-файл.
func SaveBuybackRecords(dataDir string, records []BuybackRecord) {
	p := filepath.Join(dataDir, "buyback_records.json")
	fileutil.WriteJSON(p, records)
}

// SaveBurnedSerials persists the set of burned serial numbers to a text file.
func SaveBurnedSerials(dataDir string, burned map[string]bool) {
	p := filepath.Join(dataDir, "burned_serials.txt")
	var lines string
	for s := range burned {
		lines += s + "\n"
	}
	fileutil.WriteFile(p, []byte(lines), 0600)
}
