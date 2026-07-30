// Package economy implements the island economy system
package economy

import (
		"fmt"
		"path/filepath"
	"sync"
	"time"

	"ParanoidX/internal/fileutil"
)

// TreasuryTier represents the state of the treasury based on reserve multiples.
type TreasuryTier int

const (
	TierThin     TreasuryTier = 0 // <3x monthly ops
	TierNormal   TreasuryTier = 1 // 3-6x
	TierFat      TreasuryTier = 2 // 6-12x
	TierVeryFat  TreasuryTier = 3 // 12x+
)

// DetectTier returns the tier for a given treasury reserve.
func DetectTier(treasuryNg int64, config TreasuryConfig) TreasuryTier {
	multiples := treasuryNg / config.MonthlyOpsNg
	switch {
	case multiples < config.Threshold3x:
		return TierThin
	case multiples < config.Threshold6x:
		return TierNormal
	case multiples < config.Threshold12x:
		return TierFat
	default:
		return TierVeryFat
	}
}


// String handles the String HTTP request.
func (t TreasuryTier) String() string {
	switch t {
	case TierThin:
		return "thin"
	case TierNormal:
		return "normal"
	case TierFat:
		return "fat"
	case TierVeryFat:
		return "very_fat"
	default:
		return "unknown"
	}
}

// BanknoteSet defines a set of banknotes to mint at a given tier.
type BanknoteSet struct {
	Tier        TreasuryTier `json:"tier"`
	Label       string       `json:"label"`
	Serials     []string     `json:"serials"`
	DenomNg     int64        `json:"denomination_ng"`
	Rarity      string       `json:"rarity"`
	Minted      bool         `json:"minted"`
	MintedAt    string       `json:"minted_at,omitempty"`
}

// AutoMintSchedule holds the schedule of banknote sets per tier.
type AutoMintSchedule struct {
	mu          sync.Mutex
	Sets        []BanknoteSet `json:"sets"`
	TriggeredAt string        `json:"triggered_at,omitempty"`
	LastTier    TreasuryTier  `json:"last_tier"`
}

// NewDefaultSchedule creates a schedule with predefined sets.
func NewDefaultSchedule() *AutoMintSchedule {
	return &AutoMintSchedule{
		Sets: []BanknoteSet{
			// Normal tier: 3 common banknotes
			{Tier: TierNormal, Label: "Isle Founding", Serials: []string{"MB-COMMON-AUTO-001", "MB-COMMON-AUTO-002", "MB-COMMON-AUTO-003"}, DenomNg: NGPerTLR, Rarity: "common"},
			// Fat tier: 2 rare banknotes
			{Tier: TierFat, Label: "Silver Prosperity", Serials: []string{"MB-RARE-AUTO-001", "MB-RARE-AUTO-002"}, DenomNg: 5 * NGPerTLR, Rarity: "rare"},
			// Very fat tier: 1 epic banknote
			{Tier: TierVeryFat, Label: "Golden Era", Serials: []string{"MB-EPIC-AUTO-001"}, DenomNg: 25 * NGPerTLR, Rarity: "epic"},
		},
	}
}

// LoadOrCreateSchedule loads schedule from disk or creates a new one.
func LoadOrCreateSchedule(dataDir string) *AutoMintSchedule {
	s := NewDefaultSchedule()

	fileutil.ReadJSON(filepath.Join(dataDir, "auto_mint_schedule.json"), s)
	return s
}


// Save handles the Save HTTP request.
func (s *AutoMintSchedule) Save(dataDir string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := filepath.Join(dataDir, "auto_mint_schedule.json")
	fileutil.WriteJSON(p, s)
}

// CheckAndMint checks the current treasury tier and mints any new banknote sets.
// Returns minted banknotes (to be added to registry) and any triggers that fired.
func (s *AutoMintSchedule) CheckAndMint(treasuryNg int64, config TreasuryConfig, dataDir string) ([]BanknoteV2, []string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	currentTier := DetectTier(treasuryNg, config)
	var newBanknotes []BanknoteV2
	var triggers []string
	now := time.Now().UTC().Format(time.RFC3339)

	for i := range s.Sets {
		set := &s.Sets[i]
		if set.Minted {
			continue
		}
		if set.Tier > s.LastTier && set.Tier <= currentTier {
			set.Minted = true
			set.MintedAt = now
			triggers = append(triggers, fmt.Sprintf("%s (%s)", set.Label, set.Tier.String()))

			for _, serial := range set.Serials {
				newBanknotes = append(newBanknotes, BanknoteV2{
					Serial:         serial,
					DenominationNg: set.DenomNg,
					Rarity:         set.Rarity,
					Multiplier:     rarityMultiplier[set.Rarity],
					Holder:         "treasury",
					FrozenNg:       set.DenomNg,
					Status:         "active",
					MintedAt:       now,
				})
			}
		}
	}

	if currentTier > s.LastTier {
		s.LastTier = currentTier
		s.TriggeredAt = now
	}

	return newBanknotes, triggers, nil
}

// MergeMintedBanknotes adds newly minted banknotes to the registry.
func MergeMintedBanknotes(dataDir string, newNotes []BanknoteV2) error {
	if len(newNotes) == 0 {
		return nil
	}
	banknotes, err := LoadBanknotesV2(dataDir)
	if err != nil {
		return err
	}
	banknotes = append(banknotes, newNotes...)
	SaveBanknotesV2(dataDir, banknotes)
	return nil
}
