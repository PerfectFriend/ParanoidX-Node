// Package economy implements the island economy system
package economy

import (
		"fmt"
		"path/filepath"
	"sync"
	"time"

	"ParanoidX/internal/fileutil"
)

// LicenseStatus represents the state of a franchise license.
type LicenseStatus string

const (
	LicenseActive   LicenseStatus = "active"
	LicenseSuspended LicenseStatus = "suspended"
	LicenseRevoked  LicenseStatus = "revoked"
)

// FranchiseLicense defines a franchise node's license.
type FranchiseLicense struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	OnionAddr   string        `json:"onion_addr"`
	Status      LicenseStatus `json:"status"`
	Tier        string        `json:"tier"` // standard, premium, royal
	FeeNg       int64         `json:"fee_ng"`
	CreatedAt   string        `json:"created_at"`
	UpdatedAt   string        `json:"updated_at"`
	MaxNodes    int           `json:"max_nodes"`
	RoyaltyBPS  int           `json:"royalty_bps"` // basis points (e.g. 50 = 0.5%)
}

// LicenseManager handles CRUD for franchise licenses.
type LicenseManager struct {
	mu       sync.Mutex
	Licenses map[string]*FranchiseLicense `json:"licenses"`
}

// NewLicenseManager creates a license manager with an empty license map.
func NewLicenseManager() *LicenseManager {
	return &LicenseManager{
		Licenses: make(map[string]*FranchiseLicense),
	}
}

// LoadLicenses loads from disk.
func LoadLicenses(dataDir string) *LicenseManager {
	lm := NewLicenseManager()

	fileutil.ReadJSON(filepath.Join(dataDir, "franchise_licenses.json"), lm)
	if lm.Licenses == nil {
		lm.Licenses = make(map[string]*FranchiseLicense)
	}
	return lm
}

// Save persists franchise licenses to JSON.
func (lm *LicenseManager) Save(dataDir string) {
	p := filepath.Join(dataDir, "franchise_licenses.json")
	fileutil.WriteJSON(p, lm)
}

// Create adds a new license.
func (lm *LicenseManager) Create(id, name, tier string, royaltyBPS int) (*FranchiseLicense, error) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	if _, exists := lm.Licenses[id]; exists {
		return nil, fmt.Errorf("license %q already exists", id)
	}
	if royaltyBPS < 0 || royaltyBPS > 10000 {
		return nil, fmt.Errorf("royalty must be 0-10000 bps")
	}

	tierFees := map[string]int64{
		"standard": 1_000_000_000,  // $2.41/mo — 1 node, anyone can afford
		"premium":  5_000_000_000,  // $12.05/mo — 5 nodes, small business
		"royal":    25_000_000_000, // $60.25/mo — unlimited, enterprise
	}
	fee, ok := tierFees[tier]
	if !ok {
		return nil, fmt.Errorf("invalid tier %q", tier)
	}
	maxNodes := map[string]int{
		"standard": 1,
		"premium":  5,
		"royal":    100,
	}

	now := time.Now().UTC().Format(time.RFC3339)
	license := &FranchiseLicense{
		ID:         id,
		Name:       name,
		Status:     LicenseActive,
		Tier:       tier,
		FeeNg:      fee,
		CreatedAt:  now,
		UpdatedAt:  now,
		MaxNodes:   maxNodes[tier],
		RoyaltyBPS: royaltyBPS,
	}
	lm.Licenses[id] = license
	return license, nil
}

// Get retrieves a license by ID.
func (lm *LicenseManager) Get(id string) (*FranchiseLicense, error) {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	lic, ok := lm.Licenses[id]
	if !ok {
		return nil, fmt.Errorf("license %q not found", id)
	}
	return lic, nil
}

// Update modifies an existing license.
func (lm *LicenseManager) Update(id, name, onionAddr string, status LicenseStatus) (*FranchiseLicense, error) {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	lic, ok := lm.Licenses[id]
	if !ok {
		return nil, fmt.Errorf("license %q not found", id)
	}
	if name != "" {
		lic.Name = name
	}
	if onionAddr != "" {
		lic.OnionAddr = onionAddr
	}
	if status != "" {
		lic.Status = status
	}
	lic.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return lic, nil
}

