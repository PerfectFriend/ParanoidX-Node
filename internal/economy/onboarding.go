// Package economy implements the island economy system
package economy

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"ParanoidX/internal/fileutil"
)

// OnboardingState tracks a user's progress through the new-user onboarding flow.
type OnboardingState struct {
	mu              sync.Mutex
	Pubkey          string   `json:"pubkey"`
	ClaimedWelcome  bool     `json:"claimed_welcome"`
	BoughtStarter   bool     `json:"bought_starter"`
	CompletedGuide  bool     `json:"completed_guide"`
	StartedAt       string   `json:"started_at"`
}

// OnboardingManager manages per-user onboarding state persistence and rewards.
type OnboardingManager struct {
	states map[string]*OnboardingState
	mu     sync.RWMutex
	dataDir string
}

// NewOnboardingManager creates an onboarding manager for the given data directory.
func NewOnboardingManager(dataDir string) *OnboardingManager {
	return &OnboardingManager{states: make(map[string]*OnboardingState), dataDir: dataDir}
}

func (om *OnboardingManager) load(dataDir, pubkey string) *OnboardingState {
	path := filepath.Join(dataDir, "onboarding", pubkey+".json")
	s := &OnboardingState{}
	fileutil.ReadJSON(path, s)
	return s
}

func (om *OnboardingManager) save(dataDir string, s *OnboardingState) {
	os.MkdirAll(filepath.Join(dataDir, "onboarding"), 0700)
	path := filepath.Join(dataDir, "onboarding", s.Pubkey+".json")
	fileutil.WriteJSON(path, s)
}

// Status returns the current onboarding state for a pubkey, creating one if missing.
func (om *OnboardingManager) Status(pubkey string) *OnboardingState {
	om.mu.RLock()
	s, ok := om.states[pubkey]
	om.mu.RUnlock()
	if !ok {
		s = om.load(om.dataDir, pubkey)
		om.mu.Lock()
		om.states[pubkey] = s
		om.mu.Unlock()
	}
	if s.Pubkey == "" {
		s.Pubkey = pubkey
		s.StartedAt = time.Now().UTC().Format(time.RFC3339)
	}
	return s
}

// ClaimWelcome mints the free welcome banknote (1 common TLR) for a new user.
func (om *OnboardingManager) ClaimWelcome(pubkey string) (*BanknoteV2, error) {
	om.mu.Lock()
	defer om.mu.Unlock()

	s, ok := om.states[pubkey]
	if !ok {
		s = om.load(om.dataDir, pubkey)
		om.states[pubkey] = s
	}

	if s.Pubkey == "" {
		s.Pubkey = pubkey
		s.StartedAt = time.Now().UTC().Format(time.RFC3339)
	}

	if s.ClaimedWelcome {
		return nil, fmt.Errorf("welcome banknote already claimed")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	note := BanknoteV2{
		Serial:         fmt.Sprintf("MB-COMMON-WELCOME-%s", pubkey),
		DenominationNg: NGPerTLR,
		Rarity:         "common",
		Multiplier:     1,
		Holder:         pubkey,
		FrozenNg:       NGPerTLR,
		Status:         "active",
		MintedAt:       now,
	}

	banknotes, _ := LoadBanknotesV2(om.dataDir)
	banknotes = append(banknotes, note)
	SaveBanknotesV2(om.dataDir, banknotes)

	s.ClaimedWelcome = true
	om.save(om.dataDir, s)

	return &note, nil
}

// StarterPackPriceNg is the price of the starter pack (5 TLR worth of banknotes).
const StarterPackPriceNg = 5 * NGPerTLR

// BuyStarter purchases the starter pack for a user, containing 3 common, 1 rare, and 1 epic banknote.
func (om *OnboardingManager) BuyStarter(pubkey string) ([]BanknoteV2, error) {
	om.mu.Lock()
	defer om.mu.Unlock()

	s, ok := om.states[pubkey]
	if !ok {
		s = om.load(om.dataDir, pubkey)
		om.states[pubkey] = s
	}

	if s.Pubkey == "" {
		s.Pubkey = pubkey
		s.StartedAt = time.Now().UTC().Format(time.RFC3339)
	}

	if s.BoughtStarter {
		return nil, fmt.Errorf("starter pack already purchased")
	}

	ledger := LoadLedger(om.dataDir)
	bal := ledger.Balance(pubkey)
	if bal < StarterPackPriceNg {
		return nil, fmt.Errorf("insufficient balance: need %d ng, have %d ng", StarterPackPriceNg, bal)
	}

	ledger.Mint(pubkey, -StarterPackPriceNg)
	ledger.Save(om.dataDir)

	now := time.Now().UTC().Format(time.RFC3339)
	notes := []BanknoteV2{
		{Serial: fmt.Sprintf("MB-COMMON-STARTER-%s-1", pubkey), DenominationNg: NGPerTLR, Rarity: "common", Multiplier: 1, Holder: pubkey, FrozenNg: NGPerTLR, Status: "active", MintedAt: now},
		{Serial: fmt.Sprintf("MB-COMMON-STARTER-%s-2", pubkey), DenominationNg: NGPerTLR, Rarity: "common", Multiplier: 1, Holder: pubkey, FrozenNg: NGPerTLR, Status: "active", MintedAt: now},
		{Serial: fmt.Sprintf("MB-COMMON-STARTER-%s-3", pubkey), DenominationNg: NGPerTLR, Rarity: "common", Multiplier: 1, Holder: pubkey, FrozenNg: NGPerTLR, Status: "active", MintedAt: now},
		{Serial: fmt.Sprintf("MB-RARE-STARTER-%s", pubkey), DenominationNg: NGPerTLR, Rarity: "rare", Multiplier: 2, Holder: pubkey, FrozenNg: NGPerTLR, Status: "active", MintedAt: now},
		{Serial: fmt.Sprintf("MB-EPIC-STARTER-%s", pubkey), DenominationNg: NGPerTLR, Rarity: "epic", Multiplier: 5, Holder: pubkey, FrozenNg: NGPerTLR, Status: "active", MintedAt: now},
	}

	banknotes, _ := LoadBanknotesV2(om.dataDir)
	banknotes = append(banknotes, notes...)
	SaveBanknotesV2(om.dataDir, banknotes)

	s.BoughtStarter = true
	om.save(om.dataDir, s)

	return notes, nil
}

// CompleteGuide marks the interactive guide as completed for a user.
func (om *OnboardingManager) CompleteGuide(pubkey string) error {
	om.mu.Lock()
	defer om.mu.Unlock()

	s, ok := om.states[pubkey]
	if !ok {
		s = om.load(om.dataDir, pubkey)
		om.states[pubkey] = s
	}

	if s.Pubkey == "" {
		s.Pubkey = pubkey
		s.StartedAt = time.Now().UTC().Format(time.RFC3339)
	}

	s.CompletedGuide = true
	om.save(om.dataDir, s)
	return nil
}

// IsOnboarded returns true if the user has completed all onboarding steps.
func (om *OnboardingManager) IsOnboarded(pubkey string) bool {
	s := om.Status(pubkey)
	return s.ClaimedWelcome && s.BoughtStarter && s.CompletedGuide
}
