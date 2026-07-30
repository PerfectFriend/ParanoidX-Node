// Package api provides HTTP handlers and API endpoints for the simplex-node server
package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"ParanoidX/internal/economy"
	"ParanoidX/internal/fileutil"
	"ParanoidX/internal/middleware"
)

// EscrowRecord represents an escrow transaction between buyer and seller.
type EscrowRecord struct {
	ID        string `json:"id"`
	Buyer     string `json:"buyer"`
	Seller    string `json:"seller"`
	ItemID    string `json:"item_id"`
	PriceNg   int64  `json:"price_ng"`
	Status    string `json:"status"` // active, released, cancelled
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func escrowFile(dataDir string) string {
	return filepath.Join(dataDir, "market_escrows.json")
}

func loadEscrows(dataDir string) []EscrowRecord {
	var escrows []EscrowRecord
	fileutil.ReadJSON(escrowFile(dataDir), &escrows)
	return escrows
}

func saveEscrows(dataDir string, escrows []EscrowRecord) {
	fileutil.WriteJSON(escrowFile(dataDir), escrows)
}

func loadLedger(dataDir string) *economy.Ledger {
	return economy.LoadLedger(dataDir)
}


// CreateEscrowHandler creates an escrow transaction between buyer and seller.
func CreateEscrowHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		var req struct {
			Buyer   string `json:"buyer"`
			Seller  string `json:"seller"`
			ItemID  string `json:"item_id"`
			PriceNg int64  `json:"price_ng"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
			return
		}
		if req.Buyer == "" || req.Seller == "" || req.ItemID == "" || req.PriceNg <= 0 {
			http.Error(w, "buyer, seller, item_id, price_ng required", http.StatusBadRequest)
			return
		}

		ledger := loadLedger(dataDir)

		// Check buyer balance
		if ledger.Balance(req.Buyer) < req.PriceNg {
			http.Error(w, "insufficient balance", http.StatusPaymentRequired)
			return
		}

		// Transfer from buyer to node (escrow hold)
		if err := ledger.Transfer(req.Buyer, "escrow:"+req.ItemID, req.PriceNg); err != nil {
			http.Error(w, "transfer: "+err.Error(), http.StatusInternalServerError)
			return
		}
		ledger.Save(dataDir)

		escrows := loadEscrows(dataDir)
		escrow := EscrowRecord{
			ID:        fmt.Sprintf("esc-%d-%s", time.Now().Unix(), req.ItemID),
			Buyer:     req.Buyer,
			Seller:    req.Seller,
			ItemID:    req.ItemID,
			PriceNg:   req.PriceNg,
			Status:    "active",
			CreatedAt: time.Now().Format(time.RFC3339),
			UpdatedAt: time.Now().Format(time.RFC3339),
		}
		escrows = append(escrows, escrow)
		saveEscrows(dataDir, escrows)

		slog.Info("escrow created", "id", escrow.ID, "buyer", req.Buyer, "seller", req.Seller, "price_ng", req.PriceNg)
		writeJSON(w, map[string]any{"ok": true, "escrow": escrow})
	}
}


// ReleaseEscrowHandler handles the ReleaseEscrowHandler HTTP request.
// ReleaseEscrowHandler releases funds from an escrow to the seller.
func ReleaseEscrowHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "escrow id required", http.StatusBadRequest)
			return
		}

		escrows := loadEscrows(dataDir)
		var target *EscrowRecord
		for i := range escrows {
			if escrows[i].ID == id {
				target = &escrows[i]
				break
			}
		}
		if target == nil {
			http.Error(w, "escrow not found", http.StatusNotFound)
			return
		}
		if target.Status != "active" {
			http.Error(w, "escrow already "+target.Status, http.StatusConflict)
			return
		}

		ledger := loadLedger(dataDir)

		// Transfer from escrow to seller
		if err := ledger.Transfer("escrow:"+target.ItemID, target.Seller, target.PriceNg); err != nil {
			http.Error(w, "release: "+err.Error(), http.StatusInternalServerError)
			return
		}
		ledger.Save(dataDir)

		target.Status = "released"
		target.UpdatedAt = time.Now().Format(time.RFC3339)
		saveEscrows(dataDir, escrows)

		slog.Info("escrow released", "id", id, "seller", target.Seller)
		writeJSON(w, map[string]any{"ok": true, "escrow": target})
	}
}


// CancelEscrowHandler handles the CancelEscrowHandler HTTP request.
// CancelEscrowHandler cancels an escrow and refunds the buyer.
func CancelEscrowHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "escrow id required", http.StatusBadRequest)
			return
		}

		escrows := loadEscrows(dataDir)
		var target *EscrowRecord
		for i := range escrows {
			if escrows[i].ID == id {
				target = &escrows[i]
				break
			}
		}
		if target == nil {
			http.Error(w, "escrow not found", http.StatusNotFound)
			return
		}
		if target.Status != "active" {
			http.Error(w, "escrow already "+target.Status, http.StatusConflict)
			return
		}

		ledger := loadLedger(dataDir)

		// Return funds to buyer
		if err := ledger.Transfer("escrow:"+target.ItemID, target.Buyer, target.PriceNg); err != nil {
			http.Error(w, "cancel: "+err.Error(), http.StatusInternalServerError)
			return
		}
		ledger.Save(dataDir)

		target.Status = "cancelled"
		target.UpdatedAt = time.Now().Format(time.RFC3339)
		saveEscrows(dataDir, escrows)

		slog.Info("escrow cancelled", "id", id, "buyer", target.Buyer)
		writeJSON(w, map[string]any{"ok": true, "escrow": target})
	}
}


// ListEscrowHandler handles the ListEscrowHandler HTTP request.
// ListEscrowHandler returns all escrow records, optionally filtered by status.
func ListEscrowHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		statusFilter := r.URL.Query().Get("status")
		escrows := loadEscrows(dataDir)
		var filtered []EscrowRecord
		for _, e := range escrows {
			if statusFilter == "" || e.Status == statusFilter {
				filtered = append(filtered, e)
			}
		}
		writeJSON(w, map[string]any{"count": len(filtered), "escrows": filtered})
	}
}


// MarketListHandler handles the MarketListHandler HTTP request.
// MarketListHandler returns current sell orders in the marketplace.
func MarketListHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !middleware.IsLocalOrOnionAccess(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		items := readMarketItems(dataDir)
		var available []map[string]any
		for _, it := range items {
			if fs, ok := it["for_sale"].(bool); ok && fs {
				available = append(available, it)
			}
		}
		writeJSON(w, map[string]any{"count": len(available), "items": available})
	}
}

func readMarketItems(dataDir string) []map[string]any {
	var items []map[string]any
	fileutil.ReadJSON(filepath.Join(dataDir, "market_listings.json"), &items)
	return items
}

func readRWAItems(dataDir string) []map[string]any {
	var items []map[string]any
	fileutil.ReadJSON(filepath.Join(dataDir, "rwa_registry.json"), &items)
	return items
}

func writeRWAItems(dataDir string, items []map[string]any) {
	fileutil.WriteJSON(filepath.Join(dataDir, "rwa_registry.json"), items)
}

func writeMarketItems(dataDir string, items []map[string]any) {
	fileutil.WriteJSON(filepath.Join(dataDir, "market_listings.json"), items)
}

func appendChannelPost(dataDir string, text string) {
	chFile := filepath.Join(dataDir, "channels.json")
	if b, err := os.ReadFile(chFile); err == nil {
		var chs []map[string]any
		if json.Unmarshal(b, &chs) == nil && len(chs) > 0 {
			posts, _ := chs[0]["posts"].([]interface{})
			posts = append(posts, map[string]any{"text": text, "ts": time.Now().Format(time.RFC3339)})
			chs[0]["posts"] = posts
			fileutil.WriteJSON(chFile, chs)
		}
	}
}


// MarketSellHandler places a sell order on the marketplace and records the bill.
func MarketSellHandler(dataDir string, billRecorder func(price int64, action, itemID string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !middleware.IsLocalOrOnionAccess(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		if r.Method != "POST" {
			http.Error(w, "POST required", 400)
			return
		}
		var req struct {
			ID      string `json:"id"`
			PriceNg int64  `json:"price_ng"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
			return
		}
		if req.ID == "" || req.PriceNg <= 0 {
			http.Error(w, "id and positive price_ng required", 400)
			return
		}

		rwaItems := readRWAItems(dataDir)
		found := false
		for i := range rwaItems {
			if fmt.Sprintf("%v", rwaItems[i]["id"]) == req.ID {
				rwaItems[i]["for_sale"] = true
				rwaItems[i]["price_ng"] = req.PriceNg
				found = true
				break
			}
		}
		if !found {
			mktItems := readMarketItems(dataDir)
			mktItems = append(mktItems, map[string]any{"id": req.ID, "price_ng": req.PriceNg, "for_sale": true, "listed": time.Now().Format(time.RFC3339)})
			writeMarketItems(dataDir, mktItems)
		} else {
			writeRWAItems(dataDir, rwaItems)
		}

		appendChannelPost(dataDir, "New market listing for sale: "+req.ID+" at "+fmt.Sprint(req.PriceNg)+" ng")
		if billRecorder != nil {
			billRecorder(req.PriceNg, "market_sell_listing", req.ID)
		}
		writeJSON(w, map[string]any{"ok": true, "note": "Listed in hand-to-hand marketplace."})
	}
}


