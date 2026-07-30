// Package economy — реестр банкнот (BanknoteV2), pre-mint, аудиторы, казна, долг, инвентарь.
// Все данные хранятся в JSON-файлах в dataDir, атомарно читаются/пишутся.
// Миграция BanknoteV1 → V2 автоматическая при старте: читает banknotes_registry.json,
// конвертирует denomination из TLR в ng, определяет rarity по префиксу серийного номера,
// сохраняет banknotes_registry_v2.json и переименовывает V1-файл в .bak.
//
// Также включает:
//   - PreMintManager — управление pre-mint реестром (доступные/зарезервированные/использованные серии)
//   - FirstInvestorDebt — отслеживание и автоматическое погашение долга первому инвестору
//   - Auditor — топ-10 держателей (первый инвестор навсегда + топ-9 по балансу + manual)
//   - Treasury — динамическое распределение 20% казны (ops/reserve/insurance/burn)
//   - Inventory — пер-пользовательские паки банкнот
package economy

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"ParanoidX/internal/fileutil"
)

// BanknoteV2 — новая структура банкноты.
type BanknoteV2 struct {
	Serial          string           `json:"serial"`
	DenominationNg  int64            `json:"denomination_ng"`
	Rarity          string           `json:"rarity"` // common, rare, epic, legendary, genesis
	Multiplier      int              `json:"multiplier"`
	Holder          string           `json:"holder"`
	FrozenNg        int64            `json:"frozen_ng"`
	Status          string           `json:"status"` // active, burned, genesis_reserved, escrow, pending_auction
	IsGolden        bool             `json:"is_golden"`
	SpecialSeries   string           `json:"special_series,omitempty"`
	DividendHistory []DividendRecord `json:"dividend_history"`
	MintedAt        string           `json:"minted_at"`
	PdfHash         string           `json:"pdf_hash,omitempty"`
}

type DividendRecord struct {
	Round int   `json:"round"`
	Ng    int64 `json:"ng"`
	Date  string `json:"date"`
}

// BanknoteV1 — старая структура для миграции.
type BanknoteV1 struct {
	Serial          string  `json:"serial"`
	DenominationTLR float64 `json:"denomination_tlr"`
	Holder          string  `json:"holder"`
	AccruedNg       int64   `json:"accrued_ng,omitempty"`
	Claimed         bool    `json:"claimed"`
	RegisteredAt    string  `json:"registered_at"`
}

var rarityMultiplier = map[string]int{
	"common":    1,
	"rare":      2,
	"epic":      3,
	"legendary": 4,
	"genesis":   5,
}

var rarityOrder = map[string]int{
	"common":    0,
	"rare":      1,
	"epic":      2,
	"legendary": 3,
	"genesis":   4,
}

// LoadBanknotesV2 загружает реестр v2, с миграцией если нужно.
func LoadBanknotesV2(dataDir string) ([]BanknoteV2, error) {
	p := filepath.Join(dataDir, "banknotes_registry_v2.json")
	if b, err := os.ReadFile(p); err == nil {
		var v2 []BanknoteV2
		if err := json.Unmarshal(b, &v2); err == nil {
			return v2, nil
		}
	}

	// Пробуем мигрировать v1
	v1p := filepath.Join(dataDir, "banknotes_registry.json")
	if b, err := os.ReadFile(v1p); err == nil {
		var v1 []BanknoteV1
		if err := json.Unmarshal(b, &v1); err == nil && len(v1) > 0 {
			v2 := migrateV1toV2(v1)
			// Сохраняем v2
			fileutil.WriteJSON(p, v2)
			// Переименовываем v1 в .bak
			os.Rename(v1p, v1p+".bak")
			return v2, nil
		}
	}

	return []BanknoteV2{}, nil
}

// SaveBanknotesV2 сохраняет реестр v2.
func SaveBanknotesV2(dataDir string, banknotes []BanknoteV2) {
	p := filepath.Join(dataDir, "banknotes_registry_v2.json")
	fileutil.WriteJSON(p, banknotes)
}