// List returns all licenses.
func (lm *LicenseManager) List() []*FranchiseLicense {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	list := make([]*FranchiseLicense, 0, len(lm.Licenses))
	for _, lic := range lm.Licenses {
		list = append(list, lic)
	}
	return list
}

// Delete removes a license by ID.
func (lm *LicenseManager) Delete(id string) error {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	if _, ok := lm.Licenses[id]; !ok {
		return fmt.Errorf("license %q not found", id)
	}
	delete(lm.Licenses, id)
	return nil
}

// --- Earmarked Accounts ---

// EarmarkedAccount is a purpose-specific allocation of ng for a franchise holder.
type EarmarkedAccount struct {
	ID          string `json:"id"`
	Holder      string `json:"holder"`
	Purpose     string `json:"purpose"` // ops, reserve, dividends, franchise_dev
	AllocatedNg int64  `json:"allocated_ng"`
	SpentNg     int64  `json:"spent_ng"`
	CreatedAt   string `json:"created_at"`
	LicenseID   string `json:"license_id,omitempty"`
}

// EarmarkManager manages earmarked accounts for franchise funds.
type EarmarkManager struct {
	mu       sync.Mutex
	Accounts map[string]*EarmarkedAccount `json:"accounts"`
}

// NewEarmarkManager creates an earmark manager with an empty account map.
func NewEarmarkManager() *EarmarkManager {
	return &EarmarkManager{Accounts: make(map[string]*EarmarkedAccount)}
}

// LoadEarmarks loads earmarked accounts from disk.
func LoadEarmarks(dataDir string) *EarmarkManager {
	em := NewEarmarkManager()

	fileutil.ReadJSON(filepath.Join(dataDir, "earmarked_accounts.json"), em)
	if em.Accounts == nil {
		em.Accounts = make(map[string]*EarmarkedAccount)
	}
	return em
}

// Save persists earmarked accounts to JSON.
func (em *EarmarkManager) Save(dataDir string) {
	p := filepath.Join(dataDir, "earmarked_accounts.json")
	fileutil.WriteJSON(p, em)
}

// Create creates a new earmarked account and mints the allocation to the holder.
func (em *EarmarkManager) Create(dataDir, id, holder, purpose, licenseID string, allocatedNg int64) (*EarmarkedAccount, error) {
	em.mu.Lock()
	defer em.mu.Unlock()

	if _, exists := em.Accounts[id]; exists {
		return nil, fmt.Errorf("earmark %q already exists", id)
	}
	if allocatedNg <= 0 {
		return nil, fmt.Errorf("allocation must be positive")
	}

	acct := &EarmarkedAccount{
		ID:          id,
		Holder:      holder,
		Purpose:     purpose,
		AllocatedNg: allocatedNg,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
		LicenseID:   licenseID,
	}
	em.Accounts[id] = acct

	// Mint allocation to the holder's ledger
	ledger := LoadLedger(dataDir)
	ledger.Mint(holder, allocatedNg)
	ledger.Save(dataDir)

	return acct, nil
}

// Spend deducts ng from an earmarked account's remaining balance.
func (em *EarmarkManager) Spend(id string, amountNg int64) (*EarmarkedAccount, error) {
	em.mu.Lock()
	defer em.mu.Unlock()

	acct, ok := em.Accounts[id]
	if !ok {
		return nil, fmt.Errorf("earmark %q not found", id)
	}
	remaining := acct.AllocatedNg - acct.SpentNg
	if amountNg > remaining {
		return nil, fmt.Errorf("insufficient earmark: have %d, need %d", remaining, amountNg)
	}
	acct.SpentNg += amountNg
	return acct, nil
}

// Remaining returns the unspent balance in an earmarked account.
func (em *EarmarkManager) Remaining(id string) int64 {
	em.mu.Lock()
	defer em.mu.Unlock()
	acct, ok := em.Accounts[id]
	if !ok {
		return 0
	}
	return acct.AllocatedNg - acct.SpentNg
}

