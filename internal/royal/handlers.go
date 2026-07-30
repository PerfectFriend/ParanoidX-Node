// Package royal implements the Royal API with treasury and economy endpoints
package royal

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"ParanoidX/internal/middleware"
)

// RegisterHandler handles POST /api/royal/register (sub-node registration).
func RegisterHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !middleware.IsLocalOrOnionAccess(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		var req struct {
			Pubkey string `json:"pubkey"`
			Label  string `json:"label"`
			Addr   string `json:"addr"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad json", 400)
			return
		}
		if req.Pubkey == "" {
			http.Error(w, "pubkey required", 400)
			return
		}
		node, err := svc.RegisterNode(req.Pubkey, req.Label, req.Addr)
		if err != nil {
			http.Error(w, err.Error(), 409)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "node": node})
	}
}

// NodesHandler handles GET /api/royal/nodes (list nodes).
func NodesHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nodes := svc.ListNodes()
		contactLink := ""
		if b, err := os.ReadFile(filepath.Join(svc.DataDir, "island_contact_link.txt")); err == nil {
			contactLink = string(b)
		}
		writeJSON(w, map[string]any{
			"nodes":         nodes,
			"royal_pubkey":  svc.PublicKeyHex(),
			"contact_link":  contactLink,
		})
	}
}

// CommandHandler handles POST /api/royal/command (send signed command).
func CommandHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !middleware.IsLocalOrOnionAccess(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		var req struct {
			Command string `json:"command"`
			Target  string `json:"target"` // sub-node pubkey
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad json", 400)
			return
		}
		if req.Command == "" || req.Target == "" {
			http.Error(w, "command and target required", 400)
			return
		}
		sc, err := svc.SignCommand(req.Command, req.Target)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "signed_command": sc})
	}
}

// HeartbeatHandler handles POST /api/royal/heartbeat (sub-node ping).
func HeartbeatHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pubkey := r.URL.Query().Get("pubkey")
		if pubkey == "" {
			http.Error(w, "pubkey required", 400)
			return
		}
		svc.Heartbeat(pubkey)
		writeJSON(w, map[string]any{"ok": true, "pubkey": pubkey})
	}
}

// KeyHandler returns the royal node's public key.
func KeyHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"pubkey": svc.PublicKeyHex()})
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
