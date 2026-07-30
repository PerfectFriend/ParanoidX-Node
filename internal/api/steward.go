// Package api provides HTTP handlers and API endpoints for the simplex-node server
package api

import (
	"encoding/json"
	"net/http"

	"ParanoidX/internal/steward"
)

// SendGreetingFn is a callback to send a greeting message to a role.
type SendGreetingFn func(role, text string)

// StewardHandler returns the Steward AI service handler for status GET and action POST endpoints.
func StewardHandler(svc *steward.StewardService, sendGreeting SendGreetingFn) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.Method {
		case "GET":
			writeJSON(w, svc.Status())

		case "POST":
			var req struct {
				Action string `json:"action"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "invalid json", http.StatusBadRequest)
				return
			}

			switch req.Action {
			case "evaluate":
				svc.Evaluate()
				svc.Save()
				writeJSON(w, map[string]any{"ok": true, "status": svc.Status()})
			case "enable":
				svc.State.Enabled = true
				svc.Save()
				writeJSON(w, map[string]any{"ok": true, "enabled": true})
			case "disable":
				svc.State.Enabled = false
				svc.Save()
				writeJSON(w, map[string]any{"ok": true, "enabled": false})
			case "toggle_auto_adjust":
				svc.State.AutoAdjust = !svc.State.AutoAdjust
				svc.Save()
				writeJSON(w, map[string]any{"ok": true, "auto_adjust": svc.State.AutoAdjust})
			case "greet":
				if sendGreeting != nil {
					sendGreeting("king", "ДА ЗДРАВСТВУЕТ КОРОЛЕВСКАЯ НОДА!")
				}
				writeJSON(w, map[string]any{"ok": true, "greeting": "sent"})
			default:
				http.Error(w, "unknown action: "+req.Action, http.StatusBadRequest)
			}

		default:
			http.Error(w, "GET or POST only", http.StatusMethodNotAllowed)
		}
	}
}
