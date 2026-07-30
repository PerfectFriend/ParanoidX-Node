// Package economy implements the island economy system
package economy

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"ParanoidX/internal/fileutil"
)

var (
	silverCachePrice float64
	silverCacheTime  time.Time
	silverCacheMu    sync.Mutex
)

// DefaultSilverSpotUSDperOZ is the fallback silver price used when all oracles fail.
const (
	DefaultSilverSpotUSDperOZ = 75.0
)

// PriceRecord is a single price observation.
type PriceRecord struct {
	Price float64 `json:"price"`
	Time  string  `json:"time"`
}

// SilverSpotOracle tracks the silver spot price and provides conversion.
type SilverSpotOracle struct {
	mu            sync.RWMutex
	CurrentPrice  float64       `json:"current_price"`
	LastUpdated   string        `json:"last_updated"`
	History       []PriceRecord `json:"history,omitempty"`
	done          chan struct{}
}

// NewSilverSpotOracle creates an oracle with the default price.
func NewSilverSpotOracle() *SilverSpotOracle {
	return &SilverSpotOracle{
		done:         make(chan struct{}),
		CurrentPrice: DefaultSilverSpotUSDperOZ,
		LastUpdated:  time.Now().UTC().Format(time.RFC3339),
	}
}

// LoadOracle loads the oracle state from disk.
func LoadOracle(dataDir string) *SilverSpotOracle {
	o := NewSilverSpotOracle()
	fileutil.ReadJSON(filepath.Join(dataDir, "silver_spot.json"), o)
	if o.CurrentPrice <= 0 {
		o.CurrentPrice = DefaultSilverSpotUSDperOZ
	}
	return o
}

// Save persists oracle state to JSON.
func (o *SilverSpotOracle) Save(dataDir string) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	fileutil.WriteJSON(filepath.Join(dataDir, "silver_spot.json"), o)
}

// UpdatePrice sets a new price and appends to history.
func (o *SilverSpotOracle) UpdatePrice(price float64) error {
	if price <= 0 || math.IsNaN(price) || math.IsInf(price, 0) {
		return fmt.Errorf("invalid price: %f", price)
	}
	o.mu.Lock()
	defer o.mu.Unlock()

	now := time.Now().UTC().Format(time.RFC3339)
	o.History = append(o.History, PriceRecord{Price: o.CurrentPrice, Time: o.LastUpdated})
	o.CurrentPrice = price
	o.LastUpdated = now

	// Keep only last 100 entries
	if len(o.History) > 100 {
		o.History = o.History[len(o.History)-100:]
	}
	return nil
}

// GetHistory returns the price history.
func (o *SilverSpotOracle) GetHistory() []PriceRecord {
	o.mu.RLock()
	defer o.mu.RUnlock()
	out := make([]PriceRecord, len(o.History))
	copy(out, o.History)
	return out
}

// GetPrice returns the current silver spot price.
func (o *SilverSpotOracle) GetPrice() float64 {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.CurrentPrice
}

// GetLastUpdated returns when the price was last updated.
func (o *SilverSpotOracle) GetLastUpdated() string {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.LastUpdated
}

// USDTtoNG converts USDT to ng at the current oracle price.
func (o *SilverSpotOracle) USDTtoNG(usdt float64) int64 {
	price := o.GetPrice()
	if price <= 0 {
		price = DefaultSilverSpotUSDperOZ
	}
	return int64(usdt * float64(NGPerTLR) / price)
}

// --- Deflation Manager --- (unchanged above)

// GoldAPIResponse represents the api.gold-api.com response.
type GoldAPIResponse struct {
	Price float64 `json:"price"`
}

// StartLivePolling begins periodic silver spot price fetching from api.metals.live.
// Interval defaults to 5 minutes if not specified.
func (o *SilverSpotOracle) StartLivePolling(dataDir string, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Minute
	}

	slog.Info("silver spot oracle live polling started", "interval", interval.String())
	o.pollOnce(dataDir)

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				o.pollOnce(dataDir)
			case <-o.done:
				slog.Info("silver spot oracle polling stopped")
				return
			}
		}
	}()
}

// Stop halts the live polling goroutine.
func (o *SilverSpotOracle) Stop() {
	close(o.done)
}

func (o *SilverSpotOracle) pollOnce(dataDir string) {
	price, err := fetchSilverSpot()
	if err != nil {
		slog.Warn("silver spot poll failed", "error", err)
		return
	}

	current := o.GetPrice()
	change := (price - current) / current * 100
	if err := o.UpdatePrice(price); err != nil {
		slog.Error("silver spot update failed", "error", err)
		return
	}
	o.Save(dataDir)

	txtPath := filepath.Join(dataDir, "silver_spot_usd.txt")
	if err := os.WriteFile(txtPath, []byte(fmt.Sprintf("%.6f\n", price)), 0644); err != nil {
		slog.Warn("failed to write silver_spot_usd.txt", "error", err)
	}

	if abs(change) >= 5.0 {
		slog.Warn("silver spot moved >5%", "from", current, "to", price, "change_pct", change)
	}

	slog.Debug("silver spot updated", "price", price, "change_pct", change)
}

type swissQuotePrice struct {
	SpreadProfilePrices []struct {
		Bid float64 `json:"bid"`
	} `json:"spreadProfilePrices"`
}