// MarketBuyHandler executes a buy order from the marketplace and records the bill.
func MarketBuyHandler(dataDir string, billRecorder func(price int64, action, itemID string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !middleware.IsLocalOrOnionAccess(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		if r.Method != "POST" {
			http.Error(w, "POST required", 400)
			return
		}
		var req struct {
			ID     string `json:"id"`
			Buyer  string `json:"buyer"`
			Seller string `json:"seller"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
			return
		}
		if req.ID == "" {
			http.Error(w, "id required", 400)
			return
		}

		rwaItems := readRWAItems(dataDir)
		bought := false
		var item map[string]any
		for i := range rwaItems {
			if fmt.Sprintf("%v", rwaItems[i]["id"]) == req.ID {
				price := int64(0)
				if p, ok := rwaItems[i]["price_ng"].(float64); ok {
					price = int64(p)
				} else if p, ok := rwaItems[i]["price_ng"].(int64); ok {
					price = p
				}
				if price <= 0 {
					http.Error(w, "Not for sale or no price", 400)
					return
				}
				if billRecorder != nil {
					billRecorder(price, "market_buy", req.ID)
				}
				rwaItems[i]["holder"] = "buyer-via-hand-to-hand"
				if req.Buyer != "" {
					rwaItems[i]["holder"] = req.Buyer
				}
				delete(rwaItems[i], "for_sale")
				delete(rwaItems[i], "price_ng")
				item = rwaItems[i]
				bought = true
				break
			}
		}
		if !bought {
			http.Error(w, "Item not found or not buyable", 404)
			return
		}
		writeRWAItems(dataDir, rwaItems)
		writeJSON(w, map[string]any{"ok": true, "item": item})
	}
}

// NewEscrowBuyHandler combines buy + escrow: creates escrow from buyer, then on release transfers item ownership
func NewEscrowBuyHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		var req struct {
			Buyer  string `json:"buyer"`
			Seller string `json:"seller"`
			ItemID string `json:"item_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
			return
		}
		if req.Buyer == "" || req.Seller == "" || req.ItemID == "" {
			http.Error(w, "buyer, seller, item_id required", http.StatusBadRequest)
			return
		}

		// Find item and price
		rwaItems := readRWAItems(dataDir)
		var priceNg int64
		found := false
		for i := range rwaItems {
			if fmt.Sprintf("%v", rwaItems[i]["id"]) == req.ItemID {
				if p, ok := rwaItems[i]["price_ng"].(float64); ok {
					priceNg = int64(p)
				} else if p, ok := rwaItems[i]["price_ng"].(int64); ok {
					priceNg = p
				}
				found = true
				break
			}
		}
		if !found || priceNg <= 0 {
			http.Error(w, "item not found or not for sale", http.StatusNotFound)
			return
		}

		ledger := loadLedger(dataDir)

		// Check balance
		if ledger.Balance(req.Buyer) < priceNg {
			http.Error(w, "insufficient balance", http.StatusPaymentRequired)
			return
		}

		// Hold funds in escrow
		if err := ledger.Transfer(req.Buyer, "escrow:"+req.ItemID, priceNg); err != nil {
			http.Error(w, "escrow hold: "+err.Error(), http.StatusInternalServerError)
			return
		}
		ledger.Save(dataDir)

		// Create escrow record
		escrows := loadEscrows(dataDir)
		escrow := EscrowRecord{
			ID:        fmt.Sprintf("esc-%d-%s", time.Now().Unix(), req.ItemID),
			Buyer:     req.Buyer,
			Seller:    req.Seller,
			ItemID:    req.ItemID,
			PriceNg:   priceNg,
			Status:    "active",
			CreatedAt: time.Now().Format(time.RFC3339),
			UpdatedAt: time.Now().Format(time.RFC3339),
		}
		escrows = append(escrows, escrow)
		saveEscrows(dataDir, escrows)

		slog.Info("escrow-buy created", "id", escrow.ID, "buyer", req.Buyer, "seller", req.Seller, "price", priceNg)
		writeJSON(w, map[string]any{"ok": true, "escrow": escrow, "note": "Funds held in escrow. Use /api/escrow/release?id=... to complete."})
	}
}

// AutoResolveHandler releases all completed escrows (e.g., if buyer/seller both confirm)
func AutoResolveHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		escrows := loadEscrows(dataDir)
		released := 0
		for i := range escrows {
			if escrows[i].Status == "active" {
				ledger := loadLedger(dataDir)
				if err := ledger.Transfer("escrow:"+escrows[i].ItemID, escrows[i].Seller, escrows[i].PriceNg); err == nil {
					ledger.Save(dataDir)
					escrows[i].Status = "released"
					escrows[i].UpdatedAt = time.Now().Format(time.RFC3339)
					released++
				}
			}
		}
		if released > 0 {
			saveEscrows(dataDir, escrows)
		}
		writeJSON(w, map[string]any{"released": released, "total_active": len(escrows)})
	}
}