func migrateV1toV2(v1 []BanknoteV1) []BanknoteV2 {
	var v2 []BanknoteV2
	for _, b := range v1 {
		// Определяем rarity из serial префикса
		rarity := detectRarityFromSerial(b.Serial)
		// Конвертируем denom: 1 TLR = NGPerTLR ng
		denomNg := int64(b.DenominationTLR * float64(NGPerTLR))
		if denomNg < NGPerTLR {
			denomNg = NGPerTLR // минимум 1 TLR
		}

		dh := []DividendRecord{}
		if b.AccruedNg > 0 {
			dh = append(dh, DividendRecord{
				Round: 0,
				Ng:    b.AccruedNg,
				Date:  b.RegisteredAt,
			})
		}

		v2 = append(v2, BanknoteV2{
			Serial:          b.Serial,
			DenominationNg:  denomNg,
			Rarity:          rarity,
			Multiplier:      rarityMultiplier[rarity],
			Holder:          b.Holder,
			FrozenNg:        denomNg,
			Status:          "active",
			DividendHistory: dh,
			MintedAt:        b.RegisteredAt,
		})
	}
	return v2
}

func detectRarityFromSerial(serial string) string {
	s := serial
	// Ищем префикс: MB-COMMON, MB-RARE, MB-EPIC, MB-LEGENDARY, MB-FIRST (genesis)
	if stringsContains(s, "GENESIS") || stringsContains(s, "FIRST") {
		return "genesis"
	}
	if stringsContains(s, "LEGENDARY") || stringsContains(s, "LEGEND") {
		return "legendary"
	}
	if stringsContains(s, "EPIC") {
		return "epic"
	}
	if stringsContains(s, "RARE") {
		return "rare"
	}
	return "common"
}

