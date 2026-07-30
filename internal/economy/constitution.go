// Package economy implements the island economy system
package economy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"ParanoidX/internal/fileutil"
)

// Constitution defines the rules for AI Steward decision-making.
type Constitution struct {
	mu             sync.Mutex
	Version        int               `json:"version"`
	LastAmended    string            `json:"last_amended"`
	Articles       []Article         `json:"articles"`
}

// Article is a single numbered article within the constitution.
type Article struct {
	Number  int    `json:"number"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

// SystemState is a snapshot of the economic system for Steward context.
type SystemState struct {
	ReserveNg     int64   `json:"reserve_ng"`
	TotalSupply   int64   `json:"total_supply_ng"`
	TotalBurned   int64   `json:"total_burned_ng"`
	ActiveUsers   int     `json:"active_users"`
	SilverPrice   float64 `json:"silver_price_usd"`
	LastSilverRound string `json:"last_silver_round"`
	FranchiseCount int   `json:"franchise_count"`
}

// StewardDecision records a decision made by the AI Steward.
type StewardDecision struct {
	ID          string `json:"id"`
	Type        string `json:"type"` // silver_round, moderation, treasury_action, policy_change
	Input       string `json:"input"`
	Decision    string `json:"decision"`
	Reasoning   string `json:"reasoning"`
	Executed    bool   `json:"executed"`
	Timestamp   string `json:"timestamp"`
}

// DecisionLog is a persistent log of Steward decisions.
type DecisionLog struct {
	mu        sync.Mutex
	Decisions []StewardDecision `json:"decisions"`
}

// NewDecisionLog creates an empty decision log.
func NewDecisionLog() *DecisionLog {
	return &DecisionLog{}
}

// LoadDecisionLog loads steward decisions from disk, or returns empty if missing.
func LoadDecisionLog(dataDir string) *DecisionLog {
	d := NewDecisionLog()

	fileutil.ReadJSON(filepath.Join(dataDir, "steward_decisions.json"), d)
	if d.Decisions == nil {
		d.Decisions = []StewardDecision{}
	}
	return d
}

// Save persists the decision log to JSON.
func (d *DecisionLog) Save(dataDir string) {
	p := filepath.Join(dataDir, "steward_decisions.json")
	fileutil.WriteJSON(p, d)
}

// Record appends a Steward decision to the log.
func (d *DecisionLog) Record(sd StewardDecision) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.Decisions = append(d.Decisions, sd)
}

// Recent returns the N most recent decisions.
func (d *DecisionLog) Recent(n int) []StewardDecision {
	d.mu.Lock()
	defer d.mu.Unlock()
	if n <= 0 || n > len(d.Decisions) {
		n = len(d.Decisions)
	}
	return d.Decisions[len(d.Decisions)-n:]
}

// DefaultConstitution returns the initial 10-article constitution with silver backing rules.
func DefaultConstitution() *Constitution {
	return &Constitution{
		Version:     1,
		LastAmended: time.Now().UTC().Format(time.RFC3339),
		Articles: []Article{
			{Number: 1, Title: "Silver Backing", Content: "Every 1 TLR in circulation is backed by 1 troy oz of silver in the treasury reserve. Proof of Reserve must be published daily."},
			{Number: 2, Title: "Treasury Split", Content: "Treasury commission is 2.28% on all transactions (operational budget). Total fees capped at 4.20%. Excess above 2.28% goes to dividend pool. Treasury is split by tier: thin (75% ops, 25% reserve), normal (50% ops, 25% reserve, 25% insurance), fat (20% ops, 30% reserve, 50% dividends), very fat (10% ops, 20% reserve, 30% dividends, 40% burn)."},
			{Number: 3, Title: "Subscription Tiers", Content: "Citizenship costs 2B ng/mo ($4.82, vault 2GB, 0% P2P fee, voting, POS terminal). Aristocrat costs 20B ng/mo ($48.20, x4 dividends, auto-bid, priority POS). Colonist is free with limited features."},
			{Number: 4, Title: "Golden Wheel", Content: "Daily loyalty spin available to all citizens. Rewards scale by rarity: common (60%, 1-5 TLR), rare (25%, 10-25 TLR), epic (10%, 50-100 TLR), legendary (5%, 250-500 TLR)."},
			{Number: 5, Title: "Crafting", Content: "5 banknotes of the same rarity can be upgraded to 1 of the next tier. Common→Rare→Epic→Legendary. Genesis cannot be crafted."},
			{Number: 6, Title: "Auto-Mint", Content: "When treasury reaches Normal tier (3x monthly ops), mint the Isle Founding set. At Fat tier (6x), mint Silver Prosperity. At Very Fat tier (12x), mint Golden Era."},
			{Number: 7, Title: "Deflation", Content: "At Very Fat tier (12x+ monthly ops), 40% of treasury is burned to reduce supply and prevent inflation."},
			{Number: 8, Title: "Franchise Rights", Content: "Licensed franchise nodes may mint authorized banknotes using approved templates. Standard tier: 1 node, 1B ng/mo ($2.41). Premium: 5 nodes, 5B ng/mo ($12.05). Royal: unlimited, 25B ng/mo ($60.25)."},
			{Number: 9, Title: "Auditor Governance", Content: "Top 10 holders by weighted banknote value form the auditor panel. First investor holds permanent seat. Auditors vote on treasury policy changes."},
			{Number: 10, Title: "Amendment", Content: "This constitution may be amended by Royal decree after auditor panel consultation. Amendments are logged and published."},
		},
	}
}

// LoadConstitution loads the constitution from disk, falling back to the default.
func LoadConstitution(dataDir string) *Constitution {
	p := filepath.Join(dataDir, "constitution.json")
	c := DefaultConstitution()
	if b, err := os.ReadFile(p); err == nil {
		json.Unmarshal(b, c)
	}
	return c
}

// Save persists the constitution to JSON.
func (c *Constitution) Save(dataDir string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	p := filepath.Join(dataDir, "constitution.json")
	fileutil.WriteJSON(p, c)
}

// GetArticle returns a constitution article by number, or nil if not found.
func (c *Constitution) GetArticle(n int) *Article {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, a := range c.Articles {
		if a.Number == n {
			return &a
		}
	}
	return nil
}
