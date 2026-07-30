// Package economy — Auction House: 27-часовые аукционы с proxy-bidding.
// Особенности:
//   - Прокси-ставки: bid <= proxy_max → автоматический перебив на +1 ng
//   - Авто-продление: если ставка сделана в последние 5 мин → +5 мин
//   - Комиссия: 4.20% всего (2.28% казне + 1.92% дивиденды)
//   - Юбилейные серии (jubilee): mint из AuctionPool, 100% revenue в пул
//   - Фоновая горутина: проверяет и закрывает истёкшие лоты каждые 30 сек
//   - Эскроу: ставка блокируется на счету "escrow" до завершения аукциона
package economy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"ParanoidX/internal/fileutil"
)

type Auction struct {
	ListingID      string    `json:"listing_id"`
	Serial         string    `json:"serial"`
	DenominationNg int64     `json:"denomination_ng"`
	Rarity         string    `json:"rarity"`
	SellerPubkey   string    `json:"seller_pubkey"`
	StartPriceNG   int64     `json:"start_price_ng"`
	ListingFeePaid int64     `json:"listing_fee_paid,omitempty"` // 0.5% min 1M ng
	MinBidNG       int64     `json:"min_bid_ng"`
	CurrentBidNG   int64     `json:"current_bid_ng"`
	HighestBidder  string    `json:"highest_bidder"`
	ProxyMaxNG     int64     `json:"proxy_max_ng"`
	CreatedAt      time.Time `json:"created_at"`
	EndsAt         time.Time `json:"ends_at"`
	Closed         bool      `json:"closed"`
	WonBy          string    `json:"won_by,omitempty"`
	FinalPriceNG   int64     `json:"final_price_ng,omitempty"`
	IsJubilee      bool      `json:"is_jubilee,omitempty"` // commemorative series
}

type AuctionManager struct {
	mu       sync.RWMutex
	dataDir  string
	auctions []*Auction
	nextID   int64
	ticker   *time.Ticker
	done     chan struct{}
}

// NewAuctionManager создаёт новый AuctionManager, загружает сохранённые аукционы
// и запускает фоновую горутину для автоматического закрытия истёкших лотов каждые 30 сек.
func NewAuctionManager(dataDir string) *AuctionManager {
	am := &AuctionManager{
		dataDir: dataDir,
		nextID:  1,
		done:    make(chan struct{}),
	}
	am.load()
	go am.backgroundCloser()
	return am
}

// Stop останавливает фоновую горутину закрытия аукционов.
func (am *AuctionManager) Stop() {
	close(am.done)
}

func (am *AuctionManager) auctionPath() string {
	return filepath.Join(am.dataDir, "auctions.json")
}