func stringsContains(s, substr string) bool {
	return len(s) >= len(substr) && containsSubstring(s, substr)
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// GetHolderBanknotes возвращает все банкноты пользователя.
func GetHolderBanknotes(banknotes []BanknoteV2, holder string) []BanknoteV2 {
	var result []BanknoteV2
	for _, b := range banknotes {
		if b.Holder == holder && (b.Status == "active" || b.Status == "genesis_locked") {
			result = append(result, b)
		}
	}
	return result
}

// CalculateWeightedDenom считает сумму denom * multiplier для всех активных банкнот.
func CalculateWeightedDenom(banknotes []BanknoteV2) int64 {
	var total int64
	for _, b := range banknotes {
		if b.Status == "active" || b.Status == "genesis_locked" {
			total += b.DenominationNg * int64(b.Multiplier)
		}
	}
	return total
}

// CalculateDividendForHolder считает дивиденд одному пользователю.
func CalculateDividendForHolder(banknotes []BanknoteV2, poolNg int64) map[string]int64 {
	// Сначала проверяем наличие Golden Royal у холдера
	totalWeighted := CalculateWeightedDenom(banknotes)
	if totalWeighted == 0 {
		return nil
	}

	result := make(map[string]int64) // serial -> ng
	hasGolden := false
	holderPubkey := ""
	for _, b := range banknotes {
		if b.IsGolden && b.Status == "active" {
			hasGolden = true
			holderPubkey = b.Holder
		}
	}

	for _, b := range banknotes {
		if b.Status != "active" {
			continue
		}
		mult := int64(b.Multiplier)
		if hasGolden && b.Holder == holderPubkey {
			mult = mult * 4 // Golden Royal: ×4 ко всем банкнотам в кошельке
		}
		if b.SpecialSeries != "" {
			mult = 1 // Special series не получают множителя
		}
		share := float64(b.DenominationNg*mult) / float64(totalWeighted)
		divNg := int64(float64(poolNg) * share)
		result[b.Serial] = divNg
	}
	return result
}

// --- Pre-mint registry ---

type PreMintEntry struct {
	Serial         string `json:"serial"`
	DenominationNg int64  `json:"denomination_ng"`
	Rarity         string `json:"rarity"`
	Status         string `json:"status"` // available, genesis_reserved, promoted
	PdfPath        string `json:"pdf_path"`
	SigPath        string `json:"sig_path"`
	AddedAt        string `json:"added_at"`
}

// LoadPreMint загружает pre-mint registry.
func LoadPreMint(dataDir string) []PreMintEntry {
	var entries []PreMintEntry
	fileutil.ReadJSON(filepath.Join(dataDir, "pre-mint-registry.json"), &entries)
	if entries == nil {
		entries = []PreMintEntry{}
	}
	return entries
}


// SavePreMint handles the SavePreMint HTTP request.
func SavePreMint(dataDir string, entries []PreMintEntry) {
	p := filepath.Join(dataDir, "pre-mint-registry.json")
	fileutil.WriteJSON(p, entries)
}

type PreMintManager struct{}


// NewPreMintManager handles the NewPreMintManager HTTP request.
func NewPreMintManager() *PreMintManager {
	return &PreMintManager{}
}


// MarkUsed handles the MarkUsed HTTP request.
func (pm *PreMintManager) MarkUsed(dataDir string, serials []string) {
	entries := LoadPreMint(dataDir)
	for i := range entries {
		for _, s := range serials {
			if entries[i].Serial == s && entries[i].Status == "available" {
				entries[i].Status = "promoted"
			}
		}
	}
	SavePreMint(dataDir, entries)
}

// --- Genesis reserved ---

// ReserveGenesisEntries помечает pre-mint entry как genesis_reserved.
func ReserveGenesisEntries(entries []PreMintEntry, serials []string) bool {
	found := 0
	for i := range entries {
		for _, s := range serials {
			if entries[i].Serial == s && entries[i].Status == "available" {
				entries[i].Status = "genesis_reserved"
				found++
			}
		}
	}
	return found == len(serials)
}

// --- Burned serials ---

// LoadBurnedSerials загружает список сожжённых серий.
func LoadBurnedSerials(dataDir string) map[string]bool {
	result := make(map[string]bool)
	p := filepath.Join(dataDir, "burned_serials.txt")
	if b, err := os.ReadFile(p); err == nil {
		for _, line := range stringsSplit(string(b), "\n") {
			line = stringsTrimSpace(line)
			if line != "" {
				result[line] = true
			}
		}
	}
	return result
}

func stringsTrimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

func stringsSplit(s, sep string) []string {
	var result []string
	for {
		i := containsSubstringIndex(s, sep)
		if i < 0 {
			result = append(result, s)
			break
		}
		result = append(result, s[:i])
		s = s[i+len(sep):]
	}
	return result
}

func containsSubstringIndex(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// SaveBurnedSerial добавляет серию в список сожжённых.
func SaveBurnedSerial(dataDir string, serial string) {
	p := filepath.Join(dataDir, "burned_serials.txt")
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("SaveBurnedSerial: open %s: %v", p, err)
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s\n", serial)
}

// --- Debt first investor ---

type FirstInvestorDebt struct {
	PrincipalUSDT float64 `json:"principal_usdt"`
	PrincipalNG   int64   `json:"principal_ng"`
	IssuedAt      string  `json:"issued_at"`
	RepaidAt      string  `json:"repaid_at,omitempty"`
}


// LoadDebt handles the LoadDebt HTTP request.
func LoadDebt(dataDir string) *FirstInvestorDebt {
	d := &FirstInvestorDebt{}
	fileutil.ReadJSON(filepath.Join(dataDir, "debt_first_investor.json"), d)
	return d
}


// SaveDebt handles the SaveDebt HTTP request.
func SaveDebt(dataDir string, d *FirstInvestorDebt) {
	p := filepath.Join(dataDir, "debt_first_investor.json")
	fileutil.WriteJSON(p, d)
}

// --- Auditor tokens ---

const MaxTopAuditors = 9 // топ-9 держателей + первый инвестор = 10 всего

type AuditorEntry struct {
	Pubkey    string `json:"pubkey"`
	Label     string `json:"label"`
	GrantedAt string `json:"granted_at"`
	// Тип аудитора:
	//   "first_investor" — первый инвестор, навсегда
	//   "top_holder"     — топ-9 по балансу, автоматически обновляется
	//   "manual"         — выдано вручную Royal
	Type string `json:"type,omitempty"`
}


// LoadAuditors handles the LoadAuditors HTTP request.
func LoadAuditors(dataDir string) []AuditorEntry {
	var auditors []AuditorEntry
	fileutil.ReadJSON(filepath.Join(dataDir, "auditor_tokens.json"), &auditors)
	if auditors == nil {
		auditors = []AuditorEntry{}
	}
	return auditors
}


// SaveAuditors handles the SaveAuditors HTTP request.
func SaveAuditors(dataDir string, auditors []AuditorEntry) {
	// Убираем дубликаты по pubkey
	seen := make(map[string]bool)
	deduped := []AuditorEntry{}
	for _, a := range auditors {
		if !seen[a.Pubkey] {
			seen[a.Pubkey] = true
			deduped = append(deduped, a)
		}
	}
	p := filepath.Join(dataDir, "auditor_tokens.json")
	fileutil.WriteJSON(p, deduped)
}


// IsAuditor handles the IsAuditor HTTP request.
func IsAuditor(dataDir, pubkey string) bool {
	for _, a := range LoadAuditors(dataDir) {
		if a.Pubkey == pubkey {
			return true
		}
	}
	return false
}

// RefreshTopAuditors обновляет список топ-9 держателей + сохраняет первого инвестора.
// Вызывается принудительно через API или автоматически при каждом раунде.
func RefreshTopAuditors(dataDir string) []AuditorEntry {
	ledger := LoadLedger(dataDir)
	currentAuditors := LoadAuditors(dataDir)

	// Собираем список pubkey с балансами
	type balanceEntry struct {
		pubkey string
		ng     int64
	}
	var allBalances []balanceEntry
	for pubkey, acc := range ledger.Accounts {
		if acc.BalanceNg > 0 {
			allBalances = append(allBalances, balanceEntry{pubkey: pubkey, ng: acc.BalanceNg})
		}
	}

	// Сортируем по убыванию баланса (пузырьком для простоты, на малых данных норм)
	for i := 0; i < len(allBalances); i++ {
		for j := i + 1; j < len(allBalances); j++ {
			if allBalances[j].ng > allBalances[i].ng {
				allBalances[i], allBalances[j] = allBalances[j], allBalances[i]
			}
		}
	}

	// Берём топ-9
	newAuditors := []AuditorEntry{}
	added := make(map[string]bool)

	// 1. Сохраняем первого инвестора если он есть
	for _, a := range currentAuditors {
		if a.Type == "first_investor" {
			newAuditors = append(newAuditors, a)
			added[a.Pubkey] = true
			break
		}
	}

	// 2. Добавляем топ-9 по балансу
	topCount := 0
	for _, be := range allBalances {
		if topCount >= MaxTopAuditors {
			break
		}
		if added[be.pubkey] {
			continue
		}
		newAuditors = append(newAuditors, AuditorEntry{
			Pubkey:    be.pubkey,
			Label:     fmt.Sprintf("Top Holder #%d", topCount+1),
			GrantedAt: time.Now().Format(time.RFC3339),
			Type:      "top_holder",
		})
		added[be.pubkey] = true
		topCount++
	}

	// 3. Добавляем manual аудиторов (которые не top_holder и не first_investor)
	for _, a := range currentAuditors {
		if a.Type == "manual" && !added[a.Pubkey] {
			newAuditors = append(newAuditors, a)
			added[a.Pubkey] = true
		}
	}

	SaveAuditors(dataDir, newAuditors)
	return newAuditors
}

// --- Debt repayment check ---

// CheckAndRepayDebt проверяет условия погашения долга первому инвестору.
func CheckAndRepayDebt(dataDir string, treasuryNg int64, monthlyOpsNg int64) (string, error) {
	d := LoadDebt(dataDir)
	if d.RepaidAt != "" {
		return "already_repaid", nil
	}
	if d.PrincipalNG == 0 {
		return "no_debt", nil
	}

	// Порог: казна > 3× месячной операционки И > суммы долга
	if treasuryNg > monthlyOpsNg*3 && treasuryNg > d.PrincipalNG {
		// Погашаем
		ledger := LoadLedger(dataDir)
		// Зачисляем долг на счёт первого инвестора (ищем по типу)
		auditors := LoadAuditors(dataDir)
		investorPubkey := ""
		for _, a := range auditors {
			if a.Type == "first_investor" {
				investorPubkey = a.Pubkey
				break
			}
		}
		if investorPubkey == "" {
			return "no_investor_found", fmt.Errorf("first_investor auditor not found")
		}
		ledger.Mint(investorPubkey, d.PrincipalNG)
		ledger.Save(dataDir)

		d.RepaidAt = time.Now().Format(time.RFC3339)
		SaveDebt(dataDir, d)
		return "repaid", nil
	}
	return "pending", nil
}

// --- Inventory packs ---

type Pack struct {
	PackID    string   `json:"pack_id"`
	Sealed    bool     `json:"sealed"`
	Banknotes []string `json:"banknotes"` // serials
	PriceNg   int64    `json:"price_ng"`
	Owner     string   `json:"owner"`
	CreatedAt string   `json:"created_at"`
	PackType  string   `json:"pack_type,omitempty"` // "genesis", "booster", etc.
}


// LoadInventory handles the LoadInventory HTTP request.
func LoadInventory(dataDir, pubkey string) []Pack {
	var inv []Pack
	fileutil.ReadJSON(filepath.Join(dataDir, fmt.Sprintf("inventory-%s.json", pubkey)), &inv)
	if inv == nil {
		inv = []Pack{}
	}
	return inv
}


// SaveInventory handles the SaveInventory HTTP request.
func SaveInventory(dataDir, pubkey string, inv []Pack) {
	p := filepath.Join(dataDir, fmt.Sprintf("inventory-%s.json", pubkey))
	fileutil.WriteJSON(p, inv)
}

// --- Treasury dynamic scale ---

type TreasuryConfig struct {
	MonthlyOpsNg   int64 `json:"monthly_ops_ng"`
	Threshold3x    int64 `json:"threshold_3x"`
	Threshold6x    int64 `json:"threshold_6x"`
	Threshold12x   int64 `json:"threshold_12x"`
}


// DefaultTreasuryConfig handles the DefaultTreasuryConfig HTTP request.
func DefaultTreasuryConfig() TreasuryConfig {
	return TreasuryConfig{
		MonthlyOpsNg:   500_000_000_000, // ~500g серебра в месяц на операцию
		Threshold3x:    3,
		Threshold6x:    6,
		Threshold12x:   12,
	}
}

// CalculateTreasurySplit динамически распределяет 20% казны.
// Возвращает: ops, reserve, insurance, to_people.
func CalculateTreasurySplit(treasuryNg int64, config TreasuryConfig) (ops, reserve, insurance, toPeople int64) {
	pool := treasuryNg
	if pool <= 0 {
		return 0, 0, 0, 0
	}

	multiples := pool / config.MonthlyOpsNg

	switch {
	case multiples < config.Threshold3x:
		// Казна тощая — 75% себе, 25% резерв
		ops = pool * 75 / 100
		reserve = pool * 25 / 100
		insurance = 0
		toPeople = 0
	case multiples < config.Threshold6x:
		// Нормально — 50% операция, 25% резерв, 25% страховка
		ops = pool * 50 / 100
		reserve = pool * 25 / 100
		insurance = pool * 25 / 100
		toPeople = 0
	case multiples < config.Threshold12x:
		// Жир — 20% операция, 30% резерв, 50% народу
		ops = pool * 20 / 100
		reserve = pool * 30 / 100
		insurance = 0
		toPeople = pool * 50 / 100
	default:
		// Очень жир — 10% операция, 20% резерв, 30% народу, 40% сжечь (дефляция)
		ops = pool * 10 / 100
		reserve = pool * 20 / 100
		insurance = 0
		toPeople = pool * 30 / 100
		// Burn = pool * 40 / 100 — не возвращается никому, total supply уменьшается
	}

	return
}
