// Package api provides HTTP handlers and API endpoints for the simplex-node server
package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ParanoidX/internal/ai"
	"ParanoidX/internal/economy"
	"ParanoidX/internal/middleware"
)

var ollamaBase = "http://127.0.0.1:11434"

// ── Agent DID (b113) ──────────────────────────────────────────────────────────

func StewardDIDHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		did := "did:island:steward-ai-1"
		pubkey := "steward-ai-public-key-1"
		if globalDIDKeys != nil {
			did = globalDIDKeys.StewardDID()
			pubkey = globalDIDKeys.StewardPubkeyB58()
		}
		doc := map[string]any{
			"@context": []string{"https://www.w3.org/ns/did/v1", "https://w3id.org/security/suites/ed25519-2020/v1"},
			"id":       did,
			"alsoKnownAs": []string{
				"did:island:node-1",
				"https://stmaria.org/steward",
			},
			"verificationMethod": []map[string]any{
				{
					"id":              did + "#keys-1",
					"type":            "Ed25519VerificationKey2020",
					"controller":      did,
					"publicKeyBase58": pubkey,
				},
			},
			"authentication": []string{did + "#keys-1"},
			"assertionMethod": []string{did + "#keys-1"},
			"service": []map[string]any{
				{
					"id":              did + "#steward-chat",
					"type":            "AIChatService",
					"serviceEndpoint": "https://" + r.Host + "/api/steward",
				},
				{
					"id":              did + "#ai-chat",
					"type":            "OllamaAIService",
					"serviceEndpoint": "https://" + r.Host + "/api/ai/chat",
				},
			},
		}
		writeJSON(w, map[string]any{"ok": true, "did": doc})
	}
}

// ── AI Radio Content Generator (b114) ─────────────────────────────────────────

func AIRadioContentHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		lang := r.URL.Query().Get("lang")
		if lang == "" {
			lang = "en"
		}
		topic := r.URL.Query().Get("topic")
		if topic == "" {
			topic = "island news and community updates"
		}
		durStr := r.URL.Query().Get("duration")
		if durStr == "" {
			durStr = "30 seconds"
		}

		prompt := fmt.Sprintf(`Generate a %s radio script in %s language about: %s.
Include current island economy context: silver-backed currency, sovereign network, community governance.
Make it conversational, warm, and suitable for spoken broadcast.
Return ONLY the script text, no meta commentary.`, durStr, lang, topic)

		system := "You are Saint Mary Liberty Island Radio AI. You produce broadcast scripts that inform, inspire, and unite the island community."
		script := aiGenerate(prompt, system, 500)

		w.Write([]byte(script))
	}
}

func aiGenerate(prompt, system string, maxTokens int) string {
	body, _ := json.Marshal(map[string]any{
		"model":  "gemma3:latest",
		"prompt": prompt,
		"system": system,
		"stream": false,
		"options": map[string]any{
			"temperature": 0.7,
			"num_predict": maxTokens,
		},
	})
	resp, err := http.Post(ollamaBase+"/api/generate", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Sprintf("Welcome to Saint Mary Liberty Island Radio. %s", prompt[:120])
	}
	defer resp.Body.Close()
	var result struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		io.Copy(io.Discard, resp.Body)
		return "Island Radio AI: Live from Saint Mary Liberty Island."
	}
	if result.Response == "" {
		return "Island Radio AI: Broadcasting from the sovereign network."
	}
	return result.Response
}

// ── Treasury Forecasting (b115) ───────────────────────────────────────────────

type TreasuryForecast struct {
	CurrentReserveNg int64   `json:"current_reserve_ng"`
	TotalSupplyNg    int64   `json:"total_supply_ng"`
	SilverPriceUSD   float64 `json:"silver_price_usd"`
	Projected30dNg   int64   `json:"projected_30d_ng"`
	HealthScore      string  `json:"health_score"`
	Recommendations  []string `json:"recommendations"`
}


