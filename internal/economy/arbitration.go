// Package economy implements the island economy system
package economy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"ParanoidX/internal/fileutil"
)

const (
	DisputeStatusOpen      = "open"
	DisputeStatusResponded = "responded"
	DisputeStatusAnalyzed  = "analyzed"
	DisputeStatusRuled     = "ruled"
	DisputeStatusAppealed  = "appealed"
	DisputeStatusClosed    = "closed"
)

// Evidence is a single piece of evidence in a dispute.
type Evidence struct {
	Pubkey    string `json:"pubkey"`
	Content   string `json:"content"`
	Submitted string `json:"submitted_at"`
}

// Dispute represents a dispute between two parties.
type Dispute struct {
	ID              string     `json:"id"`
	Initiator       string     `json:"initiator"`
	Respondent      string     `json:"respondent"`
	Title           string     `json:"title"`
	Description     string     `json:"description"`
	Status          string     `json:"status"`
	Evidence        []Evidence `json:"evidence"`
	AIResult        string     `json:"ai_result,omitempty"`
	Ruling          string     `json:"ruling,omitempty"`
	RuledBy         string     `json:"ruled_by,omitempty"`
	AppealReason    string     `json:"appeal_reason,omitempty"`
	CreatedAt       string     `json:"created_at"`
	UpdatedAt       string     `json:"updated_at"`
}

// ArbitrationManager handles dispute lifecycle.
type ArbitrationManager struct {
	mu      sync.Mutex
	disputes map[string]*Dispute
	dataDir  string
}

// NewArbitrationManager creates a new arbitration manager.
func NewArbitrationManager(dataDir string) *ArbitrationManager {
	am := &ArbitrationManager{
		disputes: make(map[string]*Dispute),
		dataDir:  dataDir,
	}
	am.loadAll()
	return am
}

func (am *ArbitrationManager) disputePath(id string) string {
	return filepath.Join(am.dataDir, "disputes", id+".json")
}

func (am *ArbitrationManager) loadAll() {
	disputesDir := filepath.Join(am.dataDir, "disputes")
	entries, err := os.ReadDir(disputesDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		path := filepath.Join(disputesDir, e.Name())
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var d Dispute
		if err := json.Unmarshal(b, &d); err == nil && d.ID != "" {
			am.disputes[d.ID] = &d
		}
	}
}

func (am *ArbitrationManager) saveDispute(d *Dispute) {
	fileutil.WriteJSON(am.disputePath(d.ID), d)
}

// CreateDispute opens a new dispute between two parties.
func (am *ArbitrationManager) CreateDispute(initiator, respondent, title, description, evidence string) (*Dispute, error) {
	if initiator == "" || respondent == "" {
		return nil, fmt.Errorf("initiator and respondent required")
	}
	if initiator == respondent {
		return nil, fmt.Errorf("cannot dispute with yourself")
	}

	am.mu.Lock()
	defer am.mu.Unlock()

	id := fmt.Sprintf("DSP-%d", time.Now().UnixNano())
	now := time.Now().UTC().Format(time.RFC3339)

	d := &Dispute{
		ID:          id,
		Initiator:   initiator,
		Respondent:  respondent,
		Title:       title,
		Description: description,
		Status:      DisputeStatusOpen,
		Evidence: []Evidence{{
			Pubkey:    initiator,
			Content:   evidence,
			Submitted: now,
		}},
		CreatedAt: now,
		UpdatedAt: now,
	}

	am.disputes[id] = d
	am.saveDispute(d)
	return d, nil
}

// Respond adds the respondent's evidence.
func (am *ArbitrationManager) Respond(id, pubkey, evidence string) (*Dispute, error) {
	am.mu.Lock()
	defer am.mu.Unlock()

	d, ok := am.disputes[id]
	if !ok {
		return nil, fmt.Errorf("dispute %s not found", id)
	}
	if d.Status != DisputeStatusOpen {
		return nil, fmt.Errorf("dispute is %s, not open for response", d.Status)
	}
	if pubkey != d.Respondent {
		return nil, fmt.Errorf("only %s can respond", d.Respondent)
	}

	d.Evidence = append(d.Evidence, Evidence{
		Pubkey:    pubkey,
		Content:   evidence,
		Submitted: time.Now().UTC().Format(time.RFC3339),
	})
	d.Status = DisputeStatusResponded
	d.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	am.saveDispute(d)
	return d, nil
}

// Analyze invokes AI analysis (result stored externally, passed here).
func (am *ArbitrationManager) Analyze(id, aiResult string) (*Dispute, error) {
	am.mu.Lock()
	defer am.mu.Unlock()

	d, ok := am.disputes[id]
	if !ok {
		return nil, fmt.Errorf("dispute %s not found", id)
	}
	if d.Status != DisputeStatusResponded {
		return nil, fmt.Errorf("dispute is %s, must be responded first", d.Status)
	}

	d.AIResult = aiResult
	d.Status = DisputeStatusAnalyzed
	d.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	am.saveDispute(d)
	return d, nil
}

// IssueRuling sets the final ruling.
func (am *ArbitrationManager) IssueRuling(id, ruling, ruledBy string) (*Dispute, error) {
	am.mu.Lock()
	defer am.mu.Unlock()

	d, ok := am.disputes[id]
	if !ok {
		return nil, fmt.Errorf("dispute %s not found", id)
	}
	if d.Status != DisputeStatusAnalyzed {
		return nil, fmt.Errorf("dispute is %s, must be analyzed first", d.Status)
	}

	d.Ruling = ruling
	d.RuledBy = ruledBy
	d.Status = DisputeStatusRuled
	d.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	am.saveDispute(d)
	return d, nil
}

// Appeal allows a party to appeal the ruling.
func (am *ArbitrationManager) Appeal(id, pubkey, reason string) (*Dispute, error) {
	am.mu.Lock()
	defer am.mu.Unlock()

	d, ok := am.disputes[id]
	if !ok {
		return nil, fmt.Errorf("dispute %s not found", id)
	}
	if d.Status != DisputeStatusRuled {
		return nil, fmt.Errorf("dispute is %s, must be ruled first", d.Status)
	}
	if pubkey != d.Initiator && pubkey != d.Respondent {
		return nil, fmt.Errorf("only parties can appeal")
	}

	d.Status = DisputeStatusAppealed
	d.AppealReason = reason
	d.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	am.saveDispute(d)
	return d, nil
}

// GetDispute returns a single dispute by ID.
func (am *ArbitrationManager) GetDispute(id string) (*Dispute, error) {
	am.mu.Lock()
	defer am.mu.Unlock()

	d, ok := am.disputes[id]
	if !ok {
		return nil, fmt.Errorf("dispute %s not found", id)
	}
	return d, nil
}

// ListDisputes returns all disputes, optionally filtered by pubkey.
func (am *ArbitrationManager) ListDisputes(pubkey string) []*Dispute {
	am.mu.Lock()
	defer am.mu.Unlock()

	var result []*Dispute
	for _, d := range am.disputes {
		if pubkey == "" || d.Initiator == pubkey || d.Respondent == pubkey {
			result = append(result, d)
		}
	}
	return result
}
