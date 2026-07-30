// Package economy — Advertising Tags (дефляционная реклама).
//
// Реклама за нанограммы. Механика:
//   - 20% цены тега СЖИГАЕТСЯ (чистая дефляция)
//   - 40% идёт в казну, 40% в дивидендный пул
//   - Цена тега растёт с популярностью (+10% за каждые 10 объявлений)
//   - Тег живёт 30 дней, затем его можно перекупить
package economy

import (
	"fmt"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"ParanoidX/internal/fileutil"
)

// TagBasePriceNg is the base price of a tag (5M ng = $0.012).
// TagTTLDays is the lifetime of a tag before it can be repurchased.
// TagBurnPct is the percentage of tag price burned (deflation).
// TagTreasuryPct is the percentage sent to treasury.
// TagDividendPct is the percentage sent to dividend pool.
// AdBasePriceNg is the base price of placing an ad (500K ng = $0.0012).
// MaxAdsPerTag is the maximum number of active ads per tag.
const (
	TagBasePriceNg    int64  = 5_000_000  // $0.012 — доступно, не мусор
	TagTTLDays        int    = 30
	TagBurnPct        float64 = 20.0
	TagTreasuryPct    float64 = 40.0
	TagDividendPct    float64 = 40.0
	AdBasePriceNg     int64  = 500_000   // $0.0012
	MaxAdsPerTag      int    = 100
)

// TagToken represents a purchased advertising tag with ownership, pricing, and expiry tracking.
type TagToken struct {
	Tag         string `json:"tag"`
	Owner       string `json:"owner"`
	PurchasedAt string `json:"purchased_at"`
	ExpiresAt   string `json:"expires_at"`
	PriceNg     int64  `json:"price_ng"`
	Active      bool   `json:"active"`
}

// AdItem is a single advertisement placed under a tag.
type AdItem struct {
	ID        string `json:"id"`
	Tag       string `json:"tag"`
	Owner     string `json:"owner"`
	Title     string `json:"title"`
	Desc      string `json:"desc"`
	Contact   string `json:"contact"`
	CreatedAt string `json:"created_at"`
	Active    bool   `json:"active"`
}

// TagManager manages tag ownership, ad placement, and deflation tracking.
type TagManager struct {
	mu      sync.Mutex
	Tags    map[string]*TagToken `json:"tags"`
	Ads     []AdItem             `json:"ads"`
	NextID  int                  `json:"next_id"`
	TotalBurnedNg int64          `json:"total_burned_ng"`
}

// NewTagManager creates a TagManager with empty tag and ad maps.
func NewTagManager() *TagManager {
	return &TagManager{
		Tags:   make(map[string]*TagToken),
		NextID: 1,
	}
}

// LoadTagManager loads tag state from disk, falling back to defaults if the file is missing.
func LoadTagManager(dataDir string) *TagManager {
	tm := NewTagManager()
	fileutil.ReadJSON(filepath.Join(dataDir, "tags.json"), tm)
	if tm.Tags == nil {
		tm.Tags = make(map[string]*TagToken)
	}
	if tm.Ads == nil {
		tm.Ads = []AdItem{}
	}
	return tm
}

// Save persists the tag manager state to advertising_tags.json.
func (tm *TagManager) Save(dataDir string) {
	p := filepath.Join(dataDir, "advertising_tags.json")
	fileutil.WriteJSON(p, tm)
}

func (tm *TagManager) dynamicPrice(tag string) int64 {
	adCount := 0
	for _, a := range tm.Ads {
		if a.Tag == tag && a.Active {
			adCount++
		}
	}
	premium := int64(adCount/10) * TagBasePriceNg / 10
	return TagBasePriceNg + premium
}

