// Package api provides HTTP handlers and API endpoints for the simplex-node server
package api

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"ParanoidX/internal/container"
	"ParanoidX/internal/crypto/bip39"

	"log/slog"
)

// GlobalContainer is the singleton crypto container instance.
var GlobalContainer *container.Container

// AutoDeleteConfig controls automatic message deletion behavior (enabled, period, paranoid mode).
type AutoDeleteConfig struct {
	Enabled    bool          `json:"enabled"`
	Period     time.Duration `json:"period_ns"`
	PeriodStr  string        `json:"period"`
	Paranoid   bool          `json:"paranoid"`
}

// ChatAutoDelete is the global auto-delete configuration for chat messages.
var ChatAutoDelete = AutoDeleteConfig{}

// Container entry names for config backup inside the vault.
const (
	EntryConfigJSON    = "config.json"
	EntryAddressesJSON = "addresses.json"
	EntryTokensJSON    = "tokens.json"
)

// DockerComposeDir is set from main.go so PanicHandler can stop containers.
var DockerComposeDir string

// DataDir is set from main.go so file operations work.
var DataDir string

// ContainerGenerateSeedHandler generates a new BIP39 mnemonic seed phrase for the container.
func ContainerGenerateSeedHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mnemonic, err := bip39.GenerateMnemonic()
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "seed_phrase": mnemonic, "words": len(strings.Fields(mnemonic))})
	}
}

// ContainerInitHandler initialises a new crypto container from a seed phrase.
func ContainerInitHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		if GlobalContainer == nil {
			writeJSON(w, map[string]any{"ok": false, "error": "container not initialized"})
			return
		}
		if GlobalContainer.HasContainer() {
			writeJSON(w, map[string]any{"ok": false, "error": "container already exists"})
			return
		}
		var req struct {
			SeedPhrase string `json:"seed_phrase"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
			return
		}
		if req.SeedPhrase == "" {
			writeJSON(w, map[string]any{"ok": false, "error": "seed_phrase required"})
			return
		}
		if err := GlobalContainer.Init(req.SeedPhrase); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	}
}

// ContainerOpenHandler opens an existing crypto container using the seed phrase.
func ContainerOpenHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		if GlobalContainer == nil {
			writeJSON(w, map[string]any{"ok": false, "error": "container not initialized"})
			return
		}
		var req struct {
			SeedPhrase string `json:"seed_phrase"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
			return
		}
		if req.SeedPhrase == "" {
			writeJSON(w, map[string]any{"ok": false, "error": "seed_phrase required"})
			return
		}
		if err := GlobalContainer.Open(req.SeedPhrase); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	}
}

// ContainerCloseHandler locks and closes the crypto container.
func ContainerCloseHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		if GlobalContainer == nil {
			writeJSON(w, map[string]any{"ok": false, "error": "container not initialized"})
			return
		}
		if err := GlobalContainer.Close(); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	}
}

// ContainerStatusHandler returns the container state (open/closed, entry count).
func ContainerStatusHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if GlobalContainer == nil {
			writeJSON(w, map[string]any{"ok": false, "error": "container not initialized"})
			return
		}
		entries := GlobalContainer.List()
		writeJSON(w, map[string]any{
			"opened":  GlobalContainer.IsOpen(),
			"exists":  GlobalContainer.HasContainer(),
			"entries": entries,
		})
	}
}