func (am *AuctionManager) load() {
	data, err := os.ReadFile(am.auctionPath())
	if err != nil {
		am.auctions = []*Auction{}
		return
	}
	var state struct {
		NextID   int64      `json:"next_id"`
		Auctions []*Auction `json:"auctions"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		am.auctions = []*Auction{}
		return
	}
	am.nextID = state.NextID
	am.auctions = state.Auctions
	if am.auctions == nil {
		am.auctions = []*Auction{}
	}
}

func (am *AuctionManager) save() {
	state := struct {
		NextID   int64      `json:"next_id"`
		Auctions []*Auction `json:"auctions"`
	}{
		NextID:   am.nextID,
		Auctions: am.auctions,
	}
	fileutil.WriteJSON(am.auctionPath(), state)
}

// List выставляет банкноту на аукцион. Взимает listing fee (0.5% от startPrice,
// минимум AuctionMinListingFeeNg). Проверяет статус банкноты (должна быть active
// и принадлежать sellerPubkey), переводит её в статус "pending_auction" и создаёт лот
// с начальной ценой startPriceNG и длительностью duration.
func (am *AuctionManager) List(banknote BanknoteV2, sellerPubkey string, startPriceNG int64, duration time.Duration, opts ...AuctionOption) (*Auction, error) {
	am.mu.Lock()
	defer am.mu.Unlock()

	if banknote.Status != "active" || banknote.Holder != sellerPubkey {
		return nil, fmt.Errorf("banknote not active or not owned by seller")
	}
	if startPriceNG <= 0 {
		return nil, fmt.Errorf("start price must be positive")
	}

	// Рассчитать listing fee: 0.5% от startPrice, минимум 1,000,000 ng
	listingFee := startPriceNG * GetAuctionListingFeeBPS() / 10000
	if listingFee < AuctionMinListingFeeNg {
		listingFee = AuctionMinListingFeeNg
	}

	ledger := LoadLedger(am.dataDir)
	if ledger.Balance(sellerPubkey) < listingFee {
		return nil, fmt.Errorf("insufficient balance for listing fee: need %d ng, have %d ng", listingFee, ledger.Balance(sellerPubkey))
	}
	ledger.Mint(sellerPubkey, -listingFee)
	ledger.Mint("treasury", listingFee)
	ledger.Save(am.dataDir)

	now := time.Now()
	listingID := fmt.Sprintf("AUC-%s-%d", now.Format("20060102"), am.nextID)
	am.nextID++

	isJubilee := false
	for _, opt := range opts {
		if opt.IsJubilee {
			isJubilee = true
		}
	}

	auction := &Auction{
		ListingID:      listingID,
		Serial:         banknote.Serial,
		DenominationNg: banknote.DenominationNg,
		Rarity:         banknote.Rarity,
		SellerPubkey:   sellerPubkey,
		StartPriceNG:   startPriceNG,
		ListingFeePaid: listingFee,
		MinBidNG:       startPriceNG,
		CurrentBidNG:   0,
		CreatedAt:      now,
		EndsAt:         now.Add(duration),
		Closed:         false,
		IsJubilee:      isJubilee,
	}

	banknotes, _ := LoadBanknotesV2(am.dataDir)
	for i := range banknotes {
		if banknotes[i].Serial == banknote.Serial {
			banknotes[i].Status = "pending_auction"
			banknotes[i].FrozenNg = 0
			break
		}
	}
	SaveBanknotesV2(am.dataDir, banknotes)

	am.auctions = append(am.auctions, auction)
	am.save()
	return auction, nil
}

// AuctionOption опциональные параметры создания аукциона.
type AuctionOption struct {
	IsJubilee bool // юбилейная серия (из AuctionPool)
}

// Bid делает ставку на аукцион. Работает как proxy-bid: если bidNG <= proxyMax
// текущего лидера, то ставка автоматически повышается до currentBid + 1.
// Если ставка сделана в последние 5 минут — время аукциона продлевается на 5 мин.
// Средства блокируются на эскроу-счёте.
func (am *AuctionManager) Bid(listingID string, bidderPubkey string, bidNG int64) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	auction := am.findAuctionLocked(listingID)
	if auction == nil {
		return fmt.Errorf("auction not found")
	}
	if auction.Closed {
		return fmt.Errorf("auction already closed")
	}
	if time.Now().After(auction.EndsAt) {
		return fmt.Errorf("auction has ended")
	}
	if bidderPubkey == auction.SellerPubkey {
		return fmt.Errorf("seller cannot bid on own auction")
	}

	if bidNG <= auction.CurrentBidNG {
		return fmt.Errorf("bid must exceed current bid of %d", auction.CurrentBidNG)
	}

	ledger := LoadLedger(am.dataDir)
	bal := ledger.Balance(bidderPubkey)
	if bal < bidNG {
		return fmt.Errorf("insufficient balance: have %d, need %d", bal, bidNG)
	}

	// Proxy bid: если bid <= proxy_max, то ставим на 1 больше текущей
	if bidNG <= auction.ProxyMaxNG {
		bidNG = auction.CurrentBidNG + 1
	}

	auction.CurrentBidNG = bidNG
	auction.HighestBidder = bidderPubkey

	// Auto-extend last 5 minutes
	if time.Until(auction.EndsAt) < 5*time.Minute {
		auction.EndsAt = auction.EndsAt.Add(5 * time.Minute)
	}

	am.save()

	// Pre-authorize by escrowing the bid amount
	ledger.Transfer(bidderPubkey, "escrow", bidNG)
	ledger.Save(am.dataDir)
	return nil
}

// Close принудительно закрывает аукцион (если истекло время). Применяет комиссии:
//   - Seller fee: 1% (AuctionSellerFeeBPS) — удерживается с продавца
//   - Buyer premium: 2.5% (AuctionBuyerPremiumBPS) — доплачивает победитель сверх ставки
//   - Listing fee: уже удержана при List()
//   - Юбилейные серии: 100% revenue идёт в AuctionPool
// Если ставок не было — банкнота возвращается продавцу.
func (am *AuctionManager) Close(listingID string) (*Auction, error) {
	am.mu.Lock()
	defer am.mu.Unlock()

	auction := am.findAuctionLocked(listingID)
	if auction == nil {
		return nil, fmt.Errorf("auction not found")
	}
	if auction.Closed {
		return nil, fmt.Errorf("auction already closed")
	}
	if time.Now().Before(auction.EndsAt) && !auction.Closed {
		return nil, fmt.Errorf("auction still running, ends at %s", auction.EndsAt.Format(time.RFC3339))
	}
	return am.finalizeAuction(auction)
}

func (am *AuctionManager) finalizeAuction(auction *Auction) (*Auction, error) {
	auction.Closed = true
	ledger := LoadLedger(am.dataDir)

	if auction.CurrentBidNG > 0 && auction.HighestBidder != "" {
		auction.WonBy = auction.HighestBidder
		auction.FinalPriceNG = auction.CurrentBidNG

		// Комиссия: 4.20% всего (2.28% казне + 1.92% дивиденды)
		totalFee := auction.CurrentBidNG * GetMaxTotalFeeBPS() / 10000
		treasuryCut := auction.CurrentBidNG * GetTreasuryCommissionBPS() / 10000
		dividendCut := totalFee - treasuryCut

		sellerProceeds := auction.CurrentBidNG - totalFee

		// Распределение
		if auction.IsJubilee {
			ledger.Transfer("escrow", "auction_pool", auction.CurrentBidNG)
		} else {
			ledger.Transfer("escrow", auction.SellerPubkey, sellerProceeds)
			ledger.Transfer("escrow", "treasury", treasuryCut)
			ledger.Transfer("escrow", "dividend_pool", dividendCut)
		}

		banknotes, _ := LoadBanknotesV2(am.dataDir)
		for i := range banknotes {
			if banknotes[i].Serial == auction.Serial {
				banknotes[i].Holder = auction.HighestBidder
				banknotes[i].Status = "active"
				banknotes[i].FrozenNg = banknotes[i].DenominationNg
				break
			}
		}
		SaveBanknotesV2(am.dataDir, banknotes)
	} else {
		// Нет ставок — вернуть банкноту продавцу
		banknotes, _ := LoadBanknotesV2(am.dataDir)
		for i := range banknotes {
			if banknotes[i].Serial == auction.Serial {
				banknotes[i].Status = "active"
				banknotes[i].FrozenNg = banknotes[i].DenominationNg
				break
			}
		}
		SaveBanknotesV2(am.dataDir, banknotes)
	}

	ledger.Save(am.dataDir)
	am.save()
	return auction, nil
}

func (am *AuctionManager) findAuctionLocked(listingID string) *Auction {
	for _, a := range am.auctions {
		if a.ListingID == listingID {
			return a
		}
	}
	return nil
}

// GetActive возвращает все активные (не закрытые) аукционы, отсортированные по времени завершения.
func (am *AuctionManager) GetActive() []*Auction {
	am.mu.RLock()
	defer am.mu.RUnlock()

	var active []*Auction
	for _, a := range am.auctions {
		if !a.Closed {
			active = append(active, a)
		}
	}
	sort.Slice(active, func(i, j int) bool {
		return active[i].EndsAt.Before(active[j].EndsAt)
	})
	return active
}

// GetMyListings возвращает все аукционы, где указанный pubkey является продавцом или лидирующим биддером.
func (am *AuctionManager) GetMyListings(pubkey string) []*Auction {
	am.mu.RLock()
	defer am.mu.RUnlock()

	var mine []*Auction
	for _, a := range am.auctions {
		if a.SellerPubkey == pubkey || a.HighestBidder == pubkey {
			mine = append(mine, a)
		}
	}
	return mine
}

func (am *AuctionManager) backgroundCloser() {
	am.ticker = time.NewTicker(30 * time.Second)
	defer am.ticker.Stop()

	for {
		select {
		case <-am.ticker.C:
			am.closeExpired()
		case <-am.done:
			return
		}
	}
}

func (am *AuctionManager) closeExpired() {
	am.mu.Lock()
	defer am.mu.Unlock()

	now := time.Now()
	for _, auction := range am.auctions {
		if !auction.Closed && now.After(auction.EndsAt) {
			am.finalizeAuction(auction)
		}
	}
	am.save()
}
