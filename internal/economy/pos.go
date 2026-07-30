// Package economy implements the island economy system
package economy

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"ParanoidX/internal/fileutil"
)

// POSInvoiceExpiryMinutes is the default time-to-live for POS invoices.
// POSCommissionBPS is the POS processing fee in basis points (1.00%).
const (
	POSInvoiceExpiryMinutes = 30
	POSCommissionBPS        = 100 // 1.00% POS processing fee
)

// POSInvoice is a point-of-sale payment invoice.
type POSInvoice struct {
	ID            string `json:"id"`
	Merchant      string `json:"merchant"`
	Payer         string `json:"payer,omitempty"`
	AmountNg      int64  `json:"amount_ng"`
	CommissionNg  int64  `json:"commission_ng"`
	NetAmountNg   int64  `json:"net_amount_ng"`
	Description   string `json:"description"`
	Status        string `json:"status"`
	CreatedAt     string `json:"created_at"`
	ExpiresAt     string `json:"expires_at"`
	PaidAt        string `json:"paid_at,omitempty"`
	PaymentRef    string `json:"payment_ref,omitempty"`
	PaymentURL    string `json:"payment_url,omitempty"`
}

// POSManager manages POS invoices and vouchers for merchant payments.
type POSManager struct {
	mu       sync.Mutex
	Invoices map[string]*POSInvoice `json:"invoices"`
	Vouchers map[string]*Voucher    `json:"vouchers,omitempty"`
}

// NewPOSManager creates a POS manager with empty invoice and voucher maps.
func NewPOSManager() *POSManager {
	return &POSManager{
		Invoices: make(map[string]*POSInvoice),
	}
}

// LoadPOSManager loads POS invoices and vouchers from disk.
func LoadPOSManager(dataDir string) *POSManager {
	pm := NewPOSManager()
	fileutil.ReadJSON(filepath.Join(dataDir, "pos_invoices.json"), pm)
	if pm.Invoices == nil {
		pm.Invoices = make(map[string]*POSInvoice)
	}
	return pm
}

// Save persists POS state to JSON.
func (pm *POSManager) Save(dataDir string) {
	fileutil.WriteJSON(filepath.Join(dataDir, "pos_invoices.json"), pm)
}

// RandomID generates a cryptographically random hex string (16 bytes → 32 hex chars).
func RandomID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// CreateInvoice creates a new pending POS invoice with a commission deduction.
func (pm *POSManager) CreateInvoice(merchant string, amountNg int64, description string) (*POSInvoice, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if merchant == "" {
		return nil, fmt.Errorf("merchant required")
	}
	if amountNg <= 0 {
		return nil, fmt.Errorf("amount must be > 0")
	}

	now := time.Now().UTC()
	commission := amountNg * GetPOSFeeBPS() / 10000
	netAmount := amountNg - commission

	id := RandomID()
	inv := &POSInvoice{
		ID:           id,
		Merchant:     merchant,
		AmountNg:     amountNg,
		CommissionNg: commission,
		NetAmountNg:  netAmount,
		Description:  description,
		Status:       "pending",
		CreatedAt:    now.Format(time.RFC3339),
		ExpiresAt:    now.Add(POSInvoiceExpiryMinutes * time.Minute).Format(time.RFC3339),
		PaymentURL:   fmt.Sprintf("simplex-node://pay/%s?amount=%d&merchant=%s", id, amountNg, merchant),
	}

	pm.Invoices[inv.ID] = inv
	return inv, nil
}

// PayInvoice processes payment for a POS invoice via ledger transfer.
func (pm *POSManager) PayInvoice(invoiceID, payer string, ledger *Ledger) (*POSInvoice, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	inv, ok := pm.Invoices[invoiceID]
	if !ok {
		return nil, fmt.Errorf("invoice not found")
	}
	if inv.Status != "pending" {
		return nil, fmt.Errorf("invoice status is %s, not pending", inv.Status)
	}

	expiresAt, err := time.Parse(time.RFC3339, inv.ExpiresAt)
	if err != nil || time.Now().UTC().After(expiresAt) {
		inv.Status = "expired"
		return nil, fmt.Errorf("invoice expired at %s", inv.ExpiresAt)
	}

	if payer == "" {
		return nil, fmt.Errorf("payer required")
	}

	if err := ledger.Transfer(payer, inv.Merchant, inv.AmountNg); err != nil {
		return nil, fmt.Errorf("transfer failed: %w", err)
	}

	inv.Status = "paid"
	inv.Payer = payer
	inv.PaidAt = time.Now().UTC().Format(time.RFC3339)
	inv.PaymentRef = "pos-" + RandomID()[:8]

	return inv, nil
}

// GetInvoice retrieves a POS invoice by ID, auto-expiring it if past its deadline.
func (pm *POSManager) GetInvoice(invoiceID string) (*POSInvoice, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	inv, ok := pm.Invoices[invoiceID]
	if !ok {
		return nil, fmt.Errorf("invoice not found")
	}

	// Auto-expire
	if inv.Status == "pending" {
		expiresAt, err := time.Parse(time.RFC3339, inv.ExpiresAt)
		if err == nil && time.Now().UTC().After(expiresAt) {
			inv.Status = "expired"
		}
	}

	return inv, nil
}

