// Package economy implements the two-tier tokenomics for Saint Mary Liberty Island:
//   - Liquid Taler (ng): free-circulating silver-backed digital currency (1 TLR = 31,103,480,000 ng)
//   - NFT Banknotes (BanknoteV2): frozen-value collectibles with dividend rights and rarity tiers
//
// Architecture overview:
//   - Ledger — Ed25519-based accounts with Liquid Taler balances (credit-based, no UTXO)
//   - Registry — Banknote registry v2 with rarity system (common→genesis), pre-mint, inventory
//   - PackManager — Booster pack creation/opening with guaranteed Rare+ per pack
//   - BuybackManager — Treasury buyback with rarity-based pricing (common at −2.28%, rare+ at par)
//   - AuctionManager — 27h proxy-bidding auction with 5-min auto-extension, 5% treasury fee
//   - P2P — Direct peer-to-peer banknote offers (auditor-only access)
//   - Auditor — Top-10 holder governance system (first investor permanent + top-9 by balance)
//   - Treasury — Dynamic split (thin/normal/fat/very fat) with debt repayment to first investor
//
// Silver Standard: 1 TLR = 1 troy oz = 31,103,480,000 ng (nanograms of silver)
// Current spot: $75/oz (to be updated via oracle)
package economy

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"ParanoidX/internal/crypto/bip39"
	"ParanoidX/internal/fileutil"
)

// Ledger holds all Liquid Taler accounts.
type Ledger struct {
	Version     int                `json:"version"`
	Accounts    map[string]Account `json:"accounts"`
	TotalSupply int64              `json:"total_supply_ng"`
	mu          sync.RWMutex
}

type Account struct {
	BalanceNg int64  `json:"balance_ng"`
	CreatedAt string `json:"created_at"`
	LastTx    string `json:"last_tx"`
}

// NGPerTLR — 1 TLR = 1 troy oz = 31.10348 g = 31,103,480,000 ng
const NGPerTLR int64 = 31_103_480_000

// SilverSpotUSDperOZ — текущий спот ~$75/oz (будет обновляться позже)
const SilverSpotUSDperOZ = 75.0

// LoadLedger reads the Liquid Taler ledger from disk, returning sensible defaults on error.
func LoadLedger(dataDir string) *Ledger {
	l := &Ledger{Version: 2, Accounts: make(map[string]Account)}
	fileutil.ReadJSON(filepath.Join(dataDir, "liquid_ledger.json"), l)
	if l.Accounts == nil {
		l.Accounts = make(map[string]Account)
	}
	return l
}

// Save сохраняет текущее состояние ledger в JSON-файл liquid_ledger.json.
func (l *Ledger) Save(dataDir string) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	p := filepath.Join(dataDir, "liquid_ledger.json")
	fileutil.WriteJSON(p, l)
}

// EnsureAccount создаёт счёт если нет.
func (l *Ledger) EnsureAccount(pubkey string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, ok := l.Accounts[pubkey]; !ok {
		l.Accounts[pubkey] = Account{
			BalanceNg: 0,
			CreatedAt: time.Now().Format(time.RFC3339),
		}
	}
}

// Balance возвращает баланс.
func (l *Ledger) Balance(pubkey string) int64 {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.Accounts[pubkey].BalanceNg
}

// Transfer переводит Liquid Taler между счетами.
func (l *Ledger) Transfer(from, to string, amount int64) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	a := l.Accounts[from]
	if a.BalanceNg < amount {
		return fmt.Errorf("insufficient funds: have %d, need %d", a.BalanceNg, amount)
	}
	a.BalanceNg -= amount
	a.LastTx = time.Now().Format(time.RFC3339)
	l.Accounts[from] = a

	b := l.Accounts[to]
	b.BalanceNg += amount
	b.LastTx = time.Now().Format(time.RFC3339)
	l.Accounts[to] = b

	return nil
}

// Mint добавляет total supply и зачисляет на счёт.
func (l *Ledger) Mint(to string, amount int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.TotalSupply += amount
	a := l.Accounts[to]
	a.BalanceNg += amount
	a.LastTx = time.Now().Format(time.RFC3339)
	l.Accounts[to] = a
}

// History возвращает историю транзакций для указанного pubkey.
// В текущей версии — заглушка, возвращает nil. Будет заменена на отдельный tx-лог.
func (l *Ledger) History(pubkey string) []map[string]any {
	// В этой версии — возвращаем заглушку.
	// Потом будет отдельный tx лог.
	return nil
}

// USDTtoNG конвертирует USDT в ng по текущему silver spot.
func USDTtoNG(usdt float64) int64 {
	// 1 oz = NGPerTLR ng, стоит SilverSpotUSDperOZ USD
	// ng = usdt * NGPerTLR / SilverSpotUSDperOZ
	return int64(usdt * float64(NGPerTLR) / SilverSpotUSDperOZ)
}

// GenerateKeypair creates Ed25519 keys + BIP39 mnemonic phrase (24 words).
func GenerateKeypair() (pubkeyHex, privkeyHex, mnemonic string, err error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", "", err
	}
	pubkeyHex = hex.EncodeToString(pub)
	privkeyHex = hex.EncodeToString(priv)
	mnemonic, err = bip39.GenerateMnemonic()
	if err != nil {
		return "", "", "", fmt.Errorf("generate mnemonic: %w", err)
	}
	return
}

// VerifySignedTx верифицирует Ed25519 подпись сообщения.
func VerifySignedTx(pubkeyHex string, message []byte, signature []byte) bool {
	pub, err := hex.DecodeString(pubkeyHex)
	if err != nil {
		return false
	}
	return ed25519.Verify(pub, message, signature)
}

// SignTx подписывает сообщение приватным ключом.
func SignTx(privkeyHex string, message []byte) ([]byte, error) {
	priv, err := hex.DecodeString(privkeyHex)
	if err != nil {
		return nil, err
	}
	return ed25519.Sign(priv, message), nil
}

// USDTtoTLR конвертирует USDT в TLR (целые).
func USDTtoTLR(usdt float64) int64 {
	return int64(usdt / SilverSpotUSDperOZ)
}

// TLStoNG конвертирует TLR в ng.
func TLStoNG(tlr int64) int64 {
	return tlr * NGPerTLR
}

// NGtoTLR конвертирует ng в TLR (целые).
func NGtoTLR(ng int64) int64 {
	return ng / NGPerTLR
}