// List returns all earmarked accounts.
func (em *EarmarkManager) List() []*EarmarkedAccount {
	em.mu.Lock()
	defer em.mu.Unlock()
	list := make([]*EarmarkedAccount, 0, len(em.Accounts))
	for _, acct := range em.Accounts {
		list = append(list, acct)
	}
	return list
}

// --- Mint Authorization ---

// MintAuthorization tracks a franchise node's request to mint a banknote.
type MintAuthorization struct {
	ID          string `json:"id"`
	LicenseID   string `json:"license_id"`
	Serial      string `json:"serial"`
	DenomNg     int64  `json:"denomination_ng"`
	Rarity      string `json:"rarity"`
	Approved    bool   `json:"approved"`
	ApprovedAt  string `json:"approved_at,omitempty"`
	RequestedAt string `json:"requested_at"`
	TemplateID  string `json:"template_id,omitempty"`
}

// MintAuthManager manages mint authorization requests from franchise nodes.
type MintAuthManager struct {
	mu    sync.Mutex
	Auths []MintAuthorization `json:"auths"`
}

// NewMintAuthManager creates an empty mint authorization manager.
func NewMintAuthManager() *MintAuthManager {
	return &MintAuthManager{}
}

// LoadMintAuths loads mint authorizations from disk.
func LoadMintAuths(dataDir string) *MintAuthManager {
	m := NewMintAuthManager()

	fileutil.ReadJSON(filepath.Join(dataDir, "mint_authorizations.json"), m)
	if m.Auths == nil {
		m.Auths = []MintAuthorization{}
	}
	return m
}

// Save persists mint authorizations to JSON.
func (m *MintAuthManager) Save(dataDir string) {
	p := filepath.Join(dataDir, "mint_authorizations.json")
	fileutil.WriteJSON(p, m)
}

// Request creates a new mint authorization request for a franchise node.
func (m *MintAuthManager) Request(licenseID, serial, rarity string, denomNg int64) *MintAuthorization {
	m.mu.Lock()
	defer m.mu.Unlock()
	auth := MintAuthorization{
		ID:          fmt.Sprintf("auth-%s-%d", licenseID, len(m.Auths)+1),
		LicenseID:   licenseID,
		Serial:      serial,
		DenomNg:     denomNg,
		Rarity:      rarity,
		RequestedAt: time.Now().UTC().Format(time.RFC3339),
	}
	m.Auths = append(m.Auths, auth)
	return &m.Auths[len(m.Auths)-1]
}

// Approve approves a pending mint authorization by ID.
func (m *MintAuthManager) Approve(id string) (*MintAuthorization, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.Auths {
		if m.Auths[i].ID == id {
			if m.Auths[i].Approved {
				return nil, fmt.Errorf("auth %q already approved", id)
			}
			m.Auths[i].Approved = true
			m.Auths[i].ApprovedAt = time.Now().UTC().Format(time.RFC3339)
			return &m.Auths[i], nil
		}
	}
	return nil, fmt.Errorf("auth %q not found", id)
}

// Pending returns all unapproved mint authorizations.
func (m *MintAuthManager) Pending() []MintAuthorization {
	m.mu.Lock()
	defer m.mu.Unlock()
	var p []MintAuthorization
	for _, a := range m.Auths {
		if !a.Approved {
			p = append(p, a)
		}
	}
	return p
}

// List returns all mint authorizations.
func (m *MintAuthManager) List() []MintAuthorization {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]MintAuthorization, len(m.Auths))
	copy(out, m.Auths)
	return out
}

// --- National Templates ---

// BanknoteTemplate is a franchise-approved design template for minting banknotes.
type BanknoteTemplate struct {
	ID         string `json:"id"`
	LicenseID  string `json:"license_id"`
	Name       string `json:"name"`
	DesignJSON string `json:"design_json"`
	CreatedAt  string `json:"created_at"`
}

// TemplateManager manages franchise banknote design templates.
type TemplateManager struct {
	mu        sync.Mutex
	Templates map[string]*BanknoteTemplate `json:"templates"`
}