// ContainerImportConfigHandler reads critical config files from dataDir
// and stores them inside the container as encrypted entries.
func ContainerImportConfigHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		if GlobalContainer == nil || !GlobalContainer.IsOpen() {
			writeJSON(w, map[string]any{"ok": false, "error": "container not open"})
			return
		}

		imported := map[string]string{}

		// 1. Main config: simplex-node.json
		cfgPath := filepath.Join(DataDir, "ParanoidX.json")
		if data, err := os.ReadFile(cfgPath); err == nil {
			if err := GlobalContainer.Store(EntryConfigJSON, data); err != nil {
				slog.Error("container import config", "error", err)
			} else {
				imported["config.json"] = "ParanoidX.json"
			}
		}

		// 2. Address files: smp, xftp, ice, auditor, contact
		addrFiles := map[string]string{
			"smp_client_address.txt":    "smp_address",
			"xftp_client_address.txt":   "xftp_address",
			"ice_onion.txt":             "ice_onion",
			"auditor_onion.txt":         "auditor_onion",
			"island_contact_link.txt":   "contact_link",
			"dashboard_onion.txt":       "dashboard_onion",
		}
		addrData := make(map[string]string)
		for file, key := range addrFiles {
			if data, err := os.ReadFile(filepath.Join(DataDir, file)); err == nil {
				addrData[key] = strings.TrimSpace(string(data))
			}
		}
		if len(addrData) > 0 {
			data, _ := json.Marshal(addrData)
			if err := GlobalContainer.Store(EntryAddressesJSON, data); err != nil {
				slog.Error("container import addresses", "error", err)
			} else {
				imported["addresses.json"] = "node addresses"
			}
		}

		// 3. Token files from config (telegram bot tokens, API keys)
		tokenFiles := map[string]string{
			"ask_steward_token":  filepath.Join(DataDir, "ask_steward_token.txt"),
			"torquemada_token":   filepath.Join(DataDir, "torquemada_token.txt"),
			"dark_pushkin_token": filepath.Join(DataDir, "dark_pushkin_token.txt"),
			"tron_grid_key":      filepath.Join(DataDir, "tron_grid_key.txt"),
		}
		tokenData := make(map[string]string)
		for key, path := range tokenFiles {
			if data, err := os.ReadFile(path); err == nil {
				tokenData[key] = strings.TrimSpace(string(data))
			}
		}
		if len(tokenData) > 0 {
			data, _ := json.Marshal(tokenData)
			if err := GlobalContainer.Store(EntryTokensJSON, data); err != nil {
				slog.Error("container import tokens", "error", err)
			} else {
				imported["tokens.json"] = "telegram tokens + API keys"
			}
		}

		writeJSON(w, map[string]any{"ok": true, "imported": imported})
	}
}