// TreasuryForecastHandler handles the TreasuryForecastHandler HTTP request.
func TreasuryForecastHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		reserveNg := int64(0)
		if b, err := os.ReadFile(filepath.Join(dataDir, "silver_reserve_ng.txt")); err == nil {
			s := strings.TrimSpace(string(b))
			for i := range s {
				if s[i] < '0' || s[i] > '9' {
					s = s[:i]
					break
				}
			}
			fmt.Sscanf(s, "%d", &reserveNg)
		}
		ledger := economy.LoadLedger(dataDir)
		totalSupply := ledger.TotalSupply
		silverPrice := 64.18
		if b, err := os.ReadFile(filepath.Join(dataDir, "silver_spot_usd.txt")); err == nil {
			fmt.Sscanf(string(b), "%f", &silverPrice)
		}

		backingRatio := 0.0
		if totalSupply > 0 {
			backingRatio = float64(reserveNg) / float64(totalSupply)
		}

		healthScore := "healthy"
		recs := []string{}
		if backingRatio < 1.0 {
			healthScore = "under-backed"
			recs = append(recs, "Increase silver reserve by minting new rounds")
		}
		if backingRatio < 0.5 {
			healthScore = "critical"
			recs = append(recs, "URGENT: Reserve below 50%. Suspend new minting.")
		}
		if backingRatio >= 1.0 && backingRatio < 2.0 {
			healthScore = "adequate"
			recs = append(recs, "Maintain current reserve levels")
		}
		if backingRatio >= 2.0 {
			healthScore = "strong"
			recs = append(recs, "Consider dividend increase or buyback program")
		}
		recs = append(recs, fmt.Sprintf("Monitor silver price ($%.2f/oz)", silverPrice))

		projected := reserveNg / 2
		if projected > totalSupply*2 {
			projected = totalSupply * 2
		}

		forecast := TreasuryForecast{
			CurrentReserveNg: reserveNg,
			TotalSupplyNg:    totalSupply,
			SilverPriceUSD:   silverPrice,
			Projected30dNg:   projected,
			HealthScore:      healthScore,
			Recommendations:  recs,
		}
		writeJSON(w, map[string]any{"ok": true, "forecast": forecast, "generated_at": time.Now().UTC().Format(time.RFC3339)})
	}
}

// ── AI Moderation Integration (b116) ──────────────────────────────────────────

func ModerationStatsHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		statsFile := filepath.Join(dataDir, "moderation_stats.json")
		var stats map[string]any
		if b, err := os.ReadFile(statsFile); err == nil {
			json.Unmarshal(b, &stats)
		}
		if stats == nil {
			stats = map[string]any{
				"total_checks":   0,
				"flagged":        0,
				"approved":       0,
				"last_check":     nil,
			}
		}
		writeJSON(w, map[string]any{"ok": true, "stats": stats})
	}
}


// StewardChatStreamHandler handles the StewardChatStreamHandler HTTP request.
func StewardChatStreamHandler(steward *ai.Steward) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		if r.Method != "POST" {
			http.Error(w, "POST required", 400)
			return
		}

		var req struct {
			Message string `json:"message"`
			UserID  string `json:"user_id,omitempty"`
			Profile string `json:"profile,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad json", 400)
			return
		}
		if req.Message == "" {
			http.Error(w, "message required", http.StatusBadRequest)
			return
		}
		if req.Profile == "" {
			req.Profile = "steward"
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		var tokenCh <-chan string
		var err error
		if req.UserID != "" {
			tokenCh, err = steward.AskWithMemoryStream(req.Message, req.UserID, req.Profile)
		} else {
			tokenCh, err = steward.AskWithProfileStream(req.Message, "", req.Profile)
		}
		if err != nil {
			fmt.Fprintf(w, "data: {\"error\": %q}\n\n", err.Error())
			flusher.Flush()
			return
		}

		full := strings.Builder{}
		for {
			select {
			case <-r.Context().Done():
				return
			case token, ok := <-tokenCh:
				if !ok {
					fmt.Fprintf(w, "data: {\"done\": true}\n\n")
					flusher.Flush()
					// Persist to memory if streaming completed successfully
					if req.UserID != "" && full.Len() > 0 {
						if ms := steward.Memory; ms != nil {
							ms.Add(req.UserID, "user", req.Message)
							ms.Add(req.UserID, "assistant", full.String())
						}
					}
					return
				}
				full.WriteString(token)
				b, _ := json.Marshal(map[string]string{"token": token})
				fmt.Fprintf(w, "data: %s\n\n", b)
				flusher.Flush()
			}
		}
	}
}