// NewTemplateManager creates a template manager with an empty template map.
func NewTemplateManager() *TemplateManager {
	return &TemplateManager{Templates: make(map[string]*BanknoteTemplate)}
}

// LoadTemplates loads banknote templates from disk.
func LoadTemplates(dataDir string) *TemplateManager {
	tm := NewTemplateManager()

	fileutil.ReadJSON(filepath.Join(dataDir, "banknote_templates.json"), tm)
	if tm.Templates == nil {
		tm.Templates = make(map[string]*BanknoteTemplate)
	}
	return tm
}

// Save persists banknote templates to JSON.
func (tm *TemplateManager) Save(dataDir string) {
	p := filepath.Join(dataDir, "banknote_templates.json")
	fileutil.WriteJSON(p, tm)
}

// Create adds a new banknote template for a franchise license.
func (tm *TemplateManager) Create(id, licenseID, name, designJSON string) (*BanknoteTemplate, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if _, exists := tm.Templates[id]; exists {
		return nil, fmt.Errorf("template %q already exists", id)
	}
	t := &BanknoteTemplate{
		ID:         id,
		LicenseID:  licenseID,
		Name:       name,
		DesignJSON: designJSON,
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
	}
	tm.Templates[id] = t
	return t, nil
}

// Get retrieves a template by ID.
func (tm *TemplateManager) Get(id string) (*BanknoteTemplate, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	t, ok := tm.Templates[id]
	if !ok {
		return nil, fmt.Errorf("template %q not found", id)
	}
	return t, nil
}

// List returns all banknote templates.
func (tm *TemplateManager) List() []*BanknoteTemplate {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	list := make([]*BanknoteTemplate, 0, len(tm.Templates))
	for _, t := range tm.Templates {
		list = append(list, t)
	}
	return list
}

// Delete removes a template by ID.
func (tm *TemplateManager) Delete(id string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if _, ok := tm.Templates[id]; !ok {
		return fmt.Errorf("template %q not found", id)
	}
	delete(tm.Templates, id)
	return nil
}

// --- Cross-Franchise Settlement ---

// SettlementRecord is a cross-franchise payment settlement.
type SettlementRecord struct {
	ID           string `json:"id"`
	FromLicense  string `json:"from_license"`
	ToLicense    string `json:"to_license"`
	AmountNg     int64  `json:"amount_ng"`
	Status       string `json:"status"` // pending, completed, failed
	SettledAt    string `json:"settled_at,omitempty"`
	Description  string `json:"description"`
}

// SettlementManager manages cross-franchise settlements.
type SettlementManager struct {
	mu         sync.Mutex
	Settlements []SettlementRecord `json:"settlements"`
	NextID     int                `json:"next_id"`
}

// NewSettlementManager creates a settlement manager with ID counter starting at 1.
func NewSettlementManager() *SettlementManager {
	return &SettlementManager{NextID: 1}
}

// LoadSettlements loads franchise settlements from disk.
func LoadSettlements(dataDir string) *SettlementManager {
	sm := NewSettlementManager()

	fileutil.ReadJSON(filepath.Join(dataDir, "franchise_settlements.json"), sm)
	if sm.Settlements == nil {
		sm.Settlements = []SettlementRecord{}
	}
	return sm
}

// Save persists franchise settlements to JSON.
func (sm *SettlementManager) Save(dataDir string) {
	p := filepath.Join(dataDir, "franchise_settlements.json")
	fileutil.WriteJSON(p, sm)
}

// Create creates a new pending settlement between two franchise licenses.
func (sm *SettlementManager) Create(from, to, desc string, amountNg int64) (*SettlementRecord, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if amountNg <= 0 {
		return nil, fmt.Errorf("settlement amount must be positive")
	}
	id := fmt.Sprintf("settle-%d", sm.NextID)
	sm.NextID++

	s := SettlementRecord{
		ID:          id,
		FromLicense: from,
		ToLicense:   to,
		AmountNg:    amountNg,
		Status:      "pending",
		Description: desc,
	}
	sm.Settlements = append(sm.Settlements, s)
	return &sm.Settlements[len(sm.Settlements)-1], nil
}