// ContainerRestoreHandler opens the container with the given seed phrase,
// extracts config entries back to disk, and triggers a restart to reload.
func ContainerRestoreHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		var req struct {
			SeedPhrase string `json:"seed_phrase"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
			return
		}
		if req.SeedPhrase == "" {
			writeJSON(w, map[string]any{"ok": false, "error": "seed_phrase required"})
			return
		}

		if GlobalContainer == nil {
			writeJSON(w, map[string]any{"ok": false, "error": "container not initialized"})
			return
		}
		if !GlobalContainer.HasContainer() {
			writeJSON(w, map[string]any{"ok": false, "error": "container file not found — nothing to restore"})
			return
		}

		// Try to open with provided seed
		if GlobalContainer.IsOpen() {
			GlobalContainer.Close()
		}
		if err := GlobalContainer.Open(req.SeedPhrase); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "wrong seed phrase: " + err.Error()})
			return
		}

		restored := []string{}

		// 1. Restore main config
		if data, err := GlobalContainer.Load(EntryConfigJSON); err == nil {
			cfgPath := filepath.Join(DataDir, "ParanoidX.json")
			if err := os.WriteFile(cfgPath, data, 0600); err == nil {
				restored = append(restored, "config.json -> simplex-node.json")
			}
		}

		// 2. Restore address files
		if data, err := GlobalContainer.Load(EntryAddressesJSON); err == nil {
			var addrData map[string]string
			if json.Unmarshal(data, &addrData) == nil {
				reverseMap := map[string]string{
					"smp_address":    "smp_client_address.txt",
					"xftp_address":   "xftp_client_address.txt",
					"ice_onion":      "ice_onion.txt",
					"auditor_onion":  "auditor_onion.txt",
					"contact_link":   "island_contact_link.txt",
					"dashboard_onion": "dashboard_onion.txt",
				}
				for key, file := range reverseMap {
					if val, ok := addrData[key]; ok {
						os.WriteFile(filepath.Join(DataDir, file), []byte(val), 0644)
						restored = append(restored, file)
					}
				}
			}
		}

		// 3. Restore tokens
		if data, err := GlobalContainer.Load(EntryTokensJSON); err == nil {
			var tokenData map[string]string
			if json.Unmarshal(data, &tokenData) == nil {
				tokenFiles := map[string]string{
					"ask_steward_token":  "ask_steward_token.txt",
					"torquemada_token":   "torquemada_token.txt",
					"dark_pushkin_token": "dark_pushkin_token.txt",
					"tron_grid_key":      "tron_grid_key.txt",
				}
				for key, file := range tokenFiles {
					if val, ok := tokenData[key]; ok {
						os.WriteFile(filepath.Join(DataDir, file), []byte(val), 0600)
						restored = append(restored, file)
					}
				}
			}
		}

		// Close container after restore
		GlobalContainer.Close()

		writeJSON(w, map[string]any{
			"ok":       true,
			"restored": restored,
			"message":  "Config restored from container. Restarting node to apply changes.",
		})
	}
}

// PanicHandler performs emergency wipe:
//   - Stops Docker containers (SMP, XFTP, coturn, tor)
//   - Disconnects bridge contacts
//   - Clears all chat history
//   - Wipes and deletes the crypto-container
//   - Zeroes in-memory keys
//   - Resets all in-memory state
func PanicHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}

		slog.Warn("PANIC triggered — emergency wipe in progress")

		// 1. Stop Docker containers (SMP, XFTP, coturn, tor)
		if DockerComposeDir != "" {
			for _, svc := range []string{"smp-server", "xftp-server", "coturn", "tor"} {
				cmd := exec.Command("docker", "compose", "-f",
					filepath.Join(DockerComposeDir, "docker-compose.yml"),
					"stop", svc)
				cmd.Dir = DockerComposeDir
				if out, err := cmd.CombinedOutput(); err != nil {
					slog.Warn("panic: docker stop", "service", svc, "error", err, "output", string(out))
				} else {
					slog.Info("panic: docker container stopped", "service", svc)
				}
			}
		}

		// 2. Disconnect SimpleX bridge contacts
		if SimplexCmd != nil {
			SimplexCmd("/_contacts 1")
		}

		// 3. Clear all chat history
		GlobalChatHub.ClearMessages()

		// 4. Reset auto-delete config
		ChatAutoDelete = AutoDeleteConfig{}

		// 5. Wipe and destroy the crypto-container
		if GlobalContainer != nil && GlobalContainer.IsOpen() {
			if err := GlobalContainer.Wipe(); err != nil {
				slog.Error("panic: container wipe", "error", err)
			} else {
				slog.Info("panic: container wiped and deleted")
			}
		}

		// 6. Delete all .txt address and token files from data dir
		if DataDir != "" {
			for _, pattern := range []string{"*_token.txt", "*_address.txt", "*_key.txt", "*_onion.txt", "contact_link.txt"} {
				matches, _ := filepath.Glob(filepath.Join(DataDir, pattern))
				for _, m := range matches {
					os.Remove(m)
					slog.Info("panic: removed file", "path", m)
				}
			}
		}

		// 7. Delete known_roles.json (maps Telegram chats to roles)
		os.Remove(filepath.Join(DataDir, "known_roles.json"))

		BridgeConnected = false

		slog.Warn("PANIC complete — node running in safe mode with no secrets")

		writeJSON(w, map[string]any{
			"ok":      true,
			"panic":   true,
			"message": "PANIC: all connections dropped, Docker containers stopped, container wiped, keys zeroed, config files removed. Restart container via /api/container/restore with your seed phrase.",
		})
	}
}

// AutoDeleteConfigHandler manages the chat auto-delete scheduler configuration.
func AutoDeleteConfigHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			writeJSON(w, ChatAutoDelete)
			return
		}
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		var req struct {
			Enabled  bool   `json:"enabled"`
			Period   string `json:"period"`
			Paranoia bool   `json:"paranoid"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
			return
		}
		if req.Enabled {
			d, err := time.ParseDuration(req.Period)
			if err != nil {
				writeJSON(w, map[string]any{"ok": false, "error": "invalid period"})
				return
			}
			if d < time.Minute {
				d = time.Minute
			}
			if d > 24*time.Hour {
				d = 24 * time.Hour
			}
				ChatAutoDelete = AutoDeleteConfig{
				Enabled:   true,
				Period:    d,
				PeriodStr: d.String(),
				Paranoid:  req.Paranoia,
			}
		} else {
			ChatAutoDelete = AutoDeleteConfig{}
		}
		writeJSON(w, map[string]any{"ok": true, "config": ChatAutoDelete})
	}
}

// DeleteExpiredMessages removes messages older than the configured auto-delete period from the hub.
func (h *ChatHub) DeleteExpiredMessages() int {
	cfg := ChatAutoDelete
	if !cfg.Enabled {
		return 0
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	cutoff := time.Now().Add(-cfg.Period)
	filtered := make([]ChatMessage, 0, len(h.messages))
	removed := 0
	for _, m := range h.messages {
		t, err := time.Parse(time.RFC3339, m.Timestamp)
		if err == nil && t.Before(cutoff) {
			removed++
			continue
		}
		filtered = append(filtered, m)
	}
	h.messages = filtered
	return removed
}
