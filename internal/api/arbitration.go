// Package api provides HTTP handlers and API endpoints for the simplex-node server
package api

import (
	"encoding/json"
	"net/http"

	"ParanoidX/internal/economy"
	"ParanoidX/internal/middleware"
)


// ArbitrationHandler handles the ArbitrationHandler HTTP request.
func ArbitrationHandler(dataDir string) http.HandlerFunc {
	am := economy.NewArbitrationManager(dataDir)

	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}

		switch r.Method {
		case "GET":
			pubkey := r.URL.Query().Get("pubkey")
			id := r.URL.Query().Get("id")

			if id != "" {
				d, err := am.GetDispute(id)
				if err != nil {
					http.Error(w, err.Error(), http.StatusNotFound)
					return
				}
				writeJSON(w, d)
				return
			}

			disputes := am.ListDisputes(pubkey)
			if disputes == nil {
				disputes = []*economy.Dispute{}
			}
			writeJSON(w, map[string]any{
				"count":    len(disputes),
				"disputes": disputes,
			})

		case "POST":
			var req struct {
				Action      string `json:"action"`
				ID          string `json:"id"`
				Initiator   string `json:"initiator"`
				Respondent  string `json:"respondent"`
				Title       string `json:"title"`
				Description string `json:"description"`
				Evidence    string `json:"evidence"`
				Pubkey      string `json:"pubkey"`
				AIResult    string `json:"ai_result"`
				Ruling      string `json:"ruling"`
				RuledBy     string `json:"ruled_by"`
				Reason      string `json:"reason"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "invalid json", http.StatusBadRequest)
				return
			}

			switch req.Action {
			case "create":
				d, err := am.CreateDispute(req.Initiator, req.Respondent, req.Title, req.Description, req.Evidence)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				writeJSON(w, map[string]any{"ok": true, "dispute": d})

			case "respond":
				d, err := am.Respond(req.ID, req.Pubkey, req.Evidence)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				writeJSON(w, map[string]any{"ok": true, "dispute": d})

			case "analyze":
				d, err := am.Analyze(req.ID, req.AIResult)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				writeJSON(w, map[string]any{"ok": true, "dispute": d})

			case "rule":
				d, err := am.IssueRuling(req.ID, req.Ruling, req.RuledBy)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				writeJSON(w, map[string]any{"ok": true, "dispute": d})

			case "appeal":
				d, err := am.Appeal(req.ID, req.Pubkey, req.Reason)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				writeJSON(w, map[string]any{"ok": true, "dispute": d})

			default:
				http.Error(w, "unknown action: "+req.Action, http.StatusBadRequest)
			}

		default:
			http.Error(w, "GET or POST only", http.StatusMethodNotAllowed)
		}
	}
}