func fetchSilverSpot() (float64, error) {
	silverCacheMu.Lock()
	if !silverCacheTime.IsZero() && time.Since(silverCacheTime) < 60*time.Second && silverCachePrice > 0 {
		p := silverCachePrice
		silverCacheMu.Unlock()
		return p, nil
	}
	silverCacheMu.Unlock()

	price, err := fetchGoldAPI()
	if err == nil {
		silverCacheMu.Lock()
		silverCachePrice = price
		silverCacheTime = time.Now()
		silverCacheMu.Unlock()
		return price, nil
	}
	slog.Warn("gold-api failed, trying Swissquote fallback", "error", err)
	price, err = fetchSwissquote()
	if err == nil {
		silverCacheMu.Lock()
		silverCachePrice = price
		silverCacheTime = time.Now()
		silverCacheMu.Unlock()
		return price, nil
	}
	slog.Warn("Swissquote failed, trying Metals.dev fallback", "error", err)
	price, err = fetchMetalsDev()
	if err == nil {
		silverCacheMu.Lock()
		silverCachePrice = price
		silverCacheTime = time.Now()
		silverCacheMu.Unlock()
		return price, nil
	}
	slog.Warn("all silver sources failed", "primary_error", "gold-api.com failed", "fallback_errors", err)
	return 0, fmt.Errorf("silver oracle: gold-api: %v, Swissquote: %v, Metals.dev: %v", err, err, err)
}

func fetchGoldAPI() (float64, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get("https://api.gold-api.com/price/XAG")
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var result GoldAPIResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, err
	}

	if result.Price <= 0 {
		return 0, fmt.Errorf("unexpected silver price: %f", result.Price)
	}

	return result.Price, nil
}

func fetchSwissquote() (float64, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get("https://forex-data-feed.swissquote.com/public-quotes/bboquotes/instrument/XAG/USD")
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var quotes []swissQuotePrice
	if err := json.Unmarshal(body, &quotes); err != nil {
		return 0, err
	}

	if len(quotes) == 0 || len(quotes[0].SpreadProfilePrices) == 0 {
		return 0, fmt.Errorf("no Swissquote data")
	}

	return quotes[0].SpreadProfilePrices[0].Bid, nil
}

type metalsDevResponse struct {
	Status string `json:"status"`
	Data   struct {
		Rates map[string]float64 `json:"rates"`
	} `json:"data"`
}

func fetchMetalsDev() (float64, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get("https://api.metals.dev/v1/latest?api_key=demo&currency=USD&unit=toz")
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var result metalsDevResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, err
	}

	if result.Status != "ok" {
		return 0, fmt.Errorf("metals.dev status: %s", result.Status)
	}

	price, ok := result.Data.Rates["XAG"]
	if !ok || price <= 0 {
		return 0, fmt.Errorf("metals.dev no XAG rate: %v", result.Data.Rates)
	}

	return price, nil
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

// DeflationManager handles treasury surplus deflation burns at Very Fat tier.
type DeflationManager struct {
	mu               sync.Mutex
	TotalBurnedNg    int64  `json:"total_burned_ng"`
	LastBurnRound    string `json:"last_burn_round"`
	ThresholdBasis   int64  `json:"threshold_basis"` // how many months ops before deflation kicks in
}

// NewDeflationManager creates a deflation manager with a 12-month threshold basis.
func NewDeflationManager() *DeflationManager {
	return &DeflationManager{
		ThresholdBasis: 12, // 12x monthly ops = very fat
	}
}

// LoadDeflation loads state from disk.
func LoadDeflation(dataDir string) *DeflationManager {
	d := NewDeflationManager()
	fileutil.ReadJSON(filepath.Join(dataDir, "deflation_state.json"), d)
	return d
}

// Save persists deflation state to JSON.
func (d *DeflationManager) Save(dataDir string) {
	p := filepath.Join(dataDir, "deflation_state.json")
	fileutil.WriteJSON(p, d)
}

// Burn executes a deflation burn. Returns the amount burned.
func (d *DeflationManager) Burn(dataDir string, treasuryNg int64, cfg TreasuryConfig) (int64, error) {
	d.mu.Lock()

	multiples := treasuryNg / cfg.MonthlyOpsNg
	if multiples < d.ThresholdBasis {
		d.mu.Unlock()
		return 0, nil
	}

	burnAmount := treasuryNg * 40 / 100
	if burnAmount <= 0 {
		d.mu.Unlock()
		return 0, nil
	}

	d.TotalBurnedNg += burnAmount
	d.LastBurnRound = time.Now().UTC().Format(time.RFC3339)
	d.mu.Unlock()

	d.Save(dataDir)

	// Reduce total supply in ledger
	ledger := LoadLedger(dataDir)
	ledger.TotalSupply -= burnAmount
	ledger.Save(dataDir)

	// Log the burn
	logFile := filepath.Join(dataDir, "deflation_burns.log")
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		defer f.Close()
		entry := map[string]any{"burned_ng": burnAmount, "time": d.LastBurnRound, "tier": "very_fat"}
		enc := json.NewEncoder(f)
		enc.Encode(entry)
	}

	return burnAmount, nil
}