// BuyTag покупает тег. 20% сжигается, 40% казне, 40% дивиденды.
func (tm *TagManager) BuyTag(dataDir, tag, owner string, ledger *Ledger) (*TagToken, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if tag == "" {
		return nil, fmt.Errorf("tag cannot be empty")
	}

	if existing, ok := tm.Tags[tag]; ok {
		expAt, err := time.Parse(time.RFC3339, existing.ExpiresAt)
		if err == nil && time.Now().Before(expAt) {
			return nil, fmt.Errorf("tag %q owned until %s", tag, existing.ExpiresAt)
		}
	}

	price := tm.dynamicPrice(tag)
	if ledger.Balance(owner) < price {
		return nil, fmt.Errorf("need %d ng, have %d ng", price, ledger.Balance(owner))
	}

	ledger.Mint(owner, -price)

	// Казна 2.28%, 20% сжигается, остальное — дивиденды
	toTreasury := price * GetTreasuryCommissionBPS() / 10000
	burn := price * 20 / 100
	toDividends := price - toTreasury - burn

	ledger.TotalSupply -= burn // СЖИГАНИЕ
	tm.TotalBurnedNg += burn
	ledger.Mint("treasury", toTreasury)
	ledger.Mint("dividend_pool", toDividends)
	ledger.Save(dataDir)

	now := time.Now().UTC()
	token := &TagToken{
		Tag:         tag,
		Owner:       owner,
		PurchasedAt: now.Format(time.RFC3339),
		ExpiresAt:   now.Add(time.Duration(TagTTLDays) * 24 * time.Hour).Format(time.RFC3339),
		PriceNg:     price,
		Active:      true,
	}
	tm.Tags[tag] = token
	tm.Save(dataDir)
	return token, nil
}

// PlaceAd creates a new ad under an owned tag, charging the ad base price.
func (tm *TagManager) PlaceAd(dataDir, tag, owner, title, desc, contact string, ledger *Ledger) (*AdItem, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	token, ok := tm.Tags[tag]
	if !ok || !token.Active {
		return nil, fmt.Errorf("tag %q not active", tag)
	}
	expAt, err := time.Parse(time.RFC3339, token.ExpiresAt)
	if err != nil || time.Now().After(expAt) {
		return nil, fmt.Errorf("tag %q expired", tag)
	}
	if token.Owner != owner {
		return nil, fmt.Errorf("only tag owner can place ads")
	}

	adCount := 0
	for _, a := range tm.Ads {
		if a.Tag == tag && a.Active {
			adCount++
		}
	}
	if adCount >= MaxAdsPerTag {
		return nil, fmt.Errorf("max %d ads for tag %q", MaxAdsPerTag, tag)
	}
	if ledger.Balance(owner) < AdBasePriceNg {
		return nil, fmt.Errorf("insufficient balance")
	}

	ledger.Mint(owner, -AdBasePriceNg)
	ledger.Mint("treasury", AdBasePriceNg)
	ledger.Save(dataDir)

	ad := AdItem{
		ID:        fmt.Sprintf("AD-%d", tm.NextID),
		Tag:       tag,
		Owner:     owner,
		Title:     title,
		Desc:      desc,
		Contact:   contact,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Active:    true,
	}
	tm.NextID++
	tm.Ads = append(tm.Ads, ad)
	tm.Save(dataDir)
	return &ad, nil
}

// SearchAds returns all active ads for the given tag.
func (tm *TagManager) SearchAds(tag string) []AdItem {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	var results []AdItem
	for _, a := range tm.Ads {
		if a.Tag == tag && a.Active {
			results = append(results, a)
		}
	}
	return results
}

// PopularTags returns the top N tags sorted by ad count.
func (tm *TagManager) PopularTags(limit int) []struct {
	Tag string `json:"tag"`
	Ads int    `json:"ads"`
} {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	counts := map[string]int{}
	for _, a := range tm.Ads {
		if a.Active {
			counts[a.Tag]++
		}
	}
	type tc struct {
		Tag string
		Ads int
	}
	var sorted []tc
	for tag, c := range counts {
		sorted = append(sorted, tc{tag, c})
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Ads > sorted[j].Ads })
	if limit > 0 && limit < len(sorted) {
		sorted = sorted[:limit]
	}
	result := make([]struct {
		Tag string `json:"tag"`
		Ads int    `json:"ads"`
	}, len(sorted))
	for i, s := range sorted {
		result[i] = struct {
			Tag string `json:"tag"`
			Ads int    `json:"ads"`
		}{s.Tag, s.Ads}
	}
	return result
}

// DeflationSummary возвращает статистику сожжённых ng.
type DeflationSummary struct {
	BurnedFromTags     int64 `json:"burned_from_tags"`
	TotalSupply        int64 `json:"total_supply"`
	BurnedPct          float64 `json:"burned_pct"`
}

// DeflationSummary returns deflation statistics from burned tag purchases.
func (tm *TagManager) DeflationSummary(ledger *Ledger) DeflationSummary {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	pct := 0.0
	if ledger.TotalSupply+tm.TotalBurnedNg > 0 {
		pct = float64(tm.TotalBurnedNg) / float64(ledger.TotalSupply+tm.TotalBurnedNg) * 100
	}
	return DeflationSummary{
		BurnedFromTags: tm.TotalBurnedNg,
		TotalSupply:    ledger.TotalSupply,
		BurnedPct:      pct,
	}
}