// Complete marks a pending settlement as completed.
func (sm *SettlementManager) Complete(id string) (*SettlementRecord, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	for i := range sm.Settlements {
		if sm.Settlements[i].ID == id {
			if sm.Settlements[i].Status != "pending" {
				return nil, fmt.Errorf("settlement %q is not pending", id)
			}
			sm.Settlements[i].Status = "completed"
			sm.Settlements[i].SettledAt = time.Now().UTC().Format(time.RFC3339)
			return &sm.Settlements[i], nil
		}
	}
	return nil, fmt.Errorf("settlement %q not found", id)
}

// List returns all settlement records.
func (sm *SettlementManager) List() []SettlementRecord {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	out := make([]SettlementRecord, len(sm.Settlements))
	copy(out, sm.Settlements)
	return out
}

// --- Royalty Tracking ---

// RoyaltyPayment is a franchise royalty payment record.
type RoyaltyPayment struct {
	ID          string `json:"id"`
	LicenseID   string `json:"license_id"`
	Period      string `json:"period"` // e.g. "2026-06"
	AmountNg    int64  `json:"amount_ng"`
	Paid        bool   `json:"paid"`
	DueAt       string `json:"due_at"`
	PaidAt      string `json:"paid_at,omitempty"`
}

// RoyaltyManager manages franchise royalty payments.
type RoyaltyManager struct {
	mu       sync.Mutex
	Payments []RoyaltyPayment `json:"payments"`
	NextID   int              `json:"next_id"`
}

// NewRoyaltyManager creates a royalty manager with ID counter starting at 1.
func NewRoyaltyManager() *RoyaltyManager {
	return &RoyaltyManager{NextID: 1}
}

// LoadRoyalties loads franchise royalty records from disk.
func LoadRoyalties(dataDir string) *RoyaltyManager {
	rm := NewRoyaltyManager()

	fileutil.ReadJSON(filepath.Join(dataDir, "franchise_royalties.json"), rm)
	if rm.Payments == nil {
		rm.Payments = []RoyaltyPayment{}
	}
	return rm
}

// Save persists franchise royalty records to JSON.
func (rm *RoyaltyManager) Save(dataDir string) {
	p := filepath.Join(dataDir, "franchise_royalties.json")
	fileutil.WriteJSON(p, rm)
}

// CreateDue creates a new unpaid royalty payment for a license period.
func (rm *RoyaltyManager) CreateDue(licenseID, period string, amountNg int64) *RoyaltyPayment {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	id := fmt.Sprintf("royalty-%d", rm.NextID)
	rm.NextID++
	p := RoyaltyPayment{
		ID:        id,
		LicenseID: licenseID,
		Period:    period,
		AmountNg:  amountNg,
		Paid:      false,
		DueAt:     time.Now().UTC().Format(time.RFC3339),
	}
	rm.Payments = append(rm.Payments, p)
	return &rm.Payments[len(rm.Payments)-1]
}

// Pay marks a royalty payment as paid.
func (rm *RoyaltyManager) Pay(id string) (*RoyaltyPayment, error) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	for i := range rm.Payments {
		if rm.Payments[i].ID == id {
			if rm.Payments[i].Paid {
				return nil, fmt.Errorf("royalty %q already paid", id)
			}
			rm.Payments[i].Paid = true
			rm.Payments[i].PaidAt = time.Now().UTC().Format(time.RFC3339)
			return &rm.Payments[i], nil
		}
	}
	return nil, fmt.Errorf("royalty %q not found", id)
}

// Pending returns all unpaid royalty payments.
func (rm *RoyaltyManager) Pending() []RoyaltyPayment {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	var out []RoyaltyPayment
	for _, p := range rm.Payments {
		if !p.Paid {
			out = append(out, p)
		}
	}
	return out
}

// List returns all royalty payments.
func (rm *RoyaltyManager) List() []RoyaltyPayment {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	out := make([]RoyaltyPayment, len(rm.Payments))
	copy(out, rm.Payments)
	return out
}
