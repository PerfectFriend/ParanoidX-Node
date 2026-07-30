// Package api provides HTTP handlers and API endpoints for the simplex-node server
package api

import (
	"encoding/json"
	"net/http"

	"ParanoidX/internal/economy"
	"ParanoidX/internal/middleware"
)


// OnboardingHandler returns onboarding status for a given pubkey.
func OnboardingHandler(dataDir string) http.HandlerFunc {
	om := economy.NewOnboardingManager(dataDir)

	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}

		pubkey := r.URL.Query().Get("pubkey")
		if pubkey == "" {
			http.Error(w, "pubkey required", http.StatusBadRequest)
			return
		}

		switch r.Method {
		case "GET":
			s := om.Status(pubkey)
			writeJSON(w, map[string]any{
				"pubkey":          s.Pubkey,
				"claimed_welcome": s.ClaimedWelcome,
				"bought_starter":  s.BoughtStarter,
				"completed_guide": s.CompletedGuide,
				"onboarded":       om.IsOnboarded(pubkey),
				"started_at":      s.StartedAt,
			})

		default:
			http.Error(w, "GET only", http.StatusMethodNotAllowed)
		}
	}
}


// OnboardingWelcomeHandler claims the welcome bonus for a pubkey.
func OnboardingWelcomeHandler(dataDir string) http.HandlerFunc {
	om := economy.NewOnboardingManager(dataDir)

	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		if r.Method != "POST" {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}

		pubkey := r.URL.Query().Get("pubkey")
		if pubkey == "" {
			http.Error(w, "pubkey required", http.StatusBadRequest)
			return
		}

		note, err := om.ClaimWelcome(pubkey)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}

		writeJSON(w, map[string]any{
			"ok":     true,
			"serial": note.Serial,
			"denom":  note.DenominationNg,
			"rarity": note.Rarity,
		})
	}
}


// OnboardingStarterHandler purchases the starter pack for a pubkey.
func OnboardingStarterHandler(dataDir string) http.HandlerFunc {
	om := economy.NewOnboardingManager(dataDir)

	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		if r.Method != "POST" {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Pubkey string `json:"pubkey"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.Pubkey == "" {
			http.Error(w, "pubkey required", http.StatusBadRequest)
			return
		}

		notes, err := om.BuyStarter(req.Pubkey)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}

		resp := make([]map[string]any, len(notes))
		for i, n := range notes {
			resp[i] = map[string]any{
				"serial": n.Serial,
				"denom":  n.DenominationNg,
				"rarity": n.Rarity,
			}
		}
		writeJSON(w, map[string]any{
			"ok":      true,
			"notes":   resp,
			"price":   economy.StarterPackPriceNg,
		})
	}
}


// OnboardingGuideHandler marks the onboarding guide as completed for a pubkey.
func OnboardingGuideHandler(dataDir string) http.HandlerFunc {
	om := economy.NewOnboardingManager(dataDir)

	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		if r.Method != "POST" {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}

		pubkey := r.URL.Query().Get("pubkey")
		if pubkey == "" {
			http.Error(w, "pubkey required", http.StatusBadRequest)
			return
		}

		if err := om.CompleteGuide(pubkey); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		writeJSON(w, map[string]any{
			"ok":      true,
			"pubkey":  pubkey,
		})
	}
}