// ListMerchantInvoices returns all invoices for a given merchant, auto-expiring stale ones.
func (pm *POSManager) ListMerchantInvoices(merchant string) []*POSInvoice {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	var result []*POSInvoice
	for _, inv := range pm.Invoices {
		if inv.Merchant == merchant {
			inv := inv
			// Auto-expire
			if inv.Status == "pending" {
				expiresAt, err := time.Parse(time.RFC3339, inv.ExpiresAt)
				if err == nil && time.Now().UTC().After(expiresAt) {
					inv.Status = "expired"
				}
			}
			result = append(result, inv)
		}
	}
	return result
}

// MerchantRevenue returns the total net amount received by a merchant from paid invoices.
func (pm *POSManager) MerchantRevenue(merchant string) int64 {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	var total int64
	for _, inv := range pm.Invoices {
		if inv.Merchant == merchant && inv.Status == "paid" {
			total += inv.NetAmountNg
		}
	}
	return total
}

// CleanExpired marks all overdue pending invoices as expired, returning the count.
func (pm *POSManager) CleanExpired() int {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	now := time.Now().UTC()
	count := 0
	for _, inv := range pm.Invoices {
		if inv.Status != "pending" {
			continue
		}
		expiresAt, err := time.Parse(time.RFC3339, inv.ExpiresAt)
		if err == nil && now.After(expiresAt) {
			inv.Status = "expired"
			count++
		}
	}
	return count
}

// POSNotificationData is a summary of POS activity for notification purposes.
type POSNotificationData struct {
	NewPaid      int   `json:"new_paid"`
	NewExpired   int   `json:"new_expired"`
	TotalVolume  int64 `json:"total_volume_ng"`
	TotalFees    int64 `json:"total_fees_ng"`
	InvoiceCount int   `json:"invoice_count"`
}

// Voucher is a pre-paid voucher redeemable at a POS merchant.
type Voucher struct {
	Code       string `json:"code"`
	Merchant   string `json:"merchant"`
	AmountNg   int64  `json:"amount_ng"`
	Redeemed   bool   `json:"redeemed"`
	RedeemedBy string `json:"redeemed_by,omitempty"`
	RedeemedAt string `json:"redeemed_at,omitempty"`
	CreatedAt  string `json:"created_at"`
	ExpiresAt  string `json:"expires_at,omitempty"`
}

// CreateVoucher creates a new pre-paid voucher for a merchant (expires in 30 days).
func (pm *POSManager) CreateVoucher(merchant string, amountNg int64) (*Voucher, error) {
	if merchant == "" {
		return nil, fmt.Errorf("merchant required")
	}
	if amountNg <= 0 {
		return nil, fmt.Errorf("amount must be > 0")
	}
	pm.mu.Lock()
	defer pm.mu.Unlock()
	v := &Voucher{
		Code:      RandomID()[:12],
		Merchant:  merchant,
		AmountNg:  amountNg,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		ExpiresAt: time.Now().UTC().Add(720 * time.Hour).Format(time.RFC3339),
	}
	if pm.Vouchers == nil {
		pm.Vouchers = make(map[string]*Voucher)
	}
	pm.Vouchers[v.Code] = v
	return v, nil
}

// ListVouchers returns all vouchers.
func (pm *POSManager) ListVouchers() []*Voucher {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	var result []*Voucher
	for _, v := range pm.Vouchers {
		result = append(result, v)
	}
	return result
}

// RedeemVoucher redeems a voucher code, minting the amount to the redeemer's ledger.
func (pm *POSManager) RedeemVoucher(code, redeemer string, ledger *Ledger) (*Voucher, error) {
	if code == "" || redeemer == "" {
		return nil, fmt.Errorf("code and redeemer required")
	}
	pm.mu.Lock()
	defer pm.mu.Unlock()

	v, ok := pm.Vouchers[code]
	if !ok {
		return nil, fmt.Errorf("voucher not found")
	}
	if v.Redeemed {
		return nil, fmt.Errorf("voucher already redeemed")
	}
	expiresAt, err := time.Parse(time.RFC3339, v.ExpiresAt)
	if err == nil && time.Now().UTC().After(expiresAt) {
		return nil, fmt.Errorf("voucher expired")
	}

	ledger.Mint(redeemer, 0)
	inv := &POSInvoice{
		ID:         RandomID(),
		Merchant:   v.Merchant,
		Payer:      redeemer,
		AmountNg:   v.AmountNg,
		Status:     "paid",
		PaidAt:     time.Now().UTC().Format(time.RFC3339),
		PaymentRef: "voucher-" + code,
	}
	pm.Invoices[inv.ID] = inv
	ledger.Mint(redeemer, v.AmountNg)

	v.Redeemed = true
	v.RedeemedBy = redeemer
	v.RedeemedAt = inv.PaidAt
	return v, nil
}

// NotificationSummary returns a summary of POS activity counts and volumes.
func (pm *POSManager) NotificationSummary() POSNotificationData {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	var d POSNotificationData
	d.InvoiceCount = len(pm.Invoices)
	for _, inv := range pm.Invoices {
		d.TotalVolume += inv.AmountNg
		d.TotalFees += inv.CommissionNg
		if inv.Status == "paid" {
			d.NewPaid++
		}
	}
	return d
}
