// Package steward implements the Steward AI service
package steward

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"ParanoidX/internal/ai"
	"ParanoidX/internal/economy"
	"ParanoidX/internal/fileutil"
)

// ActionLog records a steward action.
type ActionLog struct {
	Timestamp   string `json:"timestamp"`
	Action      string `json:"action"`
	Rule        string `json:"rule"`
	Details     string `json:"details"`
	AIDecision  string `json:"ai_decision,omitempty"`
}

// State is the persistent state of the steward.
type State struct {
	mu              sync.Mutex
	Enabled         bool         `json:"enabled"`
	AutoAdjust      bool         `json:"auto_adjust"`
	LastRun         string       `json:"last_run"`
	RunCount        int          `json:"run_count"`
	Actions         []ActionLog  `json:"actions"`
	LastAIAnalysis  string       `json:"last_ai_analysis,omitempty"`
}

// StewardService ties the constitution, monitor, decision engine, and AI.
type StewardService struct {
	Constitution *Constitution
	Monitor      *Monitor
	State        *State
	Params       *economy.DynamicParams
	AISteward    *ai.Steward
	dataDir      string
	tickerDone   chan struct{}
}

// NewService creates a new steward service.
func NewService(dataDir string, aiSteward *ai.Steward) *StewardService {
	s := &StewardService{
		Constitution: DefaultConstitution(),
		Monitor:      NewMonitor(dataDir),
		State:        &State{Enabled: true, AutoAdjust: true, Actions: []ActionLog{}},
		Params:       economy.LoadDynamicParams(dataDir),
		AISteward:    aiSteward,
		dataDir:      dataDir,
		tickerDone:   make(chan struct{}),
	}
	return s
}

// SetAISteward sets the AI Steward after construction.
func (s *StewardService) SetAISteward(st *ai.Steward) {
	s.AISteward = st
}

// Start begins monitoring and the evaluation loop.
func (s *StewardService) Start() {
	s.Monitor.Start()
	slog.Info("steward service started")
	go s.evalLoop()
}

func (s *StewardService) evalLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.Evaluate()
		case <-s.tickerDone:
			return
		}
	}
}

// Stop shuts down the steward.
func (s *StewardService) Stop() {
	s.Monitor.Stop()
	close(s.tickerDone)
}

// Evaluate runs the full analysis pipeline with AI augmentation.
func (s *StewardService) Evaluate() {
	s.State.mu.Lock()
	if !s.State.Enabled {
		s.State.mu.Unlock()
		return
	}
	s.State.mu.Unlock()

	metrics := s.Monitor.GetMetrics()
	deviations := Analyze(s.Constitution, metrics)
	decisions := Decide(s.Constitution, deviations)

	s.State.mu.Lock()
	s.State.LastRun = time.Now().UTC().Format(time.RFC3339)
	s.State.RunCount++

	// AI-powered analysis
	if s.AISteward != nil && len(deviations) > 0 {
		metricsJSON, _ := json.Marshal(map[string]any{
			"metrics":    metrics,
			"deviations": deviations,
			"decisions":  decisions,
		})
		aiResp, err := s.AISteward.EconomySummary(string(metricsJSON))
		if err == nil && aiResp != "" {
			s.State.LastAIAnalysis = aiResp
			slog.Info("steward AI analysis", "analysis", aiResp[:min(len(aiResp), 200)])
		}
	}

	for _, dec := range decisions {
		entry := ActionLog{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Action:    dec.Action,
			Rule:      dec.Rule,
			Details:   dec.Reason,
		}
		if s.State.LastAIAnalysis != "" {
			entry.AIDecision = s.State.LastAIAnalysis
		}
		s.State.Actions = append(s.State.Actions, entry)

		if s.State.AutoAdjust && dec.Action == "auto_adjust" {
			adjusted := s.applyAdjustment(dec)
			if adjusted {
				economy.CurrentParams = s.Params
				s.Params.Save(s.dataDir)
				slog.Info("steward auto-adjusted", "rule", dec.Rule, "reason", dec.Reason)
			}
		}
	}
	s.State.mu.Unlock()

	if len(deviations) > 0 {
		slog.Warn("steward evaluation complete", "deviations", len(deviations), "decisions", len(decisions))
	}
}

func (s *StewardService) applyAdjustment(dec Decision) bool {
	switch dec.Rule {
	case "silver_reserve_ratio":
		return s.Params.Adjust("silver_backing_ratio", 0.70)
	case "treasury_commission_bps":
		return s.Params.Adjust("treasury_commission_bps", 228)
	case "max_total_fee_bps":
		return s.Params.Adjust("max_total_fee_bps", 420)
	case "auction_fee":
		return s.Params.Adjust("auction_listing_fee_bps", 50) &&
			s.Params.Adjust("auction_seller_fee_bps", 100) &&
			s.Params.Adjust("auction_buyer_premium", 250)
	case "auto_recovery":
		s.Params.Adjust("silver_backing_ratio", 0.50)
		s.Params.Adjust("treasury_commission_bps", 400)
		ecoParams := economy.DefaultDynamicParams()
		ecoParams.SilverBackingRatio = 0.50
		ecoParams.TreasuryCommissionBPS = 400
		ecoParams.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		economy.CurrentParams = ecoParams
		s.Params.SilverBackingRatio = 0.50
		s.Params.TreasuryCommissionBPS = 400
		return true
	default:
		return false
	}
}

// Status returns the current steward state.
func (s *StewardService) Status() map[string]any {
	metrics := s.Monitor.GetMetrics()

	s.State.mu.Lock()
	runCount := s.State.RunCount
	lastRun := s.State.LastRun
	enabled := s.State.Enabled
	autoAdj := s.State.AutoAdjust
	actionCount := len(s.State.Actions)
	aiAnalysis := s.State.LastAIAnalysis
	s.State.mu.Unlock()

	recentActions := []ActionLog{}
	s.State.mu.Lock()
	if len(s.State.Actions) > 10 {
		recentActions = s.State.Actions[len(s.State.Actions)-10:]
	} else {
		recentActions = s.State.Actions
	}
	s.State.mu.Unlock()

	return map[string]any{
		"enabled":            enabled,
		"auto_adjust":        autoAdj,
		"run_count":          runCount,
		"last_run":           lastRun,
		"action_count":       actionCount,
		"recent_actions":     recentActions,
		"metrics":            metrics,
		"constitution":       s.Constitution,
		"params":             s.Params.All(),
		"last_ai_analysis":   aiAnalysis,
		"ai_steward_online":  s.AISteward != nil && s.AISteward.Client != nil && s.AISteward.Client.IsAvailable(),
	}
}

// Save persists steward state and params.
func (s *StewardService) Save() {
	s.State.mu.Lock()
	defer s.State.mu.Unlock()
	p := filepath.Join(s.dataDir, "steward_state.json")
	fileutil.WriteJSON(p, s.State)
}

// SaveParams persists economy params.
func (s *StewardService) SaveParams() {
	s.Params.Save(s.dataDir)
}

// LoadState loads steward state from disk.
func (s *StewardService) LoadState() {
	s.State.mu.Lock()
	defer s.State.mu.Unlock()
	p := filepath.Join(s.dataDir, "steward_state.json")
	if b, err := os.ReadFile(p); err == nil {
		json.Unmarshal(b, s.State)
	}
}
