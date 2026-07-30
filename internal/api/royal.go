// Package api provides HTTP handlers and API endpoints for the simplex-node server
package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"ParanoidX/internal/fileutil"
	"ParanoidX/internal/middleware"
)

type SubNode struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Address   string `json:"address"`
	Status    string `json:"status"`
	LastSeen  string `json:"last_seen"`
	PublicKey string `json:"public_key,omitempty"`
}

type RelayCommand struct {
	ID        string `json:"id"`
	SubID     string `json:"sub_id"`
	Command   string `json:"command"`
	Data      string `json:"data,omitempty"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	Result    string `json:"result,omitempty"`
}

func subNodesFile(dataDir string) string {
	return filepath.Join(dataDir, "sub_nodes.json")
}

func relayFile(dataDir string) string {
	return filepath.Join(dataDir, "relay_commands.json")
}

func loadSubNodes(dataDir string) []SubNode {
	var nodes []SubNode
	b, err := os.ReadFile(subNodesFile(dataDir))
	if err == nil {
		json.Unmarshal(b, &nodes)
	}
	return nodes
}

func saveSubNodes(dataDir string, nodes []SubNode) {
	fileutil.WriteJSON(subNodesFile(dataDir), nodes)
}

func loadRelayCommands(dataDir string) []RelayCommand {
	var cmds []RelayCommand
	b, err := os.ReadFile(relayFile(dataDir))
	if err == nil {
		json.Unmarshal(b, &cmds)
	}
	return cmds
}

func saveRelayCommands(dataDir string, cmds []RelayCommand) {
	fileutil.WriteJSON(relayFile(dataDir), cmds)
}


// RoyalControlHandler handles the RoyalControlHandler HTTP request.
func RoyalControlHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		if !isRoyalNode(dataDir) {
			http.Error(w, "royal control only on master node", http.StatusForbidden)
			return
		}

		// Detect sync path vs control path
		isSync := r.URL.Path == "/royal/sync"
		action := r.URL.Query().Get("action")
		if isSync && action == "" {
			action = "sync"
		}
		switch action {
		case "relay":
			subID := r.URL.Query().Get("sub_id")
			cmd := r.URL.Query().Get("cmd")
			if subID == "" || cmd == "" {
				http.Error(w, "sub_id and cmd required", http.StatusBadRequest)
				return
			}
			cmds := loadRelayCommands(dataDir)
			rc := RelayCommand{
				ID:        fmt.Sprintf("relay-%d", time.Now().UnixMilli()),
				SubID:     subID,
				Command:   cmd,
				Data:      r.URL.Query().Get("data"),
				Status:    "pending",
				CreatedAt: time.Now().Format(time.RFC3339),
				UpdatedAt: time.Now().Format(time.RFC3339),
			}
			cmds = append(cmds, rc)
			saveRelayCommands(dataDir, cmds)
			slog.Info("royal relay", "sub", subID, "cmd", cmd)
			writeJSON(w, map[string]any{"ok": true, "relay": rc})

		case "register-sub":
			var sub SubNode
			if err := json.NewDecoder(r.Body).Decode(&sub); err != nil {
				http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
				return
			}
			if sub.ID == "" || sub.Name == "" {
				http.Error(w, "id and name required", http.StatusBadRequest)
				return
			}
			nodes := loadSubNodes(dataDir)
			sub.Status = "registered"
			sub.LastSeen = time.Now().Format(time.RFC3339)
			updated := false
			for i := range nodes {
				if nodes[i].ID == sub.ID {
					nodes[i] = sub
					updated = true
					break
				}
			}
			if !updated {
				nodes = append(nodes, sub)
			}
			saveSubNodes(dataDir, nodes)
			slog.Info("royal register sub", "id", sub.ID, "name", sub.Name)
			writeJSON(w, map[string]any{"ok": true, "sub": sub})

		case "list-subs":
			nodes := loadSubNodes(dataDir)
			writeJSON(w, map[string]any{"count": len(nodes), "subs": nodes})

		case "list-relays":
			cmds := loadRelayCommands(dataDir)
			writeJSON(w, map[string]any{"count": len(cmds), "relays": cmds})

		case "poll":
			subID := r.URL.Query().Get("sub_id")
			if subID == "" {
				http.Error(w, "sub_id required", http.StatusBadRequest)
				return
			}
			cmds := loadRelayCommands(dataDir)
			var pending []RelayCommand
			for _, c := range cmds {
				if c.SubID == subID && c.Status == "pending" {
					pending = append(pending, c)
				}
			}
			writeJSON(w, map[string]any{"count": len(pending), "relays": pending})

		case "ack":
			relayID := r.URL.Query().Get("relay_id")
			result := r.URL.Query().Get("result")
			if relayID == "" {
				http.Error(w, "relay_id required", http.StatusBadRequest)
				return
			}
			cmds := loadRelayCommands(dataDir)
			updated := false
			for i := range cmds {
				if cmds[i].ID == relayID {
					cmds[i].Status = "completed"
					cmds[i].Result = result
					cmds[i].UpdatedAt = time.Now().Format(time.RFC3339)
					updated = true
					break
				}
			}
			if updated {
				saveRelayCommands(dataDir, cmds)
			}
			writeJSON(w, map[string]any{"ok": true, "ack": relayID})

		case "heartbeat":
			subID := r.URL.Query().Get("sub_id")
			if subID == "" {
				http.Error(w, "sub_id required", http.StatusBadRequest)
				return
			}
			nodes := loadSubNodes(dataDir)
			for i := range nodes {
				if nodes[i].ID == subID {
					nodes[i].LastSeen = time.Now().Format(time.RFC3339)
					nodes[i].Status = "online"
					saveSubNodes(dataDir, nodes)
					break
				}
			}
			writeJSON(w, map[string]any{"ok": true})

		case "sync":
			subID := r.URL.Query().Get("sub_id")
			slog.Info("royal sync", "sub_id", subID)
			writeJSON(w, map[string]any{"ok": true, "note": "Sync acknowledged"})

		default:
			http.Error(w, "unknown action (relay, register-sub, list-subs, list-relays, poll, ack, heartbeat, sync)", http.StatusBadRequest)
		}
	}
}
