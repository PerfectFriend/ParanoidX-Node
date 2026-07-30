/*
Package main is the main entry point for the simplex-node server — the sovereign
network daemon for Saint Mary Liberty Island ("The Isle").

It orchestrates hundreds of HTTP API endpoints across subsystems:
  - Economy (ledger, silver-backed assets, dividends, buyback, auction, packs)
  - SimpleX chat relay, bridge, channels, invoices, messages, groups, labels
  - Radio streaming, AI-generated content, scheduling, announcements
  - CryptoContainer (AES-256-GCM sealed vault), DC CryptoCloud (P2P torrent-like
    distribution), ParanoidX multi-layer proxy (V2Ray → Tor → SimpleX)
  - Royal treasury, governance, multi-sig, cron, alerts, node discovery
  - AI Steward, arbitration, moderation, personality profiles, memory
  - TRON USDT monitor, vault mining, POS terminal, market, escrow
  - Multi-platform gateway (WhatsApp, Signal, Matrix, Discord webhooks)
  - System health, metrics, diagnostics, disk cleanup, log rotation
  - P2P node registry, tracker, transport, relay, DID verification
  - Backup (USB, remote sync, chat archive), i18n, audit log

All subsystems run in the same process. Background goroutines handle cron jobs
(dividend 24h, backup 24h, chat backup 6h, disk cleanup 6h, docker health 15m,
auto-archive daily at 03:00, etc.). The server supports graceful shutdown on
SIGTERM/SIGINT, persisting chat, invoices, DC cloud state, and other data.
*/
package main

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"ParanoidX/internal/api"
	"ParanoidX/internal/billing"
	"ParanoidX/internal/channels"
	"ParanoidX/internal/config"
	"ParanoidX/internal/economy"
	"ParanoidX/internal/fileutil"
	"ParanoidX/internal/gateway"
	"ParanoidX/internal/health"
	"ParanoidX/internal/lock"
	"ParanoidX/internal/middleware"
	"ParanoidX/internal/paranoidx"
	"ParanoidX/internal/radio"
	"ParanoidX/internal/isle"
	"ParanoidX/internal/radio/acestep"
	"ParanoidX/internal/registry"
	"ParanoidX/internal/tracker"
	"ParanoidX/internal/transport"
	"ParanoidX/internal/royal"
	"ParanoidX/internal/ai"
	"ParanoidX/internal/bot"
	"ParanoidX/internal/bridge"
	"ParanoidX/internal/container"
	"ParanoidX/internal/dc"
	"ParanoidX/internal/status"
	"ParanoidX/internal/steward"
	"ParanoidX/internal/store"
	"ParanoidX/internal/treasury"
	"ParanoidX/internal/i18n"
	"ParanoidX/internal/vault"
	"ParanoidX/internal/webrtc"
)

const APIVersion = "v1"

var (
	buildVersion = "C41-C60"
	startTime    = time.Now()
	//go:embed app.html
	appHTML string

	lockSvc   *lock.Service
	vaultSvc  *vault.Service
	billSvc   *billing.Service

	// WebRTC signal state for peer-to-peer calls
	sigState = webrtc.NewSignalState()

	// unlockLimiter: rate-limit unlock attempts (1 req/min, burst 5)
	unlockLimiter = middleware.NewRateLimiter(1, 5, time.Minute)

	packMgr    = economy.NewPackManager()
	buybackMgr = economy.NewBuybackManager()
	auctionMgr *economy.AuctionManager

	rolesMu    sync.Mutex
	knownRoles = map[string]string{}
	rolesFile  string
)

func hashPin(code string) string {
	h := sha256.Sum256([]byte(code))
	return fmt.Sprintf("%x", h)
}

func envInt(key string) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return 0, fmt.Errorf("not set")
	}
	n := 0
	_, err := fmt.Sscanf(v, "%d", &n)
	return n, err
}

func readTrim(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func isRoyalNode(dataDir string) bool {
	p := filepath.Join(dataDir, "royal.enabled")
	if _, err := os.Stat(p); err != nil {
		return false
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return false
	}
	content := strings.TrimSpace(string(b))
	if content == "" || content == "0" {
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("writeJSON", "error", err)
	}
}

func loadRoles() {
	rolesMu.Lock()
	defer rolesMu.Unlock()
	if b, err := os.ReadFile(rolesFile); err == nil {
		json.Unmarshal(b, &knownRoles)
	}
}

func sendToIslandRole(role, text string) {
	rolesMu.Lock()
	chat, ok := knownRoles[role]
	rolesMu.Unlock()
	if !ok || chat == "" {
		return
	}
	msg := fmt.Sprintf(`{"cmd": "send", "chat": "%s", "text": "%s"}`, chat, strings.ReplaceAll(text, `"`, `\"`))
	resp, err := http.Post("http://127.0.0.1:5002/send", "application/json", strings.NewReader(msg))
	if err != nil {
		slog.Error("sendToIslandRole", "error", err)
		return
	}
	resp.Body.Close()
}

func getVaultPath(dataDir string) string {
	return vaultSvc.Path
}

func getVaultSizeMB(dataDir string) float64 {
	return vaultSvc.SizeMB()
}

func getVaultFileCount(dataDir string) int {
	return vaultSvc.FileCount()
}

func getVaultFiles(dataDir string) []map[string]any {
	files := vaultSvc.List()
	result := make([]map[string]any, len(files))
	for i, f := range files {
		result[i] = map[string]any{
			"name":  f.Name,
			"size":  f.Size,
			"mtime": f.MTime,
		}
	}
	return result
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	listen := flag.String("listen", "0.0.0.0:8080", "listen address")
	dataDir := flag.String("data", filepath.Join(os.Getenv("HOME"), ".local/share/simplex-node"), "data dir")
	cfgPath := flag.String("config", "", "path to config file (overrides -listen and -data)")
	flag.Parse()

	// Load optional config file
	cfg := config.DefaultConfig()
	if *cfgPath == "" {
		defaultCfgPath := filepath.Join(*dataDir, "simplex-node.json")
		if _, err := os.Stat(defaultCfgPath); err == nil {
			*cfgPath = defaultCfgPath
		}
	}
	if *cfgPath != "" {
		cfg = config.Load(*cfgPath)
		if cfg.Listen != "0.0.0.0:8080" || *listen == "0.0.0.0:8080" {
			*listen = cfg.Listen
		}
		if cfg.DataDir != "" {
			*dataDir = cfg.DataDir
		}
	}

	if err := os.MkdirAll(*dataDir, 0700); err != nil {
		slog.Error("data dir", "error", err)
		os.Exit(1)
	}

	lockSvc = lock.New(*dataDir)
	vaultSvc = vault.New(*dataDir)
	billSvc = billing.New(*dataDir)

	ollamaURL := cfg.OllamaURL
	if ollamaURL == "" {
		ollamaURL = "http://localhost:11434"
	}
	aiClient := ai.NewClient(ollamaURL, cfg.OllamaModel)
	aiProfiles := ai.NewProfileManager(*dataDir)
	aiSteward := ai.NewSteward(aiClient)
	aiSteward.ProfileManager = aiProfiles

	// Initialize Steward AI memory
	aiMemory := ai.NewMemoryStore(*dataDir, 20)
	aiSteward.SetMemoryStore(aiMemory)
	slog.Info("steward memory store initialized")

	go func() {
		slog.Info("ai warm-up starting — pre-loading model")
		if _, err := aiSteward.Ask("Say OK in 1 word.", ""); err != nil {
			slog.Warn("ai warm-up failed", "error", err)
		} else {
			slog.Info("ai warm-up complete — model cached")
		}
	}()

	if token := cfg.AskStewardToken; token != "" {
		askBot := bot.New(token, aiSteward)
		go func() {
			if err := askBot.Run(context.Background()); err != nil {
				slog.Error("asksteward bot exited", "error", err)
			}
		}()
		slog.Info("asksteward bot started")
	} else {
		slog.Info("asksteward bot not started — no token configured")
	}

	if token := cfg.DarkPushkinToken; token != "" {
		dpBot := bot.NewDarkPushkinBot(token, aiSteward)
		go func() {
			if err := dpBot.Run(context.Background()); err != nil {
				slog.Error("darkpushkin bot exited", "error", err)
			}
		}()
		slog.Info("darkpushkin bot started")
	} else {
		slog.Info("darkpushkin bot not started — no token configured")
	}

	if token := cfg.TorquemadaToken; token != "" {
		tqBot := bot.NewTorquemadaBot(token, aiSteward)
		go func() {
			if err := tqBot.Run(context.Background()); err != nil {
				slog.Error("torquemada bot exited", "error", err)
			}
		}()
		slog.Info("torquemada bot started")
	} else {
		slog.Info("torquemada bot not started — no token configured")
	}

	// ===== Multi-Platform Gateway =====
	gwRouter := gateway.NewRouter()
	gwRouter.Handle("/help", func(msg gateway.Message) (*gateway.OutMessage, error) {
		return &gateway.OutMessage{Text: "Available commands:\n/help — This message\n/economy — Economy status\n/wallet — Check balance\n/vault — File storage\n/market — Marketplace\n/p2p — Peer-to-peer\n/radio — Radio stations"}, nil
	})
	gwRouter.Handle("/start", func(msg gateway.Message) (*gateway.OutMessage, error) {
		return &gateway.OutMessage{Text: "Welcome to Saint Mary Liberty Island!"}, nil
	})
	gwRouter.Fallback(func(msg gateway.Message) (*gateway.OutMessage, error) {
		return &gateway.OutMessage{Text: "Unknown command. Try /help."}, nil
	})

	if cfg.WhatsAppAPIToken != "" && cfg.WhatsAppPhoneID != "" {
		wa := gateway.NewWhatsAppAdapter("", cfg.WhatsAppPhoneID, cfg.WhatsAppAPIToken)
		http.HandleFunc("/api/webhook/whatsapp", func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "GET" {
				challenge, ok := wa.WebhookVerify(r)
				if ok {
					w.Write([]byte(challenge))
					return
				}
				http.Error(w, "verification failed", 403)
				return
			}
			if r.Method == "POST" {
				msgs, err := wa.WebhookHandler(r)
				if err != nil {
					slog.Error("whatsapp webhook", "error", err)
					http.Error(w, "bad request", 400)
					return
				}
				for _, msg := range msgs {
					if out, err := gwRouter.Route(msg); err == nil && out != nil {
						wa.Send(msg.ChatID, *out)
					}
				}
				w.WriteHeader(200)
				return
			}
			http.Error(w, "method not allowed", 405)
		})
		slog.Info("whatsapp webhook handler registered at /api/webhook/whatsapp")
	} else {
		slog.Info("whatsapp adapter not started — no api token configured")
	}

	// Register other platform webhook endpoints
	http.HandleFunc("/api/webhook/signal", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			body, _ := io.ReadAll(r.Body)
			slog.Info("webhook signal", "size", len(body), "remote", r.RemoteAddr)
			writeJSON(w, map[string]any{"ok": true, "platform": "signal"})
			return
		}
		if r.Method == "GET" {
			writeJSON(w, map[string]any{"ok": true, "platform": "signal", "status": "active"})
			return
		}
		http.Error(w, "method not allowed", 405)
	})
	http.HandleFunc("/api/webhook/matrix", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			body, _ := io.ReadAll(r.Body)
			slog.Info("webhook matrix", "size", len(body), "remote", r.RemoteAddr)
			writeJSON(w, map[string]any{"ok": true, "platform": "matrix"})
			return
		}
		if r.Method == "GET" {
			writeJSON(w, map[string]any{"ok": true, "platform": "matrix", "status": "active"})
			return
		}
		http.Error(w, "method not allowed", 405)
	})
	http.HandleFunc("/api/webhook/discord", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			body, _ := io.ReadAll(r.Body)
			slog.Info("webhook discord", "size", len(body), "remote", r.RemoteAddr)
			writeJSON(w, map[string]any{"ok": true, "platform": "discord"})
			return
		}
		if r.Method == "GET" {
			writeJSON(w, map[string]any{"ok": true, "platform": "discord", "status": "active"})
			return
		}
		http.Error(w, "method not allowed", 405)
	})
	slog.Info("gateway webhooks registered: /api/webhook/{whatsapp,signal,matrix,discord}")

	// ===== Auto-payout cron for vault mining (every hour) =====
	tqNotifier := bot.NewNotifySender(cfg.TorquemadaToken, cfg.TorquemadaChatID)
	go func() {
		vm := economy.LoadVaultMining(*dataDir)
		for {
			time.Sleep(1 * time.Hour)
			amt, err := vm.ProcessDeferredPayouts(*dataDir)
			if err != nil {
				slog.Error("auto-payout cron", "error", err)
				continue
			}
			if amt > 0 {
				vm.Save(*dataDir)
				slog.Info("auto-payout completed", "amount_ng", amt, "pool", vm.DeferredPoolNg)
				tqNotifier.Send(fmt.Sprintf("⛏️ Mining payout: %d ng distributed\nPool remaining: %d ng", amt, vm.DeferredPoolNg))
			}
		}
	}()
	slog.Info("mining auto-payout cron started (interval: 1h)")

	// ===== POS auto-expiry cleanup (every 15 min) =====
	go func() {
		for {
			time.Sleep(15 * time.Minute)
			pm := economy.LoadPOSManager(*dataDir)
			expired := pm.CleanExpired()
			if expired > 0 {
				pm.Save(*dataDir)
				slog.Info("pos cleanup", "expired", expired)
			}
		}
	}()
	slog.Info("POS auto-expiry cron started (interval: 15min)")

	// ===== Disk alert check (every 30 min) =====
	go func() {
		for {
			time.Sleep(30 * time.Minute)
		disk := status.CheckDiskAndAlert()
		for k, v := range disk {
			if m, ok := v.(map[string]any); ok {
				if pct, ok := m["used_pct"].(string); ok {
					pctNum := 0.0
					fmt.Sscanf(pct, "%f%%", &pctNum)
					if pctNum > 90.0 {
						tqNotifier.Send(fmt.Sprintf("⚠️ Disk warning: %s %s used (%.0f%%) — clean up!", k, pct, pctNum))
					}
				}
			}
		}
		}
	}()
	slog.Info("disk alert cron started (interval: 30min)")

	// ===== TRON USDT Monitor (every 60s) =====
	tronMon := treasury.New(*dataDir, cfg.TronTreasuryAddr, cfg.TronGridAPIKey)
	if cfg.TronTreasuryAddr != "" {
		go tronMon.Start()
		go tronMon.StartAutoRound(*dataDir, vaultSvc.Path)
		slog.Info("tron monitor + auto-round started", "addr", cfg.TronTreasuryAddr, "interval", "60s")
	} else {
		slog.Info("tron monitor not started — no treasury address configured")
	}

	// Bandwidth tracker init
	api.InitBandwidthTracker()
	slog.Info("bandwidth tracker started (sample interval: 60s)")

	// Memory tracker init
	api.InitMemoryTracker()
	slog.Info("memory tracker started (sample interval: 60s)")

	// P2P Marketplace init
	api.InitMarketplace(*dataDir)
	slog.Info("marketplace initialized")

	// i18n Multi-language init
	i18n.Init(*dataDir)
	slog.Info("i18n initialized", "languages", i18n.Global.Languages())

	// DID key manager init (Ed25519 real keys)
	api.InitDIDKeys(*dataDir)
	slog.Info("DID keys initialized")

	// DAO Governance init
	api.InitDAO(*dataDir)

	// ===== Dividend Distribution Cron (every 24h) =====
	go func() {
		dd := economy.NewDividendDistributor()
		for {
			time.Sleep(24 * time.Hour)
			ledger := economy.LoadLedger(*dataDir)
			pool := ledger.Balance("dividend_pool")
			if pool > 0 {
				round, err := dd.Distribute(*dataDir, pool, "daily-"+time.Now().Format("20060102"))
				if err != nil {
					slog.Error("dividend cron", "error", err)
				} else {
					// Distribute already mints directly to holders; clear pool
					ledger.Transfer("dividend_pool", "treasury", pool)
					ledger.Save(*dataDir)
					slog.Info("dividend cron", "round", round.RoundID, "total", pool, "holders", len(round.Payments))
				}
			}
		}
	}()
	slog.Info("dividend distribution cron started (interval: 24h)")

	// Full system backup to USB (daily)
	go func() {
		for {
			time.Sleep(24 * time.Hour)
			home := os.Getenv("HOME")
			backupScript := filepath.Join(home, "ParanoidX/scripts/backup-to-usb.sh")
			if _, err := os.Stat(backupScript); err == nil {
				exec.Command("bash", backupScript).Run()
				slog.Info("auto-backup to USB triggered")
			}
			// Clean backups older than 30 days on USB
			usbDir := "/run/media/tomas/SIMPLEX-USB"
			entries, err := os.ReadDir(usbDir)
			if err == nil {
				cutoff := time.Now().Add(-30 * 24 * time.Hour)
				for _, e := range entries {
					if strings.HasSuffix(e.Name(), ".tar") || strings.HasSuffix(e.Name(), ".gitbundle") {
						info, err := e.Info()
						if err == nil && info.ModTime().Before(cutoff) {
							os.Remove(filepath.Join(usbDir, e.Name()))
							slog.Info("removed old backup", "file", e.Name())
						}
					}
				}
			}
		}
	}()
	slog.Info("auto-backup to USB cron started (interval: 24h)")

	// Chat auto-backup (every 6h)
	backupDir := filepath.Join(*dataDir, "backups")
	os.MkdirAll(backupDir, 0755)
	go func() {
		for {
			time.Sleep(6 * time.Hour)
			src := filepath.Join(*dataDir, "chat_history.json")
			inv := filepath.Join(*dataDir, "invoices.json")
			ts := time.Now().Format("20060102-150405")
			for _, pair := range [][2]string{{src, "chat_history"}, {inv, "invoices"}} {
				data, err := os.ReadFile(pair[0])
				if err != nil {
					continue
				}
				os.WriteFile(filepath.Join(backupDir, pair[1]+"-"+ts+".json"), data, 0644)
			}
			// Keep only latest 10 backups
			entries, _ := os.ReadDir(backupDir)
			var backups []string
			for _, e := range entries {
				if !e.IsDir() {
					backups = append(backups, e.Name())
				}
			}
			if len(backups) > 10 {
				sort.Strings(backups)
				for _, f := range backups[:len(backups)-10] {
					os.Remove(filepath.Join(backupDir, f))
				}
			}
			slog.Info("chat auto-backup complete", "dir", backupDir)
		}
	}()
	slog.Info("chat auto-backup cron started (interval: 6h)")

	// Log rotation (every 24h — keep 3 most recent log files)
	logDir := filepath.Join(*dataDir, "logs")
	go func() {
		for {
			time.Sleep(24 * time.Hour)
			entries, err := os.ReadDir(logDir)
			if err != nil {
				continue
			}
			var logs []string
			for _, e := range entries {
				if !e.IsDir() && (strings.HasSuffix(e.Name(), ".log") || strings.HasSuffix(e.Name(), ".json")) {
					logs = append(logs, e.Name())
				}
			}
			if len(logs) > 3 {
				sort.Strings(logs)
				for _, f := range logs[:len(logs)-3] {
					os.Remove(filepath.Join(logDir, f))
					slog.Info("rotated old log", "file", f)
				}
			}
		}
	}()
	slog.Info("log rotation cron started (interval: 24h, keep: 3)")

	// Docker health check cron — restart unhealthy containers (every 15 min)
	dockerComposeDir := filepath.Join(os.Getenv("HOME"), "ParanoidX", "docker")
	go func() {
		for {
			time.Sleep(15 * time.Minute)
			cmd := exec.Command("docker", "compose", "ps", "--format", "{{.Name}}\t{{.Status}}")
			cmd.Dir = dockerComposeDir
			out, err := cmd.Output()
			if err != nil {
				continue
			}
			for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
				if strings.Contains(line, "(unhealthy)") || strings.Contains(line, "Exit") {
					parts := strings.SplitN(line, "\t", 2)
					if len(parts) > 0 {
						slog.Warn("restarting unhealthy container", "name", parts[0])
						exec.Command("docker", "restart", parts[0]).Run()
					}
				}
			}
		}
	}()
	slog.Info("docker health cron started (interval: 15min)")

	// Weekly docker image pull to refresh containers
	go func() {
		for {
			time.Sleep(7 * 24 * time.Hour)
			slog.Info("weekly docker image pull starting…")
			cmd := exec.Command("docker", "compose", "pull")
			cmd.Dir = dockerComposeDir
			if out, err := cmd.CombinedOutput(); err != nil {
				slog.Warn("docker pull failed", "error", err.Error(), "output", string(out))
			} else {
				slog.Info("docker images pulled, restarting stack")
				exec.Command("docker", "compose", "up", "-d").Run()
			}
		}
	}()
	slog.Info("weekly docker pull cron started (interval: 7d)")

	// Auto-disk-cleanup cron (every 6h, triggers when disk > 85%)
	go func() {
		for {
			time.Sleep(6 * time.Hour)
			if api.IsMaintenanceMode() {
				slog.Info("auto-cleanup: skipped (maintenance mode)")
				continue
			}
			// Check disk usage with df
			dfOut, err := exec.Command("df", "-h", "/").Output()
			if err != nil {
				continue
			}
			lines := strings.Split(strings.TrimSpace(string(dfOut)), "\n")
			trigger := false
			if len(lines) >= 2 {
				fields := strings.Fields(lines[1])
				if len(fields) >= 5 {
					pctStr := strings.TrimSuffix(fields[4], "%")
					if p, err := strconv.Atoi(pctStr); err == nil && p > 85 {
						trigger = true
					}
				}
			}
			if !trigger {
				continue
			}
			slog.Warn("auto-cleanup: disk >85%, starting cleanup")
		home := os.Getenv("HOME")
		// Docker prune
		exec.Command("docker", "system", "prune", "-f", "--filter", "until=24h").Run()
		exec.Command("docker", "builder", "prune", "-f", "--filter", "until=24h").Run()
		exec.Command("docker", "volume", "prune", "-f").Run()
		// Old logs (keep 3)
		logDir := filepath.Join(*dataDir, "logs")
		if entries, err := os.ReadDir(logDir); err == nil {
			var logs []string
			for _, e := range entries {
				if !e.IsDir() && (strings.HasSuffix(e.Name(), ".log") || strings.HasSuffix(e.Name(), ".json")) {
					logs = append(logs, e.Name())
				}
			}
			if len(logs) > 3 {
				sort.Strings(logs)
				for _, f := range logs[:len(logs)-3] {
					os.Remove(filepath.Join(logDir, f))
				}
			}
		}
		// Old A1-backups (keep 5)
		a1Dir := filepath.Join(home, "A1-backups")
		if entries, err := os.ReadDir(a1Dir); err == nil {
			type backup struct {
				name string
				mod  time.Time
			}
			var backups []backup
			for _, e := range entries {
				if info, err := e.Info(); err == nil {
					backups = append(backups, backup{name: e.Name(), mod: info.ModTime()})
				}
			}
			if len(backups) > 5 {
				sort.Slice(backups, func(i, j int) bool { return backups[i].mod.After(backups[j].mod) })
				for _, b := range backups[5:] {
					os.RemoveAll(filepath.Join(a1Dir, b.name))
				}
			}
		}
		// APT cache
		exec.Command("apt-get", "clean").Run()
		// Journald logs (keep 200MB)
		exec.Command("journalctl", "--vacuum-size=200M").Run()
		// Snap cache prune
		exec.Command("snap", "set", "system", "snap.retain=2").Run()
		// Go cache
		os.RemoveAll(filepath.Join(home, ".cache", "go-build"))
		// Pip cache
		os.RemoveAll(filepath.Join(home, ".cache", "pip"))
		slog.Info("auto-cleanup complete")
		}
	}()
	slog.Info("auto-disk-cleanup cron started (interval: 6h, threshold: 85%)")

	// Silver spot oracle with live polling
	silverOracle := economy.LoadOracle(*dataDir)
	api.GlobalOracleRef = silverOracle
	go silverOracle.StartLivePolling(*dataDir, 5*time.Minute)
	slog.Info("silver spot oracle live polling started", "interval", "5m")

	// Steward AI service
	stewardSvc := steward.NewService(*dataDir, aiSteward)
	stewardSvc.LoadState()
	go stewardSvc.Start()

	rolesFile = filepath.Join(*dataDir, "known_roles.json")
	loadRoles()

	staticHandler := http.StripPrefix("/static/", http.FileServer(http.Dir(*dataDir)))
	http.Handle("/static/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		staticHandler.ServeHTTP(w, r)
	}))

 	// ===== Chat file uploads =====
 	filesDir := filepath.Join(*dataDir, "files")
 	if err := os.MkdirAll(filesDir, 0700); err == nil {
 		http.Handle("/files/", http.StripPrefix("/files/", http.FileServer(http.Dir(filesDir))))
 	}

 	// ===== POS Pay Page =====
 	http.HandleFunc("/pos/pay", func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		id := r.URL.Query().Get("id")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<!DOCTYPE html>
<html lang="ru"><head><meta charset="UTF-8"><title>Pay Invoice</title>
<style>body{font-family:sans-serif;max-width:500px;margin:40px auto;padding:0 20px;text-align:center}
h1{color:#1a1a2e}.card{background:#f5f5f5;padding:30px;border-radius:12px;margin:20px 0}
.amount{font-size:48px;font-weight:bold;color:#1a1a2e;margin:20px 0}
.ng{font-size:16px;color:#666}.desc{color:#555;margin:20px 0}.status{color:#ff9800;font-weight:bold}
input{width:100%;padding:12px;margin:10px 0;border:2px solid #ddd;border-radius:8px;font-size:16px}
button{background:#1a1a2e;color:#fff;padding:14px 40px;border:none;border-radius:8px;font-size:18px;cursor:pointer}
button:hover{background:#16213e}.success{color:#4caf50;font-weight:bold}.error{color:#f44336}
</style></head><body>
<h1>🏪 Pay Invoice</h1>
<div id="invoice" class="card"><p class="status">Loading...</p></div>
<script>
const id = '` + id + `';
async function load(){const r=await fetch('/api/pos?action=invoice&id='+id);const inv=await r.json();
if(inv.status==='paid'){document.getElementById('invoice').innerHTML='<h2 class=success>✅ Paid</h2><p>Invoice '+id.slice(0,8)+'...</p>';return}
if(inv.status==='expired'){document.getElementById('invoice').innerHTML='<h2 class=error>⌛ Expired</h2>';return}
document.getElementById('invoice').innerHTML=
'<div class=amount>'+(inv.amount_ng/1e9).toFixed(2)+' <span class=ng>B ng</span></div>'+
'<p class=desc>'+(inv.description||'Payment')+'</p>'+
'<p>Merchant: '+(inv.merchant||'').slice(0,16)+'...</p>'+
'<p>Expires: '+new Date(inv.expires_at).toLocaleString()+'</p>'+
'<input id=payer placeholder="Your pubkey"><br>'+
'<button onclick=pay()>💳 Pay</button>';}
async function pay(){const p=document.getElementById('payer').value;if(!p)return alert('Enter pubkey');
const r=await fetch('/api/pos?action=pay',{method:'POST',
headers:{'Content-Type':'application/json'},
body:JSON.stringify({invoice_id:id,payer:p})});
const result=await r.json();
if(result.status==='paid'){document.getElementById('invoice').innerHTML='<h2 class=success>✅ Paid!</h2><p>Ref: '+result.payment_ref+'</p>';}
else{document.getElementById('invoice').innerHTML='<h2 class=error>❌ Error</h2><p>'+(result.error||'Payment failed')+'</p>';}}
load();
</script></body></html>`))
	})

	// ===== Dashboard SPA =====
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		dashPath := filepath.Join(*dataDir, "dashboard.html")
		if b, err := os.ReadFile(dashPath); err == nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(b)
		} else {
			http.Error(w, "dashboard not found", 404)
		}
	})

	// ===== ARGENTUM Mini App =====
	http.HandleFunc("/app/", func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(appHTML))
	})

	// ===== Documentation Browser =====
	http.HandleFunc("/docs", func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<!DOCTYPE html>
<html lang="en"><head><meta charset="UTF-8"><title>Documentation — The Isle</title>
<style>
body{font-family:system-ui,sans-serif;max-width:900px;margin:40px auto;padding:0 20px;background:#0a0a0c;color:#e5e7eb}
h1{color:#b8860b;border-bottom:1px solid #222;padding-bottom:10px}
.docs-table{width:100%;border-collapse:collapse;margin:20px 0}
th,td{padding:12px 10px;text-align:left;border-bottom:1px solid #222}
th{background:#141418;color:#b8860b;font-size:0.85rem;text-transform:uppercase;letter-spacing:0.5px}
tr:hover{background:#141418}
a{color:#3b82f6;text-decoration:none}
a:hover{text-decoration:underline}
.badge{display:inline-block;padding:2px 8px;border-radius:999px;font-size:0.75rem;font-weight:600}
.badge-en{background:#1e3a5f;color:#93c5fd}
.badge-ru{background:#5f1e1e;color:#fca5a5}
.badge-es{background:#1e5f1e;color:#86efac}
.paper-icon{font-size:1.2rem}
.loading{text-align:center;padding:40px;color:#666}
input{width:100%;padding:10px;margin:10px 0;background:#141418;border:1px solid #333;border-radius:8px;color:#fff;font-size:0.95rem}
</style></head><body>
<h1>📄 Project Documentation</h1>
<input id="search" placeholder="Search documents..." oninput="filter()">
<table class="docs-table"><thead><tr><th>Document</th><th>Language</th><th>Size</th><th>Modified</th><th>Actions</th></tr></thead><tbody id="docList"></tbody></table>
<script>
async function load(){const r=await(await fetch('/api/docs/list')).json();window.docs=r.entries||[];render(window.docs);}
function render(list){const t=document.getElementById('docList');
let h='';for(const d of list){
const lang=d.lang?d.lang:'—';
const langClass='badge-'+d.lang;
const icon=d.name.includes('WHITE-PAPER')?'📜':'📄';
const size=(d.size/1024).toFixed(1)+' KB';
h+='<tr><td>'+icon+' '+d.name.replace(/-/g,' ').replace('.md','').replace(/_/g,' ')+'</td>'+
'<td>'+(d.lang?'<span class="badge '+langClass+'">'+lang.toUpperCase()+'</span>':'—')+'</td>'+
'<td>'+size+'</td><td>'+d.modified+'</td>'+
'<td><a href="'+d.path+'" download>⬇️ Download</a> | <a href="/api/docs/view?name='+encodeURIComponent(d.name)+(d.path.includes('root=1')?'&root=1':'')+'" target="_blank">👁️ View</a></td></tr>';}
t.innerHTML=h;}
function filter(){const q=document.getElementById('search').value.toLowerCase();
if(!window.docs)return;
const filtered=window.docs.filter(d=>d.name.toLowerCase().includes(q));
render(filtered);}
load();
</script></body></html>`))
	})

	// ===== Radio Dashboard =====
	http.HandleFunc("/radio", func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<!DOCTYPE html>
<html lang="ru"><head><meta charset="UTF-8"><title>Radio — The Isle</title>
<style>
body{font-family:system-ui,sans-serif;max-width:800px;margin:40px auto;padding:0 20px;background:#0a0a0c;color:#e5e7eb}
h1{color:#b8860b;border-bottom:1px solid #222;padding-bottom:10px}
h2{color:#b8860b;margin-top:30px}
.station{background:#141418;border:1px solid #333;border-radius:12px;padding:16px;margin:12px 0;cursor:pointer;transition:all .2s}
.station:hover{border-color:#b8860b;transform:translateX(4px)}
.station.active{border-color:#22c55e;background:#14281a}
.station .title{font-size:1.1rem;font-weight:600;color:#fff}
.station .meta{font-size:0.85rem;color:#888;margin-top:4px}
.station .desc{font-size:0.9rem;color:#aaa;margin-top:6px}
.station .flag{font-size:1.4rem}
.track{background:#141418;border:1px solid #333;border-radius:8px;padding:10px 14px;margin:6px 0}
.track.announce{border-left:3px solid #b8860b}
.track.ad{border-left:3px solid #ef4444}
.track .title{font-size:0.95rem}
.track .meta{font-size:0.8rem;color:#888}
.now-playing{background:#14281a;border:1px solid #22c55e;border-radius:12px;padding:20px;margin:16px 0;text-align:center}
.now-playing .label{font-size:0.85rem;color:#22c55e;text-transform:uppercase;letter-spacing:1px}
.now-playing .track-title{font-size:1.2rem;color:#fff;margin:8px 0}
.back{color:#b8860b;cursor:pointer;margin:10px 0;display:inline-block}
.back:hover{text-decoration:underline}
.loading{text-align:center;padding:40px;color:#666}
.error{color:#ef4444;padding:20px;text-align:center}
button{background:#b8860b;color:#000;border:none;padding:8px 20px;border-radius:8px;cursor:pointer;font-weight:600}
button:hover{background:#d4a017}
select{background:#141418;color:#fff;border:1px solid #333;border-radius:8px;padding:8px;margin:4px}
input,textarea{background:#141418;color:#fff;border:1px solid #333;border-radius:8px;padding:8px;margin:4px;width:100%;box-sizing:border-box}
textarea{min-height:80px}
</style></head><body>
<h1>📻 Radio Liberty</h1>
<div id="app"><div class="loading">Загрузка станций...</div></div>
<script>
let stations=[], selected=null;
const typeIcons={music:'🎵',news:'📰',talk:'🎙️',mixed:'📻'};
const flags={en:'🇬🇧',ru:'🇷🇺',es:'🇪🇸'};
const langLabels={en:'English',ru:'Русский',es:'Español'};
function ti(t){return typeIcons[t]||'📻'}
function fl(l){return flags[l]||'🌐'}
function ll(l){return langLabels[l]||l}
async function loadStations(){
document.getElementById('app').innerHTML='<div class="loading">Загрузка станций...</div>';
try{
const r=await(await fetch('/api/radio?action=stations')).json();
stations=r.stations||[];
renderList();
}catch(e){document.getElementById('app').innerHTML='<div class="error">Ошибка: '+e.message+'</div>';}
}
function renderList(){
let h='';
for(const s of stations){
h+='<div class="station" onclick="selectStation(\''+s.id+'\')">'
+'<div class="title">'+ti(s.type)+' '+fl(s.lang)+' '+s.name+'</div>'
+'<div class="meta">'+ll(s.lang)+' • '+(s.enabled?'🟢 В эфире':'🔴 Офлайн')+'</div>'
+'<div class="desc">'+s.description+'</div></div>';
}
h+='<div style="margin-top:30px"><h2>📢 Сделать объявление</h2>'
+'<select id="announcer"><option value="king">👑 Король</option><option value="torquemada">🔧 Торквемада</option><option value="steward">🤖 Стюард</option></select>'
+'<select id="annLang"><option value="ru">🇷🇺 Русский</option><option value="en">🇬🇧 English</option><option value="es">🇪🇸 Español</option></select>'
+'<input id="annTitle" placeholder="Заголовок"><textarea id="annBody" placeholder="Текст объявления"></textarea>'
+'<button onclick="postAnnounce()">📨 Отправить в эфир</button></div>';
document.getElementById('app').innerHTML=h;
}
async function selectStation(id){
selected=id;
document.getElementById('app').innerHTML='<div class="loading">Загрузка плейлиста...</div>';
try{
const r=await(await fetch('/api/radio?action=playlist&station='+id)).json();
const station=r.station, playlist=r.playlist||[];
let h='<span class="back" onclick="loadStations()">← Все станции</span>';
h+='<div class="now-playing"><div class="label">🟢 В ЭФИРЕ</div>'
+'<div class="track-title">'+ti(station.type)+' '+station.name+'</div>'
+'<div style="color:#888">'+fl(station.lang)+' '+ll(station.lang)+' • '+station.description+'</div></div>';
h+='<h2>Плейлист ('+playlist.length+' треков)</h2>';
for(const t of playlist){
const cls=t.is_ad?'track ad':t.is_announce?'track announce':'track';
const label=t.is_ad?'📢 Реклама':t.is_announce?'📰 Объявление':'🎵 Трек';
h+='<div class="'+cls+'"><div class="title">'+t.title+'</div>'
+'<div class="meta">'+label+' • '+(t.duration||'—')+'с</div></div>';
}
document.getElementById('app').innerHTML=h;
}catch(e){document.getElementById('app').innerHTML='<div class="error">Ошибка: '+e.message+'</div>';}
}
async function postAnnounce(){
const announcer=document.getElementById('announcer').value;
const lang=document.getElementById('annLang').value;
const title=document.getElementById('annTitle').value;
const body=document.getElementById('annBody').value;
if(!title||!body){alert('Заполните заголовок и текст');return;}
try{
const r=await(await fetch('/api/radio?action=announce',{
method:'POST',headers:{'Content-Type':'application/json'},
body:JSON.stringify({announcer,lang,title,body,stations:selected?[selected]:[]})
})).json();
if(r.ok){alert('✅ Объявление отправлено в эфир!');document.getElementById('annTitle').value='';document.getElementById('annBody').value='';}
else alert('❌ Ошибка: '+JSON.stringify(r));
}catch(e){alert('❌ Ошибка: '+e.message);}
}
loadStations();
</script></body></html>`))
	})

	// ===== POS Dashboard =====
	http.HandleFunc("/pos", func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<!DOCTYPE html>
<html lang="ru"><head><meta charset="UTF-8"><title>POS Terminal</title>
<style>body{font-family:sans-serif;max-width:800px;margin:40px auto;padding:0 20px}
h1{color:#1a1a2e;color:#b8860b}table{width:100%;border-collapse:collapse;margin:20px 0}
th,td{padding:10px;text-align:left;border-bottom:1px solid #ddd}
th{background:#1a1a2e;color:#fff}.badge{display:inline-block;padding:3px 8px;border-radius:4px;font-size:12px}
.badge-paid{background:#4caf50;color:#fff}.badge-pending{background:#ff9800;color:#fff}
.badge-expired{background:#f44336;color:#fff}
form{background:#f5f5f5;padding:20px;border-radius:8px;margin:20px 0}
input,textarea,select{width:100%;padding:8px;margin:8px 0;border:1px solid #ddd;border-radius:4px}
button{background:#1a1a2e;color:#fff;padding:10px 20px;border:none;border-radius:4px;cursor:pointer}
button:hover{background:#16213e}
#stats{display:flex;gap:20px;margin:20px 0;flex-wrap:wrap}
.stat-card{flex:1;min-width:140px;background:#f5f5f5;padding:20px;border-radius:8px;text-align:center}
.stat-card h3{margin:0;color:#666;font-size:14px}.stat-card .value{font-size:28px;font-weight:bold;color:#1a1a2e}
.tab{display:inline-block;padding:10px 20px;cursor:pointer;border-radius:8px 8px 0 0;margin-right:4px}
.tab.active{background:#1a1a2e;color:#fff}.tab.inactive{background:#ddd}.tab-content{display:none}
.tab-content.active{display:block}.copy{font-size:11px;color:#666;cursor:pointer;margin-left:6px}
</style></head><body>
<h1>🏪 POS Terminal</h1>
<div id="stats"></div>
<div><span class="tab active" onclick="switchTab('invoices')">Invoices</span><span class="tab inactive" onclick="switchTab('vouchers')">Vouchers</span></div>

<div id="tab-invoices" class="tab-content active">
<form id="createForm"><h3>Create Invoice</h3>
<input id="merchant" placeholder="Merchant pubkey" required>
<input id="amount" type="number" placeholder="Amount ng" required>
<input id="desc" placeholder="Description">
<button type="submit">Create Invoice</button></form>
<h2>Invoices <input id="merchantFilter" placeholder="Filter by merchant pubkey" oninput="load()" style="width:auto;display:inline;margin-left:10px;padding:4px;font-size:13px"></h2>
<table><thead><tr><th>ID</th><th>Merchant</th><th>Amount</th><th>Status</th><th>Expires</th><th>QR</th></tr></thead><tbody id="invoices"></tbody></table>
</div>

<div id="tab-vouchers" class="tab-content">
<form id="voucherForm"><h3>Create Voucher</h3>
<input id="vMerchant" placeholder="Merchant pubkey" required>
<input id="vAmount" type="number" placeholder="Amount ng" required>
<button type="submit">Create Voucher</button></form>
<form id="redeemForm"><h3>Redeem Voucher</h3>
<input id="rCode" placeholder="Voucher code" required>
<input id="rRedeemer" placeholder="Redeemer pubkey" required>
<button type="submit">Redeem</button></form>
<div id="voucherResult"></div>
<h2>Existing Vouchers</h2>
<table><thead><tr><th>Code</th><th>Merchant</th><th>Amount</th><th>Status</th><th>Redeemed By</th></tr></thead><tbody id="vouchers"></tbody></table>
</div>

<script>
function switchTab(name){document.querySelectorAll('.tab').forEach(t=>t.className='tab inactive');document.querySelectorAll('.tab-content').forEach(t=>t.className='tab-content');document.querySelector('.tab[onclick*="'+name+'"]').className='tab active';document.getElementById('tab-'+name).className='tab-content active';}
async function load(){const s=await(await fetch('/api/pos?action=stats')).json();
document.getElementById('stats').innerHTML=
'<div class="stat-card"><h3>Volume</h3><div class="value">'+(s.total_volume_ng/1e9).toFixed(2)+'B</div></div>'+
'<div class="stat-card"><h3>Invoices</h3><div class="value">'+s.total_invoices+'</div></div>'+
'<div class="stat-card"><h3>Paid</h3><div class="value">'+s.paid+'</div></div>'+
'<div class="stat-card"><h3>Commission</h3><div class="value">'+(s.total_commission/1e9).toFixed(2)+'B</div></div>';
const mf=document.getElementById('merchantFilter').value;const q=mf?'&filter='+encodeURIComponent(mf):'';
const r2=await(await fetch('/api/pos?action=list&merchant=all'+q)).json();
let rows='';if(r2.invoices){for(const inv of r2.invoices){
const badge='badge-'+inv.status;const exp=new Date(inv.expires_at).toLocaleString();
rows+='<tr><td title="'+inv.id+'">'+inv.id.slice(0,8)+'...</td><td title="'+inv.merchant+'">'+inv.merchant.slice(0,12)+'...</td><td>'+(inv.amount_ng/1e9)+'B</td><td><span class="badge '+badge+'">'+inv.status+'</span></td><td>'+exp+'</td><td><a href="/api/pos/qr?id='+inv.id+'" target="_blank" title="QR code">🔲</a></td></tr>';}}
document.getElementById('invoices').innerHTML=rows;

const v=await(await fetch('/api/pos?action=list-vouchers')).json();
let vrows='';if(v.vouchers){for(const vv of v.vouchers){
const st=vv.redeemed?'<span class="badge badge-paid">Redeemed</span>':'<span class="badge badge-pending">Active</span>';
const by=vv.redeemed_by?v.redeemed_by.slice(0,12)+'...':'—';
vrows+='<tr><td><code>'+vv.code+'</code><span class="copy" onclick="navigator.clipboard.writeText(\''+vv.code+'\')">📋</span></td><td>'+vv.merchant.slice(0,12)+'...</td><td>'+(vv.amount_ng/1e9)+'B</td><td>'+st+'</td><td>'+by+'</td></tr>';}}
document.getElementById('vouchers').innerHTML=vrows;}
document.getElementById('createForm').onsubmit=async function(e){e.preventDefault();
await fetch('/api/pos?action=create-invoice',{method:'POST',
headers:{'Content-Type':'application/json'},
body:JSON.stringify({merchant:merchant.value,amount_ng:parseInt(amount.value),description:desc.value})});
merchant.value='';amount.value='';desc.value='';load();};
document.getElementById('voucherForm').onsubmit=async function(e){e.preventDefault();
const d=await(await fetch('/api/pos?action=create-voucher',{method:'POST',
headers:{'Content-Type':'application/json'},
body:JSON.stringify({merchant:vMerchant.value,amount_ng:parseInt(vAmount.value)})})).json();
document.getElementById('voucherResult').innerHTML='<div style="background:#e8f5e9;padding:15px;border-radius:8px;margin:10px 0">Voucher created: <code style="font-size:18px">'+d.code+'</code><span class="copy" onclick="navigator.clipboard.writeText(\''+d.code+'\')">📋</span></div>';
vMerchant.value='';vAmount.value='';load();};
document.getElementById('redeemForm').onsubmit=async function(e){e.preventDefault();
const d=await(await fetch('/api/pos?action=redeem-voucher',{method:'POST',
headers:{'Content-Type':'application/json'},
body:JSON.stringify({code:rCode.value,redeemer:rRedeemer.value})})).json();
document.getElementById('voucherResult').innerHTML=d.code?'<div style="background:#e8f5e9;padding:15px;border-radius:8px;margin:10px 0">✅ Redeemed '+d.amount_ng+' ng for '+d.redeemed_by.slice(0,12)+'...</div>':'<div style="background:#ffebee;padding:15px;border-radius:8px;margin:10px 0">❌ '+(d.error||'Redemption failed')+'</div>';
rCode.value='';rRedeemer.value='';load();};
load();setInterval(load,10000);
</script></body></html>`))
	})

	// ===== Status =====
	http.HandleFunc("/api/version", func(w http.ResponseWriter, r *http.Request) {
		uptime := time.Since(startTime).Hours()
		writeJSON(w, map[string]any{
			"version":      buildVersion,
			"api_version":  APIVersion,
			"build":        fmt.Sprintf("px-node-%s", buildVersion),
			"go":           runtime.Version(),
			"started":      startTime.Format(time.RFC3339),
			"uptime_hours": float64(int(uptime*10)) / 10,
		})
	})

	http.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		data := status.Collect(*dataDir, vaultSvc.Path, startTime)
		data["locked"] = lockSvc.IsLocked()
		data["version"] = buildVersion
		data["bridge"] = map[string]any{"connected": api.BridgeConnected}

		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		memTotal := uint64(0)
		if b, err := os.ReadFile("/proc/meminfo"); err == nil {
			for _, line := range strings.Split(string(b), "\n") {
				if strings.HasPrefix(line, "MemTotal:") {
					fields := strings.Fields(line)
					if len(fields) >= 2 {
						memTotal, _ = strconv.ParseUint(fields[1], 10, 64)
					}
					break
				}
			}
		}
		data["memory"] = map[string]any{
			"alloc_mb":  m.Alloc / (1024 * 1024),
			"sys_mb":    m.Sys / (1024 * 1024),
			"total_mb":  memTotal / 1024,
			"used_mb":   (memTotal - getAvailMem()) / 1024,
		}

		if loadAvg, err := os.ReadFile("/proc/loadavg"); err == nil {
			parts := strings.Fields(string(loadAvg))
			if len(parts) >= 3 {
				data["load"] = map[string]any{
					"load1": parts[0],
					"load5": parts[1],
					"load15": parts[2],
				}
			}
		}

		writeJSON(w, data)
	})

	http.HandleFunc("/api/addresses", func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		writeJSON(w, map[string]string{
			"smp":     readTrim(filepath.Join(*dataDir, "smp_client_address.txt")),
			"xftp":    readTrim(filepath.Join(*dataDir, "xftp_client_address.txt")),
			"ice":     readTrim(filepath.Join(*dataDir, "ice_onion.txt")),
			"auditor": readTrim(filepath.Join(*dataDir, "auditor_onion.txt")),
			"contact": readTrim(filepath.Join(*dataDir, "island_contact_link.txt")),
		})
	})

	http.HandleFunc("/api/disk-check", func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		writeJSON(w, status.CheckDiskAndAlert())
	})

	// ===== Lock =====
	http.HandleFunc("/api/lock-status", api.LockStatusHandler(lockSvc))
	http.HandleFunc("/api/lock", api.LockHandler(lockSvc))
	http.HandleFunc("/api/unlock", api.UnlockHandler(lockSvc, unlockLimiter))
	http.HandleFunc("/api/change-lock-code", api.ChangeLockCodeHandler(lockSvc))

	// ===== Rotation =====
	var (
		rotateRunning bool
		rotateMu      sync.Mutex
	)
	http.HandleFunc("/api/rotate", func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		rotateMu.Lock()
		if rotateRunning {
			rotateMu.Unlock()
			http.Error(w, "rotation already in progress", 429)
			return
		}
		rotateRunning = true
		rotateMu.Unlock()
		tmp := filepath.Join(*dataDir, "rotate.sh")
		dockerDir := os.Getenv("SIMPLEX_SRC")
		if dockerDir == "" {
			dockerDir = filepath.Join(os.Getenv("HOME"), "ParanoidX")
		}
		dockerDir = filepath.Join(dockerDir, "docker")
		cdCmd := fmt.Sprintf(`cd "%s" 2>/dev/null || true`, dockerDir)
		script := `#!/bin/sh
set -e
` + cdCmd + `
BACKUP_BASE="/home/tomas/.local/share/simplex-node/tor-keys-backup"
for d in smp xftp; do
  if [ -f "./tor/hidden_services/$d/hs_ed25519_secret_key" ]; then
    mkdir -p "$BACKUP_BASE/$d" 2>/dev/null || true
    cp "./tor/hidden_services/$d/hs_ed25519_secret_key" "$BACKUP_BASE/$d/" 2>/dev/null || true
    cp "./tor/hidden_services/$d/hs_ed25519_public_key" "$BACKUP_BASE/$d/" 2>/dev/null || true
    cp "./tor/hidden_services/$d/hostname" "$BACKUP_BASE/$d/" 2>/dev/null || true
  fi
done
docker compose stop tor smp-server xftp-server 2>/dev/null || true
rm -rf ./tor/hidden_services/smp ./tor/hidden_services/xftp 2>/dev/null || true
mkdir -p ./tor/hidden_services/smp ./tor/hidden_services/xftp
chmod 700 ./tor/hidden_services/smp ./tor/hidden_services/xftp 2>/dev/null || true
docker compose up -d --remove-orphans 2>/dev/null || true
`
		if err := os.WriteFile(tmp, []byte(script), 0755); err != nil {
			slog.Error("rotate write script", "error", err)
			http.Error(w, "failed to write script", 500)
			return
		}
		go func() {
			time.Sleep(300 * time.Millisecond)
			if err := exec.Command("bash", tmp).Run(); err != nil {
				slog.Error("rotate run script", "error", err)
			}
			rotateMu.Lock()
			rotateRunning = false
			rotateMu.Unlock()
		}()
		writeJSON(w, map[string]string{"status": "rotation started"})
	})

	// ===== Dashboard Onion =====
	http.HandleFunc("/api/dashboard-onion", func(w http.ResponseWriter, r *http.Request) {
		if !middleware.IsStrictLocalOnly(r) {
			http.Error(w, "Forbidden. This information is only available when accessing directly via 127.0.0.1", http.StatusForbidden)
			return
		}
		onion := readTrim(filepath.Join(*dataDir, "dashboard_onion.txt"))
		if onion == "" {
			http.Error(w, "dashboard onion not available", 404)
			return
		}
		writeJSON(w, map[string]string{
			"onion": onion,
			"url":   "http://" + onion,
			"qr":    "/static/qr-dashboard.png",
		})
	})

	// ===== ICE / TURN =====
	http.HandleFunc("/api/ice-config", func(w http.ResponseWriter, r *http.Request) {
		if !middleware.IsLocalOrOnionAccess(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		iceCfg := &webrtc.ICEConfig{
			Secret: readTrim(filepath.Join(*dataDir, "ice_turn_secret.txt")),
			Onion:  readTrim(filepath.Join(*dataDir, "ice_onion.txt")),
		}
		writeJSON(w, iceCfg.GenerateConfig())
	})

	// ===== WebRTC Signaling =====
	http.HandleFunc("/api/call-signal", func(w http.ResponseWriter, r *http.Request) {
		if !middleware.IsLocalOrOnionAccess(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		room := r.URL.Query().Get("room")
		if room == "" {
			room = "default"
		}
		if r.Method == "POST" {
			defer r.Body.Close()
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, "bad json", 400)
				return
			}
			sigState.PostSignal(room, payload)
			writeJSON(w, map[string]any{"ok": true})
			return
		}
		writeJSON(w, sigState.GetState(room))
	})

	// ===== Vault =====
	http.HandleFunc("/api/vault/list", func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		writeJSON(w, map[string]any{
			"files":    vaultSvc.List(),
			"used_mb":  vaultSvc.SizeMB(),
			"quota_mb": vault.QuotaMB,
		})
	})

	http.HandleFunc("/api/vault/upload", func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		if r.Method != "POST" {
			http.Error(w, "POST only", 405)
			return
		}
		if err := r.ParseMultipartForm(50 << 20); err != nil {
			http.Error(w, "parse form: "+err.Error(), 400)
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "file: "+err.Error(), 400)
			return
		}
		defer file.Close()

		if header.Size > 50<<20 {
			http.Error(w, "file too large (max 50MB per upload)", 413)
			return
		}

		name, err := vaultSvc.Upload(header.Filename, file, header.Size)
		if err != nil {
			if err == io.ErrUnexpectedEOF {
				http.Error(w, "vault quota (2GB) exceeded", 413)
				return
			}
			http.Error(w, "save: "+err.Error(), 500)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "name": name, "size": header.Size})
	})

	http.HandleFunc("/api/vault/download", func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		name := r.URL.Query().Get("name")
		if name == "" {
			http.Error(w, "name required", 400)
			return
		}
		http.ServeFile(w, r, vaultSvc.Download(name))
	})

	http.HandleFunc("/api/vault/delete", func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		if r.Method != "POST" {
			http.Error(w, "POST only", 405)
			return
		}
		name := r.URL.Query().Get("name")
		if name == "" {
			http.Error(w, "name required", 400)
			return
		}
		if err := vaultSvc.Delete(name); err != nil {
			http.Error(w, "delete: "+err.Error(), 500)
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	})

	http.HandleFunc("/api/vault/save-note", func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		if r.Method != "POST" {
			http.Error(w, "POST only", 405)
			return
		}
		defer r.Body.Close()
		var req struct {
			Name    string `json:"name"`
			Content string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad json", 400)
			return
		}
		name, err := vaultSvc.SaveNote(req.Name, req.Content)
		if err != nil {
			http.Error(w, "save note: "+err.Error(), 500)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "name": name})
	})

	// ===== Treasury =====
	http.HandleFunc("/api/treasury/usdt-deposits", treasury.USDTDepositsHandler(tronMon))
	http.HandleFunc("/api/treasury/init-silver-round", treasury.InitSilverRoundHandler(*dataDir, vaultSvc.Path, func(price int64, action, ref string) {
		billSvc.RecordPayment(price, action, ref)
	}))
	http.HandleFunc("/api/treasury/claim-dividends", api.ClaimDividendsHandler(*dataDir))
	http.HandleFunc("/api/treasury/auto-round", treasury.AutoRoundHandler(*dataDir, vaultSvc.Path, tronMon))
	http.HandleFunc("/api/treasury/register-banknote", treasury.RegisterBanknoteHandler(*dataDir))
	http.HandleFunc("/api/treasury/simulate-deposit", treasury.SimulateDepositHandler(*dataDir, tronMon))
	http.HandleFunc("/api/reserve/por", treasury.ProofOfReserveHandler(*dataDir))
	http.HandleFunc("/api/reserve/proof", api.ProofOfReserveDetailHandler(*dataDir))

	// ===== Silver-Backed Asset API (b103) =====
	http.HandleFunc("/api/silver/mint", api.SilverAssetMintHandler(*dataDir))
	http.HandleFunc("/api/silver/burn", api.SilverAssetBurnHandler(*dataDir))
	http.HandleFunc("/api/silver/list", api.SilverAssetListHandler(*dataDir))

	// ===== Silver Coin Shop (b122+cycles+DC) =====
	http.HandleFunc("/api/silver/shop", api.SilverShopHandler(*dataDir))
	http.HandleFunc("/api/silver/buy", api.SilverBuyHandler(*dataDir))
	http.HandleFunc("/api/silver/my-coins", api.SilverMyCoinsHandler(*dataDir))
	http.HandleFunc("/api/silver/redeem", api.SilverRedeemHandler(*dataDir))

	http.HandleFunc("/api/treasury/state", api.TreasuryStateHandler(*dataDir))
	http.HandleFunc("/api/treasury/proof-of-reserve", api.ProofOfReserveHandler(*dataDir))

	// ===== Subscription Tiers =====
	http.HandleFunc("/api/subscription", api.SubscriptionHandler(*dataDir))

	// ===== Golden Wheel =====
	http.HandleFunc("/api/economy/wheel", api.WheelHandler())

	// ===== Auto-Mint by Treasury Tier =====
	http.HandleFunc("/api/economy/auto-mint", api.AutoMintHandler(*dataDir))

	// ===== Crafting 5→1 + Leaderboard + Auto-Reinvest =====
	http.HandleFunc("/api/economy/crafting", api.CraftingHandler(*dataDir))
	http.HandleFunc("/api/economy/reinvest", api.AutoReinvestHandler(*dataDir))

	// ===== Onboarding Funnel (A3.1) =====
	http.HandleFunc("/api/economy/onboarding", api.OnboardingHandler(*dataDir))
	http.HandleFunc("/api/economy/onboarding/welcome", api.OnboardingWelcomeHandler(*dataDir))
	http.HandleFunc("/api/economy/onboarding/starter", api.OnboardingStarterHandler(*dataDir))
	http.HandleFunc("/api/economy/onboarding/guide", api.OnboardingGuideHandler(*dataDir))

	// ===== Silver Spot Oracle + Deflation =====
	http.HandleFunc("/api/economy/oracle", api.OracleLiveHandler(silverOracle))
	http.HandleFunc("/api/economy/deflate", api.DeflationHandler(*dataDir))

	// ===== Steward AI Core (A5.2) =====
	http.HandleFunc("/api/steward", api.StewardHandler(stewardSvc, sendToIslandRole))

	// ===== AI Arbitration (A5.4) =====
	http.HandleFunc("/api/arbitration", api.ArbitrationHandler(*dataDir))

	// ===== Franchise Licenses + Earmarked Accounts =====
	http.HandleFunc("/api/franchise/licenses", api.LicenseHandler(*dataDir))
	http.HandleFunc("/api/franchise/earmarks", api.EarmarkHandler(*dataDir))

	// ===== Mint Authorization + Templates =====
	http.HandleFunc("/api/franchise/mint-auth", api.MintAuthHandler(*dataDir))
	http.HandleFunc("/api/franchise/templates", api.TemplateHandler(*dataDir))

	// ===== Service Registry + Marketplace =====
	http.HandleFunc("/api/services/registry", api.ServiceRegistryHandler(*dataDir))
	http.HandleFunc("/api/services/marketplace", api.ServiceMarketplaceHandler(*dataDir))

	// ===== AI Constitution + Monitor =====
	http.HandleFunc("/api/ai/constitution", api.ConstitutionHandler(*dataDir))
	http.HandleFunc("/api/ai/monitor", api.StewardMonitorHandler())

	// ===== Cross-Franchise Settlement + Royalties =====
	http.HandleFunc("/api/franchise/settlements", api.SettlementHandler(*dataDir))
	http.HandleFunc("/api/franchise/royalties", api.RoyaltyHandler(*dataDir))

	// ===== POS Terminal (Merchant Payments) =====
	http.HandleFunc("/api/pos", api.POSHandler(*dataDir))
	http.HandleFunc("/api/pos/qr", api.QRHandler(*dataDir))

	// ===== Tokenomics Constants =====
	http.HandleFunc("/api/economy/tokenomics", api.TokenomicsHandler())
	http.HandleFunc("/api/economy/dividend-admin", api.DividendAdminHandler(*dataDir))
	http.HandleFunc("/api/economy/rates", api.MultiCurrencyRatesHandler())
	http.HandleFunc("/api/economy/invoice-webhook-test", api.InvoiceWebhookTestHandler())

	// ===== Vault Mining (subscription-funded) =====
	http.HandleFunc("/api/mining", api.MiningHandler(*dataDir))
	http.HandleFunc("/api/argentum", api.ArgentumHandler(*dataDir))

	// ===== Advertising Tags (deflationary) =====
	http.HandleFunc("/api/advertising", api.AdvertisingHandler(*dataDir))

	// ===== Genesis ICO =====
	http.HandleFunc("/api/genesis/ico", api.GenesisIcoHandler(*dataDir))

	// ===== Genesis Lock (frozen dividends) =====
	http.HandleFunc("/api/genesis/lock", api.GenesisLockHandler(*dataDir))

	// ===== Transport API (app registry + WebSocket relay) =====
	transportHub := transport.NewHub(*dataDir)
	http.HandleFunc("/api/transport/v1/register", transportHub.RegisterHandler())
	http.HandleFunc("/api/transport/v1/ws", transportHub.WsHandler())
	http.HandleFunc("/api/transport/v1/send", transportHub.SendHandler())
	http.HandleFunc("/api/transport/v1/health", transportHub.HealthHandler())
	http.HandleFunc("/api/transport/v1/stats", transportHub.StatsHandler())
	http.HandleFunc("/api/transport/v1/apps", transportHub.ListHandler())
	http.HandleFunc("/api/transport/v1/contacts", transportHub.ContactHandler())
	http.HandleFunc("/api/transport/v1/batch", transportHub.BatchHandler())
	http.HandleFunc("/api/transport/v1/config", transportHub.ConfigHandler())
	http.HandleFunc("/api/transport/v1/audit", transportHub.AuditHandler())
	http.HandleFunc("/api/transport/v1/backpressure", transportHub.BackpressureHandler())
	http.HandleFunc("/api/transport/v1/webhook", transportHub.WebhookHandler())
	http.HandleFunc("/api/transport/v1/backup", transportHub.BackupHandler())
	http.HandleFunc("/api/transport/v1/backup/restore", transportHub.RestoreHandler())
	http.HandleFunc("/api/transport/v1/discover", transportHub.DiscoveryHandler())
	http.HandleFunc("/api/transport/v1/gateway", transportHub.GatewayHandler())

	// Legacy transport proxy (API gateway)
	http.HandleFunc("/api/transport/info", api.TransportHandler(*dataDir))
	http.HandleFunc("/api/transport/health", api.TransportHandler(*dataDir))
	http.HandleFunc("/api/transport/status", api.TransportHandler(*dataDir))
	http.HandleFunc("/api/transport/send", api.TransportSendHandler(*dataDir))
	slog.Info("transport handlers registered (app registry + SimpleX bridge relay)")

	// ===== Simplex Chat Relay (admin UI backend) =====
	chatHub := api.GlobalChatHub.WithFile(filepath.Join(*dataDir, "chat_history.json"))

	// SQLite ChatStore for persistent message storage (graceful fallback to JSON)
	if cs, err := store.NewChatStore(filepath.Join(*dataDir, "chat.db")); err != nil {
		slog.Warn("chat store not available, falling back to JSON", "error", err)
	} else {
		chatHub.ChatStore = cs
		slog.Info("SQLite chat store initialized")
	}

	// ===== Multi-Currency Token Store =====
	dbStore, err := store.Open(filepath.Join(*dataDir, "isle.db"))
	if err != nil {
		slog.Error("failed to open isle.db", "error", err)
		os.Exit(1)
	}
	_ = store.NewPINStore(dbStore)
	tokenStore := store.NewTokenStore(dbStore)
	extWalletStore := store.NewExternalWalletStore(dbStore)
	slog.Info("wallet stores initialized (PIN, NT, tokens, external wallets)")

	http.HandleFunc("/api/chat/history", api.ChatHistoryHandler(chatHub))
	http.HandleFunc("/api/chat/stream", api.ChatStreamHandler(chatHub))
	http.HandleFunc("/api/chat/send", api.ChatSendHandler(chatHub))
	http.HandleFunc("/api/chat/clear", api.ChatClearHandler(chatHub))
	http.HandleFunc("/api/chat/delete", api.ChatDeleteHandler(chatHub))
	http.HandleFunc("/api/chat/delete/contact", api.ChatDeleteContactHandler(chatHub))
	http.HandleFunc("/api/chat/address", api.ChatAddressHandler(*dataDir))
	http.HandleFunc("/api/chat/address/create", api.ChatAddressCreateHandler(*dataDir))
	http.HandleFunc("/api/chat/contacts", api.ChatContactsHandler())
	http.HandleFunc("/api/chat/contact", api.ChatContactHandler())
	http.HandleFunc("/api/chat/connect", api.ChatConnectHandler())
	http.HandleFunc("/api/chat/qr", api.ChatQRHandler(*dataDir))
	http.HandleFunc("/api/chat/status", api.ChatStatusHandler(chatHub))
	http.HandleFunc("/api/inquisitor/report", api.InquisitorReportHandler(chatHub, startTime))
	http.HandleFunc("/api/chat/export", api.ChatExportHandler(chatHub))
	http.HandleFunc("/api/chat/archive", api.ChatArchiveHandler(chatHub))
	http.HandleFunc("/api/chat/archive/list", api.ChatArchiveListHandler(chatHub, *dataDir))
	http.HandleFunc("/api/chat/archive/restore", api.ChatArchiveRestoreHandler(chatHub, *dataDir))
	http.HandleFunc("/api/chat/edit", api.ChatEditHandler(chatHub))
	http.HandleFunc("/api/chat/search", api.ChatSearchHandler(chatHub))
	http.HandleFunc("/api/chat/contact/alias", api.ChatAliasHandler())
	http.HandleFunc("/api/chat/stats", api.ChatStatsHandler(chatHub))
	http.HandleFunc("/api/chat/backup", api.ChatBackupHandler(chatHub))
	http.HandleFunc("/api/chat/contact/info", api.ChatContactInfoHandler(chatHub))
	http.HandleFunc("/api/chat/clear-old", api.ChatClearOldHandler(chatHub))
	http.HandleFunc("/api/chat/pin", api.ChatPinHandler(chatHub))
	http.HandleFunc("/api/chat/react", api.ChatReactHandler(chatHub))
	http.HandleFunc("/api/chat/server-status", api.SetServerStatusHandler(*dataDir))
	http.HandleFunc("/api/chat/bridge-health", api.BridgeHealthHandler())
	http.HandleFunc("/api/chat/bridge-heartbeat", api.BridgeHeartbeatHandler())
	http.HandleFunc("/api/chat/bridge-metrics", api.BridgeMetricsHandler())
	http.HandleFunc("/api/chat/bridge-latency", api.BridgeLatencyHandler())
	http.HandleFunc("/api/chat/bridge-reconnect", bridge.BridgeReconnectHandler())
	http.HandleFunc("/api/chat/broadcast", api.ChatBroadcastHandler(chatHub))
	http.HandleFunc("/api/marketplace", api.MarketplaceHandler())
	http.HandleFunc("/api/dao", api.DAOHandler())
	http.HandleFunc("/api/chat/last-message", api.ChatLastMessageHandler(chatHub))
	http.HandleFunc("/api/chat/typing", api.ChatTypingHandler())
	http.HandleFunc("/api/chat/schedule", api.ChatScheduleHandler())
	http.HandleFunc("/api/chat/auto-reply", api.ChatAutoReplyHandler())
	http.HandleFunc("/api/chat/groups", api.ChatGroupsHandler())
	http.HandleFunc("/api/chat/labels", api.ChatLabelsHandler())
	http.HandleFunc("/api/chat/drafts", api.ChatDraftsHandler())
	http.HandleFunc("/api/chat/webhook", api.ChatWebhookHandler())
	http.HandleFunc("/api/chat/search/advanced", api.ChatAdvancedSearchHandler(chatHub))
	http.HandleFunc("/api/chat/templates", api.ChatTemplatesHandler())
	http.HandleFunc("/api/chat/template/send", api.ChatTemplateSendHandler())
	http.HandleFunc("/api/chat/analytics", api.ChatAnalyticsHandler(chatHub))
	http.HandleFunc("/api/chat/batch-forward", api.ChatBatchForwardHandler(chatHub))
	http.HandleFunc("/api/chat/pay", api.ChatPayHandler(chatHub))
	http.HandleFunc("/api/chat/recall", api.ChatRecallHandler(chatHub))
	http.HandleFunc("/api/chat/read-receipt", api.ChatReadReceiptHandler(chatHub))
	http.HandleFunc("/api/chat/voice", api.ChatVoiceHandler())
	http.HandleFunc("/api/chat/send-file", api.ChatSendFileHandler(*dataDir))
	http.HandleFunc("/api/chat/theme", api.ChatThemeHandler())
	http.HandleFunc("/api/chat/language", api.ChatLanguageHandler())
	http.HandleFunc("/api/chat/contact/language", api.ChatContactLanguageHandler())
	http.HandleFunc("/api/chat/ai", api.ChatStewardAIHandler())
	http.HandleFunc("/api/chat/encryption", api.ChatEncryptionHandler())

	// Cycle 21: Contact trust/verification
	http.HandleFunc("/api/chat/trust", api.ChatTrustHandler())

	// Cycle 23: Media gallery
	http.HandleFunc("/api/chat/media", api.ChatMediaHandler(chatHub))

	// Cycle 24: Message translation
	http.HandleFunc("/api/chat/translate", api.ChatTranslateHandler())

	// Cycle 25: Link preview
	http.HandleFunc("/api/chat/link-preview", api.ChatLinkPreviewHandler())

	// Cycle 26: Custom notification sounds per contact
	http.HandleFunc("/api/chat/sound", api.ChatSoundHandler())

	// Cycle 27: AI context-aware suggestions
	http.HandleFunc("/api/chat/suggest", api.ChatSuggestHandler(chatHub))

	// Cycle 28: Bulk operations
	http.HandleFunc("/api/chat/bulk-delete", api.ChatBulkDeleteHandler(chatHub))
	http.HandleFunc("/api/chat/bulk-forward", api.ChatBulkForwardHandler(chatHub))

	// Cycle 29: Contact status indicators
	http.HandleFunc("/api/chat/contact/status", api.ChatContactStatusHandler())

	// Cycle 30: Security audit log (enhanced)
	http.HandleFunc("/api/admin/audit-log/enhanced", api.AuditLogEnhancedHandler())

	http.HandleFunc("/api/admin/audit-log", api.AuditLogHandler())
	http.HandleFunc("/api/admin/metrics", api.DetailedMetricsHandler())
	http.HandleFunc("/api/admin/diagnostics", api.SystemDiagnosticsHandler())
	http.HandleFunc("/api/admin/status-page", api.StatusPageHandler(chatHub))
	http.HandleFunc("/api/admin/rate-limit-status", api.RateLimitStatusHandler())
	http.HandleFunc("/api/admin/rate-limit-config", api.RateLimitConfigHandlerV2())
	http.HandleFunc("/api/admin/rate-limit-config/v2", api.RateLimitConfigHandlerV2())
	http.HandleFunc("/api/admin/content-filter", api.ContentFilterHandler())
	http.HandleFunc("/api/admin/content-filter/rules", api.ContentFilterRulesHandler())
	http.HandleFunc("/api/admin/content-filter/test", api.ContentFilterTestHandler())
	http.HandleFunc("/api/admin/docker", api.DockerStatusHandler())
	http.HandleFunc("/api/admin/metrics/system", api.SystemMetricsHandler())
	http.HandleFunc("/api/admin/metrics/bandwidth", api.BandwidthHandler())
	http.HandleFunc("/api/admin/metrics/memory-trend", api.MemoryTrendHandler())
	http.HandleFunc("/api/admin/port-scan", api.PortScanHandler())
	http.HandleFunc("/api/admin/service-deps", api.ServiceDepsHandler())
	http.HandleFunc("/api/admin/dns-check", api.DNSCheckHandler())
	http.HandleFunc("/api/admin/infra-audit", api.InfraAuditHandler())
	http.HandleFunc("/api/admin/full-audit", api.FullAuditHandler())
	http.HandleFunc("/api/admin/snap-check", api.SnapMigrationHandler())
	http.HandleFunc("/api/admin/service-hardening", api.ServiceHardeningHandler())
	http.HandleFunc("/api/admin/kernel-tuning", api.KernelTuningHandler())
	http.HandleFunc("/api/admin/backup-verify", api.USBBackupVerifyHandler())
	http.HandleFunc("/api/admin/updates", api.UpdateCheckHandler())
	http.HandleFunc("/api/admin/events", api.EventsHandler())
	http.HandleFunc("/api/admin/metrics/stream", api.MetricsStreamHandler())
	http.HandleFunc("/api/admin/live", api.LiveDashboardHandler())
	http.HandleFunc("/api/admin/routes", api.RoutesHandler())
	http.HandleFunc("/api/admin/disk-alerts", health.DiskAlertHandler())
	http.HandleFunc("/api/admin/disk-alerts/ack", health.DiskAlertAckHandler())
	http.HandleFunc("/api/admin/disk-usage", api.DiskUsageHandler())
	http.HandleFunc("/api/admin/disk-trend", api.DiskTrendHandler())
	http.HandleFunc("/api/admin/disk-cleanup", api.DiskCleanupHandler())
	http.HandleFunc("/api/admin/maintenance", api.MaintenanceHandler(*dataDir))
	http.HandleFunc("/api/admin/logs", api.LogsHandler(*dataDir))
	http.HandleFunc("/api/admin/info", api.NodeInfoHandler(*dataDir, *listen, buildVersion, startTime))
	http.HandleFunc("/api/admin/backup", api.BackupHandler(*dataDir))
	http.HandleFunc("/api/admin/backup/verify", api.BackupVerifyHandler())
	http.HandleFunc("/api/admin/backup/verify-all", api.BackupVerifyAllHandler())

	// ===== Backup Remote Sync (C46) =====
	api.InitBackupSync(*dataDir)
	http.HandleFunc("/api/admin/backup/sync", api.BackupSyncHandler(*dataDir))
	http.HandleFunc("/api/admin/backup/sync-status", api.BackupSyncStatusHandler())

	// ===== Service Restart (C31) =====
	http.HandleFunc("/api/admin/service/restart", api.ServiceRestartHandler())
	http.HandleFunc("/api/admin/service/status", api.ServiceStatusHandler())
	http.HandleFunc("/api/admin/container/list", api.DockerContainerListHandler())
	http.HandleFunc("/api/admin/container/logs", api.DockerContainerLogsHandler())

	// ===== Watchdog ping =====
	http.HandleFunc("/api/admin/ping", api.PingHandler(startTime))

	// ===== Mobile companion API (C19) =====
	http.HandleFunc("/api/mobile/status", api.MobileStatusHandler(startTime))
	slog.Info("mobile companion API registered")

	// i18n endpoints (Phase IV C5)
	http.HandleFunc("/api/i18n/languages", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"ok":        true,
			"languages": i18n.Global.Languages(),
		})
	})
	http.HandleFunc("/api/i18n/translate", func(w http.ResponseWriter, r *http.Request) {
		lang := r.URL.Query().Get("lang")
		key := r.URL.Query().Get("key")
		if lang == "" || key == "" {
			http.Error(w, "lang and key required", 400)
			return
		}
		writeJSON(w, map[string]any{
			"ok":    true,
			"lang":  lang,
			"key":   key,
			"value": i18n.T(lang, key),
		})
	})

	// Performance profiling (Phase IV C6)
	http.HandleFunc("/api/admin/metrics/perf", api.PerfStatsHandler())
	http.HandleFunc("/api/admin/metrics/perf/reset", api.PerfStatsResetHandler())

	// HTML Health Dashboard (Phase IV C7)
	http.HandleFunc("/api/admin/dashboard/html", api.DashboardHandler())

	// ===== Relay test / status =====
	http.HandleFunc("/api/admin/relay/test", api.RelayTestHandler())
	http.HandleFunc("/api/admin/relay/status", api.RelayStatusHandler())

	http.HandleFunc("/api/admin/config", api.ConfigHandler(*dataDir))

	// ===== Container auto-healer (Cycle 58) =====
	api.StartContainerAutoHealer()
	slog.Info("container auto-healer registered")

	// ===== Webhook delivery queue =====
	api.GlobalWebhookQueue = api.NewWebhookQueue(filepath.Join(*dataDir, "webhook_queue.json"))
	http.HandleFunc("/api/admin/webhook-queue", api.WebhookQueueHandler())
	http.HandleFunc("/api/admin/webhook-queue/stats", api.WebhookQueueStatsHandler())
	http.HandleFunc("/api/admin/webhook-queue/retry-dead", api.WebhookQueueRetryDeadHandler())
	http.HandleFunc("/api/admin/webhook-queue/dead", api.WebhookQueueDeadHandler())
	http.HandleFunc("/api/admin/webhook-queue/retry-all", api.WebhookQueueRetryAllHandler())
	slog.Info("webhook delivery queue registered")

	// ===== Database backup/restore =====
	http.HandleFunc("/api/db/list", api.DBListHandler(*dataDir))
	http.HandleFunc("/api/db/backup", api.DBBackupHandler(*dataDir))
	http.HandleFunc("/api/db/backup/list", api.DBBackupListHandler(*dataDir))
	http.HandleFunc("/api/db/restore", api.DBRestoreHandler(*dataDir))
	http.HandleFunc("/api/db/upload", api.DBUploadHandler(*dataDir))
	slog.Info("database backup/restore handlers registered")

	// ===== ParanoidX Bridge (V2Ray -> VPN -> Tor -> SimpleX) =====
	pxBridge := paranoidx.NewBridge(*dataDir, dockerComposeDir, 9050, "wg0", 17225)
	if err := pxBridge.Start(); err != nil {
		slog.Warn("paranoidx bridge start (non-fatal)", "err", err)
	} else {
		slog.Info("paranoidx bridge started, proxy chain: " + pxBridge.ResolveProxyChain())
	}
	http.HandleFunc("/api/paranoidx/status", pxBridge.StatusHandler)
	http.HandleFunc("/api/paranoidx/history", paranoidx.HistoryHandler())
	http.HandleFunc("/api/paranoidx/config", pxBridge.ConfigHandler)
	http.HandleFunc("/api/paranoidx/config/update", pxBridge.ConfigUpdateHandler)
	http.HandleFunc("/api/paranoidx/chain/build", pxBridge.ChainBuildHandler)
	http.HandleFunc("/api/paranoidx/chain/teardown", pxBridge.ChainTeardownHandler)
	http.HandleFunc("/api/paranoidx/chain/state", pxBridge.ChainStateHandler)
	http.HandleFunc("/api/paranoidx/chain/test", pxBridge.ChainTestHandler)
	http.HandleFunc("/api/paranoidx/vpn/profiles", pxBridge.VPNProfileHandler)
	http.HandleFunc("/api/paranoidx/vpn/up", pxBridge.VPNUpHandler)
	http.HandleFunc("/api/paranoidx/vpn/down", pxBridge.VPNDownHandler)
	http.HandleFunc("/api/paranoidx/vpn/delete", pxBridge.VPNProfileDeleteHandler)

	// ===== VMess Management (Phase VI C4) =====
	api.InitVMess(*dataDir)
	http.HandleFunc("/api/paranoidx/vmess/status", api.VMessStatusHandler())
	http.HandleFunc("/api/paranoidx/vmess/init", api.VMessInitHandler())
	http.HandleFunc("/api/paranoidx/vmess/rotate", api.VMessRotateHandler())
	http.HandleFunc("/api/paranoidx/vmess/config", api.VMessConfigHandler())
	slog.Info("vmess management handlers registered")

	// ===== VLESS+Reality Management (Phase VI C5 — replaces deprecated VMess) =====
	api.InitVLESS(*dataDir)
	http.HandleFunc("/api/paranoidx/vless/status", api.VLESSStatusHandler())
	http.HandleFunc("/api/paranoidx/vless/init", api.VLESSInitHandler())
	http.HandleFunc("/api/paranoidx/vless/rotate", api.VLESSRotateHandler())
	http.HandleFunc("/api/paranoidx/vless/config", api.VLESSConfigHandler())
	slog.Info("vless+reality management handlers registered")

	// ===== Comprehensive debug endpoint (C66) =====
	http.HandleFunc("/api/debug", func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		hostname, _ := os.Hostname()

		data := map[string]any{
			"ok":        true,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"hostname":  hostname,
			"pid":       os.Getpid(),
			"data_dir":  *dataDir,
			"system":    api.CollectSystemMetrics(startTime),
			"transport": map[string]any{
				"available": true,
				"stats":     transportHub.Stats(),
			},
			"bridge": map[string]any{
				"connected":       api.BridgeConnected,
				"reconnect_count": api.BridgeReconnectCount,
			},
			"paranoidx": paranoidx.GetOverallStatus(),
					}

					accept := r.Header.Get("Accept")
					if strings.Contains(accept, "text/html") {
						w.Header().Set("Content-Type", "text/html; charset=utf-8")
						api.RenderDebugHTML(w, data)
						return
					}
					writeJSON(w, data)
				})
	slog.Info("debug endpoint /api/debug registered")

	// Register configurable rate limiters
	api.RegisterConfigurableLimiter("chat_send", api.GetChatSendLimiter())
	api.RegisterConfigurableLimiter("unlock", unlockLimiter)
	api.GlobalPerEndpointLimiter = api.NewPerEndpointRateLimiter(*dataDir)
	api.GlobalContentFilter = api.NewContentFilterEngine(*dataDir)

	// ===== Container API (BIP39-sealed vault for config + secrets) =====
	api.GlobalContainer = container.New(filepath.Join(*dataDir, "container.bin"))
	api.DataDir = *dataDir
	api.DockerComposeDir = dockerComposeDir
	http.HandleFunc("/api/container/generate-seed", api.ContainerGenerateSeedHandler())
	http.HandleFunc("/api/container/init", api.ContainerInitHandler())
	http.HandleFunc("/api/container/open", api.ContainerOpenHandler())
	http.HandleFunc("/api/container/close", api.ContainerCloseHandler())
	http.HandleFunc("/api/container/status", api.ContainerStatusHandler())
	http.HandleFunc("/api/container/import-config", api.ContainerImportConfigHandler())
	http.HandleFunc("/api/container/restore", api.ContainerRestoreHandler())
	http.HandleFunc("/api/panic", api.PanicHandler())
	http.HandleFunc("/api/chat/auto-delete", api.AutoDeleteConfigHandler())

	// ===== DC CryptoCloud API (torrent-like container distribution) =====
	dcCloud := dc.NewCloud(*dataDir)
	dcCloud.LoadState()
	dcHandlers := dc.NewDCHandlers(dcCloud)
	http.HandleFunc("/api/dc/seed", dcHandlers.SeedHandler())
	http.HandleFunc("/api/dc/announce", dcHandlers.AnnonceHandler())
	http.HandleFunc("/api/dc/swarm", dcHandlers.SwarmHandler())
	http.HandleFunc("/api/dc/fetch", dcHandlers.FetchHandler())
	http.HandleFunc("/api/dc/list", dcHandlers.ListHandler())
	http.HandleFunc("/api/dc/status", dcHandlers.StatusHandler())
	http.HandleFunc("/api/dc/manifest", dcHandlers.ManifestHandler())
	http.HandleFunc("/api/dc/piece", dcHandlers.PieceHandler())
	http.HandleFunc("/api/dc/unseed", dcHandlers.UnseedHandler())
	http.HandleFunc("/api/dc/heal-report", dcHandlers.HealReportHandler())
	http.HandleFunc("/api/dc/seed-container", func(w http.ResponseWriter, r *http.Request) {
		containerPath := filepath.Join(*dataDir, "container.bin")
		if _, err := os.Stat(containerPath); os.IsNotExist(err) {
			writeJSON(w, map[string]any{"error": "no container.bin found"})
			return
		}
		manifest, err := dcCloud.SeedContainer(containerPath, "cryptocontainer")
		if err != nil {
			writeJSON(w, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, map[string]any{
			"ok":        true,
			"infohash":  manifest.Infohash,
			"size_mb":   manifest.Size / (1024 * 1024),
			"pieces":    manifest.PieceCount,
		})
	})

	// ===== Invoice API =====
	api.GlobalInvoiceManager = api.NewInvoiceManager().WithFile(filepath.Join(*dataDir, "invoices.json"))
	http.HandleFunc("/api/chat/invoice/create", api.InvoiceCreateHandler(api.GlobalInvoiceManager))
	http.HandleFunc("/api/chat/invoice/list", api.InvoiceListHandler(api.GlobalInvoiceManager))
	http.HandleFunc("/api/chat/invoice/pay", api.InvoicePayHandler(api.GlobalInvoiceManager))
	http.HandleFunc("/api/chat/invoice/stats", api.InvoiceStatsHandler(api.GlobalInvoiceManager))
	http.HandleFunc("/api/chat/invoice/export-csv", api.InvoiceExportCSVHandler(api.GlobalInvoiceManager))
	api.GlobalMsgIndex = api.NewMessageIndex(chatHub)
	http.HandleFunc("/api/admin/search-index", api.SearchIndexHandler(api.GlobalMsgIndex))

	// ===== Inverted Search Index (C44) =====
	api.GlobalInvertedIndex = api.NewInvertedIndex(chatHub, *dataDir)
	http.HandleFunc("/api/chat/search/index-status", api.SearchIndexStatusHandler(api.GlobalInvertedIndex))
	http.HandleFunc("/api/chat/search/rebuild-index", api.SearchIndexRebuildHandler(api.GlobalInvertedIndex))
	http.HandleFunc("/api/chat/search/rebuild", api.SearchIndexRebuildHandler(api.GlobalInvertedIndex))

	// ===== Disk Trend (C28) =====
	api.InitDiskTrend(*dataDir).StartRecording(5 * time.Minute)
	slog.Info("disk trend recording started (interval: 5min)")
	slog.Info("message index initialized + search-index handler registered")
	slog.Info("invoice API registered")

	go func() {
		for {
			time.Sleep(1 * time.Hour)
			n := api.GlobalInvoiceManager.ExpireOld(24 * time.Hour)
			if n > 0 {
				slog.Info("invoice expiry: expired pending invoices", "count", n)
			}
		}
	}()

	// Swap expiry cron — auto-expire swaps every hour
	go func() {
		for {
			time.Sleep(1 * time.Hour)
			api.SwapExpiryCleaner()
		}
	}()
	slog.Info("swap expiry cron started (interval: 1h)")

	http.HandleFunc("/api/restart", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", 400)
			return
		}
		slog.Warn("restart requested via /api/restart")
		writeJSON(w, map[string]string{"status": "restarting"})
		// Fallback: start node directly after 45s if launch-node.sh hangs
		go func() {
			time.Sleep(45 * time.Second)
			if pgrep, _ := exec.Command("pgrep", "-x", "ParanoidX").Output(); len(pgrep) == 0 {
				slog.Warn("restart: launch-node.sh did not start node, starting directly")
				exec.Command("bash", "-c", "nohup /home/tomas/bin/ParanoidX &>/tmp/ParanoidX.log &").Start()
			}
		}()
		time.Sleep(300 * time.Millisecond)
		slog.Info("restart: calling launch-node.sh, then exiting")
		exec.Command("/home/tomas/ParanoidX/scripts/launch-node.sh").Start()
		syscall.Kill(syscall.Getpid(), syscall.SIGTERM)
	})

	// Auto-archive background goroutine (C57) — runs daily at 03:00
	api.GlobalAutoArchiveDays = 90
	go func() {
		for {
			now := time.Now()
			next := time.Date(now.Year(), now.Month(), now.Day(), 3, 0, 0, 0, now.Location())
			if now.After(next) {
				next = next.Add(24 * time.Hour)
			}
			time.Sleep(time.Until(next))
			days := api.GlobalAutoArchiveDays
			if days <= 0 {
				days = 90
			}
			archived, err := chatHub.ArchiveOldMessages(*dataDir, days)
			if err != nil {
				slog.Error("auto-archive", "error", err)
			} else if archived > 0 {
				slog.Info("auto-archive completed", "archived", archived, "older_than_days", days)
			}
		}
	}()
	slog.Info("auto-archive cron started (runs daily at 03:00)")

	// Auto-delete background goroutine
	go func() {
		for {
			time.Sleep(30 * time.Second)
			removed := chatHub.DeleteExpiredMessages()
			if removed > 0 {
				slog.Info("auto-delete cleaned messages", "count", removed)
			}
		}
	}()
	api.GlobalSchedule = api.NewScheduleManager(chatHub, *dataDir)
	api.InitContactLanguages(*dataDir)
	slog.Info("chat relay handlers registered")

	// ===== Account Management (BIP39 + Ed25519) =====
	http.HandleFunc("/api/account/create", api.AccountCreateHandler())
	http.HandleFunc("/api/account/restore", api.AccountRestoreHandler())
	http.HandleFunc("/api/account/verify", api.AccountVerifyHandler())
	slog.Info("account handlers registered (BIP39 mnemonic)")

	// ===== BIP39 Crypto API =====
	http.HandleFunc("/api/crypto/bip39/generate", api.BIP39GenerateHandler())
	http.HandleFunc("/api/crypto/bip39/validate", api.BIP39ValidateHandler())
	http.HandleFunc("/api/crypto/bip39/wordlist", api.BIP39WordlistHandler())
	slog.Info("bip39 crypto handlers registered")

	// ===== BTC Atomic Swaps =====
	http.HandleFunc("/api/swap/create", api.SwapCreateHandler())
	http.HandleFunc("/api/swap/confirm", api.SwapConfirmHandler())
	http.HandleFunc("/api/swap/cancel", api.SwapCancelHandler())
	http.HandleFunc("/api/swap/claim", api.SwapClaimHandler())
	http.HandleFunc("/api/swap/refund", api.SwapRefundHandler())
	http.HandleFunc("/api/swap/list", api.SwapListHandler())
	slog.Info("btc atomic swap handlers registered")

	// ===== ETH Bridge (LayerZero) =====
	http.HandleFunc("/api/bridge/create", api.BridgeCreateHandler())
	http.HandleFunc("/api/bridge/confirm", api.BridgeConfirmHandler())
	http.HandleFunc("/api/bridge/complete", api.BridgeCompleteHandler())
	http.HandleFunc("/api/bridge/list", api.BridgeListHandler())
	http.HandleFunc("/api/bridge/status", api.BridgeStatusHandler())
	slog.Info("eth bridge handlers registered")

	// ===== ICO Vesting Tiers =====
	http.HandleFunc("/api/ico/info", api.ICOInfoHandler(*dataDir))
	http.HandleFunc("/api/ico/invest", api.ICOInvestHandler(*dataDir))
	http.HandleFunc("/api/ico/status", api.ICOStatusHandler(*dataDir))

	// ===== Vault Encryption =====
	http.HandleFunc("/api/vault/encrypt", api.VaultEncryptHandler(vaultSvc))
	http.HandleFunc("/api/vault/decrypt", api.VaultDecryptHandler(vaultSvc))

	// ===== Wallet Send/Receive =====
	http.HandleFunc("/api/wallet/send", api.WalletSendHandler(*dataDir))
	http.HandleFunc("/api/wallet/receive", api.WalletReceiveHandler(*dataDir))
	http.HandleFunc("/api/wallet/history", api.WalletHistoryHandler(*dataDir))

	// ===== Token Wallet =====
	http.HandleFunc("/api/token/list", api.TokenListHandler(tokenStore))
	http.HandleFunc("/api/token/balances", api.TokenBalancesHandler(tokenStore))
	http.HandleFunc("/api/token/add-custom", api.TokenAddCustomHandler(tokenStore))
	http.HandleFunc("/api/token/remove-custom", api.TokenRemoveCustomHandler(tokenStore))
	http.HandleFunc("/api/token/update-balance", api.TokenUpdateBalanceHandler(tokenStore))
	slog.Info("token wallet routes registered")

	// ===== External Wallets =====
	http.HandleFunc("/api/external-wallet/list", api.ExternalWalletListHandler(extWalletStore))
	http.HandleFunc("/api/external-wallet/link", api.ExternalWalletLinkHandler(extWalletStore))
	http.HandleFunc("/api/external-wallet/unlink", api.ExternalWalletUnlinkHandler(extWalletStore))
	http.HandleFunc("/api/external-wallet/sync", api.ExternalWalletSyncHandler(extWalletStore))
	http.HandleFunc("/api/external-wallet/verify", api.ExternalWalletVerifyHandler(extWalletStore))
	slog.Info("external wallet routes registered")

	// ===== RWA =====
	http.HandleFunc("/api/rwa/register", func(w http.ResponseWriter, r *http.Request) {
		if !middleware.IsLocalOrOnionAccess(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		if !isRoyalNode(*dataDir) {
			http.Error(w, "RWA / tokenization requires royal node", http.StatusForbidden)
			return
		}
		defer r.Body.Close()
		var req struct {
			Type      string `json:"type"`
			Serial    string `json:"serial"`
			BackingNg int64  `json:"backing_ng"`
			Holder    string `json:"holder"`
			NfcUid    string `json:"nfc_uid,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad json", 400)
			return
		}
		if req.Type == "" || req.Serial == "" || req.Holder == "" {
			http.Error(w, "type, serial, and holder are required", http.StatusBadRequest)
			return
		}
		if req.BackingNg <= 0 {
			http.Error(w, "backing_ng must be positive", http.StatusBadRequest)
			return
		}
		curReserve := int64(0)
		if b, err := os.ReadFile(filepath.Join(*dataDir, "silver_reserve_ng.txt")); err == nil {
			fmt.Sscanf(string(b), "%d", &curReserve)
		}
		if req.BackingNg > curReserve {
			http.Error(w, "backing_ng exceeds current silver reserve", http.StatusBadRequest)
			return
		}
		rwaFile := filepath.Join(*dataDir, "rwa_registry.json")
		var items []map[string]any
		if b, err := os.ReadFile(rwaFile); err == nil {
			json.Unmarshal(b, &items)
		}
		// Check for duplicate serial
		for _, existing := range items {
			if existing["serial"] == req.Serial {
				http.Error(w, "serial already registered", http.StatusConflict)
				return
			}
		}
		item := map[string]any{
			"id":         "rwa-" + time.Now().Format("20060102-150405"),
			"type":       req.Type,
			"serial":     req.Serial,
			"backing_ng": req.BackingNg,
			"holder":     req.Holder,
			"nfc_uid":    req.NfcUid,
			"issued":     time.Now().Format(time.RFC3339),
			"token":      "SILVER-BACKED-" + req.Serial,
		}
		items = append(items, item)
		fileutil.WriteJSON(rwaFile, items)
		pr := billSvc.GetPrices()
		token, _ := item["token"].(string)
		billSvc.RecordPayment(pr.RwaRegisterNg, "rwa_register", token)
		writeJSON(w, map[string]any{
			"ok":   true,
			"item": item,
			"nfc_format": "NFC tag UID (7-byte hex, e.g., 0479A1B2C3D4E5) — stored with RWA record for offline verification",
		})
	})

	http.HandleFunc("/api/rwa/list", func(w http.ResponseWriter, r *http.Request) {
		if !middleware.IsLocalOrOnionAccess(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		rwaFile := filepath.Join(*dataDir, "rwa_registry.json")
		var items []map[string]any
		if b, err := os.ReadFile(rwaFile); err == nil {
			json.Unmarshal(b, &items)
		}
		writeJSON(w, map[string]any{"items": items})
	})

	// ===== Billing =====
	http.HandleFunc("/api/billing/prices", func(w http.ResponseWriter, r *http.Request) {
		if !middleware.IsLocalOrOnionAccess(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		writeJSON(w, billSvc.GetPrices())
	})

	http.HandleFunc("/api/billing/payments", func(w http.ResponseWriter, r *http.Request) {
		if !middleware.IsLocalOrOnionAccess(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		writeJSON(w, map[string]any{"payments": billSvc.RecentPayments(20)})
	})

	// ===== Radio System =====
	radioSvc := radio.NewRadioService(*dataDir)
	annStore := radio.NewAnnouncementStore(*dataDir)
	radioHandler := api.RadioHandler(radioSvc, annStore)

	// Acestep AI Radio: connect if URL configured
	var acestepGen *acestep.Generator
	acestepURL := cfg.AcestepURL
	if acestepURL == "" {
		acestepURL = "http://192.168.1.129:8001"
	}
	acestepGen = acestep.NewGenerator(acestepURL, *dataDir)
	// Async health check — don't block startup for 2min TCP timeout
	go func() {
		if acestepGen.Healthy() {
			slog.Info("acestep AI radio connected", "url", acestepURL)
		} else {
			slog.Info("acestep AI radio not available", "url", acestepURL)
		}
	}()
	http.HandleFunc("/api/radio/acestep", api.AcestepHandler(acestepGen))

	// ===== Radio AI Content Generator (Phase VI C5) =====
	api.GlobalRadioAIGen = radio.NewAIContentGenerator(aiClient)
	slog.Info("radio AI content generator initialized", "ollama", ollamaURL)

	// ===== Radio Content Schedule (C33/C55) =====
	api.GlobalContentScheduler = radio.NewEnhancedScheduler(*dataDir)
	api.GlobalContentScheduler.SetOnTick(func(entry radio.ContentScheduleEntry) {
		slog.Info("content schedule tick", "type", entry.Type, "prompt", entry.Prompt)
	})
	api.GlobalContentScheduler.Start()
	http.HandleFunc("/api/radio/schedule-content", api.RadioScheduleHandler())
	http.HandleFunc("/api/radio/schedule", api.RadioScheduleHandler())
	http.HandleFunc("/api/radio/schedule/optimize", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		api.GlobalContentScheduler.Optimize()
		writeJSON(w, map[string]any{"ok": true})
	})
	http.HandleFunc("/api/radio/schedule/stats", func(w http.ResponseWriter, r *http.Request) {
		stats := api.GlobalContentScheduler.Stats()
		writeJSON(w, map[string]any{"ok": true, "stats": stats})
	})
	http.HandleFunc("/api/radio/schedule/rotation", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			writeJSON(w, map[string]any{"ok": true, "mode": api.GlobalContentScheduler.GetRotationMode()})
		case "POST":
			var req struct {
				Mode string `json:"mode"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
				return
			}
			mode := radio.RotationMode(req.Mode)
			if mode != radio.RotationSequential && mode != radio.RotationShuffle && mode != radio.RotationWeighted {
				writeJSON(w, map[string]any{"ok": false, "error": "mode must be sequential, shuffle, or weighted"})
				return
			}
			api.GlobalContentScheduler.SetRotationMode(mode)
			writeJSON(w, map[string]any{"ok": true, "mode": mode})
		default:
			http.Error(w, "GET/POST", 405)
		}
	})
	http.HandleFunc("/api/radio/schedule/time-slots", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			writeJSON(w, map[string]any{"ok": true, "time_slots": api.GlobalContentScheduler.GetTimeSlots()})
		case "POST":
			var req struct {
				TimeSlots []radio.TimeSlotSched `json:"time_slots"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
				return
			}
			api.GlobalContentScheduler.SetTimeSlots(req.TimeSlots)
			writeJSON(w, map[string]any{"ok": true})
		default:
			http.Error(w, "GET/POST", 405)
		}
	})

	// Pre-buffer first radio track for fast stream startup
	go func() {
		tracks := radio.BuildRadioPlaylist(*dataDir, true, true)
		if len(tracks) > 0 {
			fp := tracks[0].FilePath
			buf := make([]byte, 256*1024)
			f, err := os.Open(fp)
			if err == nil {
				n, _ := f.Read(buf)
				f.Close()
				slog.Info("radio pre-buffered first track", "path", fp, "bytes", n)
			}
		}
	}()

	// ===== P2P Registry: node discovery, regional routing =====
	nodeReg := registry.NewRegistry(*dataDir)
	http.HandleFunc("/api/node/announce", nodeReg.AnnounceHandler)
	http.HandleFunc("/api/node/discover", nodeReg.DiscoverHandler)
	http.HandleFunc("/api/node/heartbeat", nodeReg.HeartbeatHandler)
	http.HandleFunc("/api/node/list", nodeReg.ListHandler)
	http.HandleFunc("/api/node/status", nodeReg.StatusHandler)
	slog.Info("node registry started")

	// ===== P2P Tracker: BitTorrent-style swarm for radio tracks =====
	trk := tracker.New()
	http.HandleFunc("/api/tracker/announce", trk.Announce)
	http.HandleFunc("/api/tracker/scrape", trk.Scrape)
	http.HandleFunc("/api/tracker/nodes", trk.Nodes)
	slog.Info("P2P tracker started")

	// ===== P2P Transport: direct TCP for radio/vault/DC transfer =====
	p2pPort := 17001
	if p2p, err := envInt("P2P_PORT"); err == nil {
		p2pPort = p2p
	}
	p2pTrans := transport.NewTransfer(filepath.Join(*dataDir, "peer_cache"), p2pPort)
	if err := p2pTrans.Start(); err != nil {
		slog.Error("P2P transport", "error", err)
	} else {
		slog.Info("P2P transport listening", "port", p2pPort)
	}
	defer p2pTrans.Stop()

	// Connect DC cloud to P2P transport
	dcNodeID := fmt.Sprintf("%x", sha256.Sum256([]byte(*dataDir)))[:16]
	dcCloud.SetTransport(dc.NewP2PTransport(p2pTrans.Addr(), dcNodeID))

	// Connect DC cloud to node registry for auto-peer discovery
	dcReg := dc.NewRegistryClient("http://127.0.0.1:8080", dcNodeID, "island-node", p2pTrans.Addr())

	// Expose DC cloud globally for royal handlers
	api.GlobalDCCloud = dcCloud
	dcCloud.SetRegistry(dcReg)

	// .isle manifest generator hook into radio upload
	http.HandleFunc("/api/isle/generate", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			TrackID string `json:"track_id"`
			Path    string `json:"path"`
			Title   string `json:"title"`
			Kind    string `json:"kind"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if req.TrackID == "" || req.Path == "" {
			http.Error(w, "track_id and path required", http.StatusBadRequest)
			return
		}
		m, err := isle.BuildManifest(req.TrackID, req.Path, req.Title, req.Kind)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		m.Save(filepath.Join(*dataDir, "manifests"))
		writeJSON(w, map[string]any{"ok": true, "manifest": m})
	})
	slog.Info("isle manifest generator ready")

	// Unified radio API: stations, playlist, announce, upload
	http.HandleFunc("/api/radio", radioHandler)

	// Serve individual audio tracks from radio content directory
	http.HandleFunc("/api/radio/track", api.TrackStreamHandler(*dataDir))

	// Continuous MP3 stream from radio folder (shuffle + auto-repeat)
	http.HandleFunc("/api/radio/stream", api.StreamHandler(*dataDir))
	slog.Info("radio stream handler registered at /api/radio/stream")

	// Onion-routed radio stream — returns M3U8 playlist with onion-track URLs
	onionAddr := readTrim(filepath.Join(*dataDir, "dashboard_onion.txt"))
	http.HandleFunc("/api/radio/onion-stream", api.OnionStreamHandler(*dataDir, onionAddr))
	slog.Info("onion radio stream handler registered at /api/radio/onion-stream")

	// Legacy: list audio files from vault (for backwards compatibility)
	http.HandleFunc("/api/vault/audio", func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		vp := vaultSvc.Path
		audios := []map[string]any{}
		exts := map[string]bool{".mp3": true, ".wav": true, ".webm": true, ".ogg": true, ".m4a": true, ".aac": true, ".flac": true}
		ents, _ := os.ReadDir(vp)
		for _, e := range ents {
			if e.IsDir() || strings.HasPrefix(e.Name(), ".") { continue }
			if exts[strings.ToLower(filepath.Ext(e.Name()))] {
				if fi, err := e.Info(); err == nil {
					audios = append(audios, map[string]any{"name": e.Name(), "size": fi.Size(), "mtime": fi.ModTime().Format(time.RFC3339)})
				}
			}
		}
		writeJSON(w, map[string]any{"audios": audios})
	})

	// ===== SimpleX Channel Management (b109) =====
	http.HandleFunc("/api/simplex/channel/create", api.ChannelCreateHandler())
	http.HandleFunc("/api/simplex/channel/list", api.ChannelListHandler())
	http.HandleFunc("/api/simplex/channel/join", api.ChannelJoinHandler())
	http.HandleFunc("/api/simplex/channel/post", api.ChannelPostHandler())

	// ===== DID Verification (b110) =====
	http.HandleFunc("/api/did", api.DIDHandler())
	http.HandleFunc("/api/did/contact", api.ContactDIDHandler())

	// ===== Bridge Config (b111) =====
	http.HandleFunc("/api/chat/bridge-config", api.BridgeConfigHandler())

	// ===== Inter-Node Relay (b112) =====
	http.HandleFunc("/api/relay/receive", api.RelayReceiveHandler())
	http.HandleFunc("/api/relay/send", api.RelaySendHandler())
	http.HandleFunc("/api/relay/history", api.RelayHistoryHandler())

	// ===== Channels (using channels.Manager) =====
	chManager := channels.NewManager(*dataDir)
	http.HandleFunc("/api/channels/list", func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		writeJSON(w, map[string]any{"channels": chManager.ListChannels()})
	})
	http.HandleFunc("/api/channels/create", func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		if r.Method != "POST" {
			http.Error(w, "POST only", 405)
			return
		}
		var req struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad json", 400)
			return
		}
		if req.Name == "" {
			http.Error(w, "name required", 400)
			return
		}
		id := "ch-" + time.Now().Format("20060102-150405")
		chManager.AddChannel(id, req.Name, "", "creator")
		writeJSON(w, map[string]any{"ok": true, "channel": chManager.GetChannel(id)})
	})
	http.HandleFunc("/api/channels/access", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			v := r.URL.Query()
			channel := v.Get("channel")
			if channel == "" {
				writeJSON(w, map[string]any{"ok": true, "channels": []string{"general", "economy", "radio", "announcements"}, "public": true})
				return
			}
			ch := chManager.GetChannel(channel)
			if ch == nil {
				http.Error(w, "channel not found", 404)
				return
			}
			writeJSON(w, map[string]any{"ok": true, "channel": ch.Name, "role": ch.Role, "created": ch.CreatedAt})
			return
		}
		http.Error(w, "GET required", 400)
	})
	http.HandleFunc("/api/channels/view", func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id required", 400)
			return
		}
		ch := chManager.GetChannel(id)
		if ch == nil {
			http.Error(w, "not found", 404)
			return
		}
		writeJSON(w, map[string]any{"channel": ch, "messages": chManager.GetMessages(id, 50)})
	})
	http.HandleFunc("/api/channels/post", func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		if r.Method != "POST" {
			http.Error(w, "POST only", 405)
			return
		}
		var req struct {
			ChannelID string `json:"channel_id"`
			Text      string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad json", 400)
			return
		}
		if req.ChannelID == "" || req.Text == "" {
			http.Error(w, "channel_id and text required", 400)
			return
		}
		chManager.AddMessage(req.ChannelID, req.Text, "user")
		writeJSON(w, map[string]any{"ok": true})
	})
	http.HandleFunc("/api/channels/unread", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			var req struct {
				ChannelID string `json:"channel_id"`
			}
			json.NewDecoder(r.Body).Decode(&req)
			if req.ChannelID != "" {
				chManager.MarkRead(req.ChannelID)
			}
		}
		writeJSON(w, map[string]any{"ok": true, "unread": 0})
	})

	// ===== Market =====
	billRecorder := func(price int64, action, itemID string) {
		billSvc.RecordPayment(price, action, itemID)
	}

	http.HandleFunc("/api/market/list", api.MarketListHandler(*dataDir))
	http.HandleFunc("/api/market/sell", api.MarketSellHandler(*dataDir, billRecorder))
	http.HandleFunc("/api/market/buy", api.MarketBuyHandler(*dataDir, billRecorder))

	http.HandleFunc("/api/escrow/create", api.CreateEscrowHandler(*dataDir))
	http.HandleFunc("/api/escrow/release", api.ReleaseEscrowHandler(*dataDir))
	http.HandleFunc("/api/escrow/cancel", api.CancelEscrowHandler(*dataDir))
	http.HandleFunc("/api/escrow/list", api.ListEscrowHandler(*dataDir))
	http.HandleFunc("/api/escrow/buy", api.NewEscrowBuyHandler(*dataDir))
	http.HandleFunc("/api/escrow/auto-resolve", api.AutoResolveHandler(*dataDir))

	// ===== Royal→Sub Node Control =====
	royalSvc := royal.NewService(*dataDir)
	http.HandleFunc("/api/royal/register", royal.RegisterHandler(royalSvc))
	http.HandleFunc("/api/royal/nodes", royal.NodesHandler(royalSvc))
	http.HandleFunc("/api/royal/command", royal.CommandHandler(royalSvc))
	http.HandleFunc("/api/royal/heartbeat", royal.HeartbeatHandler(royalSvc))
	http.HandleFunc("/api/royal/key", royal.KeyHandler(royalSvc))
	go func() {
		for {
			time.Sleep(royal.HeartbeatInterval)
			royalSvc.CheckStale()
		}
	}()

	// ===== Royal Control =====
	http.HandleFunc("/royal/control", api.RoyalControlHandler(*dataDir))
	http.HandleFunc("/royal/sync", api.RoyalControlHandler(*dataDir))

	// ===== Royal Treasury Control (C1-C5) =====
	http.HandleFunc("/api/royal/treasury/state", api.RoyalTreasuryStateHandler(*dataDir))
	http.HandleFunc("/api/royal/treasury/reserve", api.RoyalReserveHandler(*dataDir))
	http.HandleFunc("/api/royal/treasury/oracle", api.RoyalOracleHandler(*dataDir))
	http.HandleFunc("/api/royal/treasury/deflation", api.RoyalDeflationHandler(*dataDir))
	http.HandleFunc("/api/royal/treasury/auto-mint", api.RoyalAutoMintHandler(*dataDir))
	http.HandleFunc("/api/royal/treasury/dividend", api.RoyalDividendHandler(*dataDir))
	http.HandleFunc("/api/royal/treasury/mint", api.RoyalMintHandler(*dataDir))
	http.HandleFunc("/api/royal/treasury/burn", api.RoyalBurnHandler(*dataDir))
	http.HandleFunc("/api/royal/treasury/silver-assets", api.RoyalSilverAssetsHandler(*dataDir))
	http.HandleFunc("/api/royal/treasury/banknotes", api.RoyalBanknotesHandler(*dataDir))
	http.HandleFunc("/api/royal/treasury/proof-of-reserve", api.RoyalProofOfReserveHandler(*dataDir))
	http.HandleFunc("/api/royal/treasury/rates", api.RoyalRatesHandler())
	http.HandleFunc("/api/royal/treasury/tokenomics", api.RoyalTokenomicsHandler())
	http.HandleFunc("/api/royal/treasury/forecast", api.RoyalForecastHandler(*dataDir))
	http.HandleFunc("/api/royal/audit-log", api.RoyalAuditLogHandler(*dataDir))

	// ===== Royal DC Cloud (C2) =====
	http.HandleFunc("/api/royal/dc/status", api.RoyalDCStatusHandler(*dataDir))
	http.HandleFunc("/api/royal/dc/swarm", api.RoyalDCSwarmHandler(*dataDir))
	http.HandleFunc("/api/royal/dc/seed", api.RoyalDCSeedHandler(*dataDir))
	http.HandleFunc("/api/royal/dc/unseed", api.RoyalDCUnseedHandler(*dataDir))
	http.HandleFunc("/api/royal/dc/heal", api.RoyalDCHealHandler(*dataDir))

	// ===== Royal Governance (C4) =====
	http.HandleFunc("/api/royal/governance/constitution", api.RoyalConstitutionHandler(*dataDir))
	http.HandleFunc("/api/royal/governance/proposals", api.RoyalGovernanceProposalsHandler(*dataDir))

	// ===== Royal Relay (C3) =====
	http.HandleFunc("/api/royal/relay", api.RoyalRelayHandler(*dataDir))

	// ===== Royal Chat Bridge (C5) =====
	http.HandleFunc("/api/royal/chat/broadcast", api.RoyalChatBroadcastHandler(*dataDir))
	http.HandleFunc("/api/royal/chat/treasury-alert", api.RoyalTreasuryAlertHandler(*dataDir))

	// ===== Royal UI Dashboard (C21) =====
	http.HandleFunc("/api/royal/ui", api.RoyalDashboardHandler(*dataDir))

	// ===== Royal SSE Events (C22) =====
	http.HandleFunc("/api/royal/events", api.RoyalSSEHandler(*dataDir))

	// ===== Royal Alert Rules (C23) =====
	api.GlobalAlertState = api.LoadAlertRules(*dataDir)
	http.HandleFunc("/api/royal/alerts/list", api.RoyalAlertsListHandler(*dataDir))
	http.HandleFunc("/api/royal/alerts/add", api.RoyalAlertsAddHandler(*dataDir))
	http.HandleFunc("/api/royal/alerts/delete", api.RoyalAlertsDeleteHandler(*dataDir))

	// ===== Royal Health =====
	http.HandleFunc("/api/royal/health", api.RoyalHealthHandler(*dataDir, startTime))

	// ===== Royal Multi-Sig (C24) =====
	http.HandleFunc("/api/royal/multisig", api.RoyalMultiSigHandler(*dataDir))

	// ===== Royal Auto-Cron (C25) =====
	api.GlobalCronScheduler = api.LoadCronRules(*dataDir)
	http.HandleFunc("/api/royal/cron/list", api.RoyalCronListHandler(*dataDir))
	http.HandleFunc("/api/royal/cron/add", api.RoyalCronAddHandler(*dataDir))
	http.HandleFunc("/api/royal/cron/delete", api.RoyalCronDeleteHandler(*dataDir))

	// ===== Inter-Node Sync (C26) =====
	http.HandleFunc("/api/royal/sync", api.RoyalSyncHandler(*dataDir))

	// ===== Scheduled Actions (C28) =====
	http.HandleFunc("/api/royal/schedule/list", api.RoyalScheduleListHandler(*dataDir))
	http.HandleFunc("/api/royal/schedule/create", api.RoyalScheduleCreateHandler(*dataDir))

	// ===== Audit Export (C29) =====
	http.HandleFunc("/api/royal/audit/export", api.RoyalAuditExportHandler(*dataDir))

	// ===== Node Groups (C30) =====
	http.HandleFunc("/api/royal/nodes/groups", api.RoyalNodeGroupsHandler(*dataDir))

	// ===== Emergency Stop (C31) =====
	http.HandleFunc("/api/royal/emergency-stop", api.RoyalEmergencyStopHandler(*dataDir))

	// ===== Node Reputation + Heartbeat (C32) =====
	http.HandleFunc("/api/royal/nodes/reputation", api.RoyalReputationHandler(*dataDir))
	http.HandleFunc("/api/royal/nodes/heartbeat", api.RoyalHeartbeatHandler(*dataDir))

	// ===== Treasury Analytics (C33) =====
	http.HandleFunc("/api/royal/analytics/treasury-trends", api.RoyalAnalyticsHandler(*dataDir))

	// ===== Multi-Currency Reserves (C34) =====
	http.HandleFunc("/api/royal/crypto-reserves", api.RoyalCryptoHandler(*dataDir))

	// ===== Rate Limit Stats (C38) =====
	http.HandleFunc("/api/royal/rate-limit-stats", api.RoyalRateLimitHandler(*dataDir))

	// ===== Test Ping (C40) =====
	http.HandleFunc("/api/royal/test/ping", api.RoyalPingHandler(*dataDir))

	// ===== Documentation API =====
	projectDir := filepath.Join(os.Getenv("HOME"), "ParanoidX")
	api.APIDocsRegisterDefault()
	http.HandleFunc("/api/docs/list", api.DocsListHandler(projectDir))
	http.HandleFunc("/api/docs/download", api.DocsDownloadHandler(projectDir))
	http.HandleFunc("/api/docs/view", api.DocsServeHandler(projectDir))
	http.HandleFunc("/api/docs", api.APIDocsHandler())
	http.HandleFunc("/api/docs/ui", api.APIDocsUIHandler())

	// ===== AI Steward =====
	http.HandleFunc("/api/ai/chat", func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		if r.Method != "POST" {
			http.Error(w, "POST required", 400)
			return
		}
		var req struct {
			Question string `json:"question"`
			Context  string `json:"context,omitempty"`
			Profile  string `json:"profile,omitempty"`
			UserID   string `json:"user_id,omitempty"` // for memory
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad json", 400)
			return
		}
		if req.Question == "" {
			http.Error(w, "question required", http.StatusBadRequest)
			return
		}
		if req.Profile == "" {
			req.Profile = "steward"
		}
		// Use memory if user_id provided
		if req.UserID != "" {
			answer, err := aiSteward.AskWithMemory(req.Question, req.UserID, req.Profile)
			if err != nil {
				slog.Error("ai chat with memory", "error", err)
				http.Error(w, "ai error: "+err.Error(), 500)
				return
			}
			writeJSON(w, map[string]any{"answer": answer, "memory": true})
		} else {
			answer, err := aiSteward.AskWithProfile(req.Question, req.Context, req.Profile)
			if err != nil {
				slog.Error("ai chat", "error", err)
				http.Error(w, "ai error: "+err.Error(), 500)
				return
			}
			writeJSON(w, map[string]any{"answer": answer})
		}
	})

	http.HandleFunc("/api/ai/chat/stream", api.StewardChatStreamHandler(aiSteward))

	http.HandleFunc("/api/ai/explain-silver", func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		answer, err := aiSteward.SilverStandardExplain()
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]any{"answer": answer})
	})

	// Steward AI memory management
	http.HandleFunc("/api/ai/memory/stats", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, aiMemory.Stats())
	})
	http.HandleFunc("/api/ai/memory/clear", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 400)
			return
		}
		var req struct {
			UserID string `json:"user_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad json", 400)
			return
		}
		aiMemory.Clear(req.UserID)
		writeJSON(w, map[string]any{"ok": true, "user_id": req.UserID})
	})

	http.HandleFunc("/api/ai/suggest-treasury", func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		var req struct {
			ReserveNg      int64   `json:"reserve_ng"`
			TotalSupply    int64   `json:"total_supply"`
			RecentDeposits float64 `json:"recent_deposits"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad json", 400)
			return
		}
		suggestion, err := aiSteward.SuggestTreasuryAction(req.ReserveNg, req.TotalSupply, req.RecentDeposits)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]any{"suggestion": suggestion})
	})

	http.HandleFunc("/api/ai/health", func(w http.ResponseWriter, r *http.Request) {
		available := aiClient.IsAvailable()
		writeJSON(w, map[string]any{
			"available":  available,
			"ollama_url": ollamaURL,
			"model":      aiClient.Model,
		})
	})

	http.HandleFunc("/api/ai/economy-summary", func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		var req struct {
			StateJSON string `json:"state_json"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad json", 400)
			return
		}
		summary, err := aiSteward.EconomySummary(req.StateJSON)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]any{"summary": summary})
	})

	// ===== AI Profiles (C54) =====
	http.HandleFunc("/api/ai/profiles", func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		switch r.Method {
		case "GET":
			writeJSON(w, map[string]any{"ok": true, "profiles": aiProfiles.List()})
		case "POST":
			var p ai.PersonalityProfile
			if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
				writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
				return
			}
			if p.ID == "" || p.Name == "" {
				writeJSON(w, map[string]any{"ok": false, "error": "id and name required"})
				return
			}
			aiProfiles.Add(p)
			writeJSON(w, map[string]any{"ok": true, "profile": p})
		default:
			http.Error(w, "GET/POST", 405)
		}
	})
	http.HandleFunc("/api/ai/profiles/update", func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		if r.Method != "PUT" && r.Method != "POST" {
			http.Error(w, "PUT/POST", 405)
			return
		}
		id := r.URL.Query().Get("id")
		if id == "" {
			writeJSON(w, map[string]any{"ok": false, "error": "id required"})
			return
		}
		var p ai.PersonalityProfile
		json.NewDecoder(r.Body).Decode(&p)
		if aiProfiles.Update(id, p) {
			writeJSON(w, map[string]any{"ok": true})
		} else {
			writeJSON(w, map[string]any{"ok": false, "error": "profile not found"})
		}
	})

	// ===== Agent DID (b113) =====
	http.HandleFunc("/api/ai/steward-did", api.StewardDIDHandler())
	// ===== AI Radio (b114) =====
	http.HandleFunc("/api/radio/ai-content", api.AIRadioContentHandler())
	// ===== Treasury Forecast (b115) =====
	http.HandleFunc("/api/economy/treasury-forecast", api.TreasuryForecastHandler(*dataDir))
	// ===== Moderation Stats (b116) =====
	http.HandleFunc("/api/admin/moderation-stats", api.ModerationStatsHandler(*dataDir))

	http.HandleFunc("/api/ai/moderation", func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		var req struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad json", 400)
			return
		}
		if req.Text == "" {
			http.Error(w, "text required", http.StatusBadRequest)
			return
		}
		safe, reason, err := aiSteward.ModerationCheck(req.Text)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]any{
			"safe":   safe,
			"reason": reason,
		})
	})

	// ===== Role Chat =====
	http.HandleFunc("/api/set_role_chat", func(w http.ResponseWriter, r *http.Request) {
		role := r.URL.Query().Get("role")
		chat := r.URL.Query().Get("chat")
		if role == "" || chat == "" {
			http.Error(w, "role and chat required", http.StatusBadRequest)
			return
		}
		rolesMu.Lock()
		knownRoles[role] = chat
		b, err := json.MarshalIndent(knownRoles, "", "  ")
		if err != nil {
			rolesMu.Unlock()
			http.Error(w, "marshal error", 500)
			return
		}
		if err := fileutil.WriteFile(rolesFile, b, 0600); err != nil {
			rolesMu.Unlock()
			http.Error(w, "write error", 500)
			return
		}
		rolesMu.Unlock()
		writeJSON(w, map[string]any{"ok": true, "role": role})
	})

	http.HandleFunc("/api/send_to_role", func(w http.ResponseWriter, r *http.Request) {
		role := r.URL.Query().Get("role")
		text := r.URL.Query().Get("text")
		if role == "" || text == "" {
			http.Error(w, "role and text required", http.StatusBadRequest)
			return
		}
		sendToIslandRole(role, text)
		writeJSON(w, map[string]any{"ok": true})
	})

	// ===== A2: Economy API =====
	http.HandleFunc("/api/economy/state", func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		ledger := economy.LoadLedger(*dataDir)
		banknotes, _ := economy.LoadBanknotesV2(*dataDir)
		preMint := economy.LoadPreMint(*dataDir)
		debt := economy.LoadDebt(*dataDir)
		auditors := economy.LoadAuditors(*dataDir)
		burned := economy.LoadBurnedSerials(*dataDir)
		reserveNg := int64(0)
		if b, err := os.ReadFile(filepath.Join(*dataDir, "silver_reserve_ng.txt")); err == nil {
			s := strings.TrimSpace(string(b))
			for i := range s {
				if s[i] < '0' || s[i] > '9' {
					s = s[:i]
					break
				}
			}
			fmt.Sscanf(s, "%d", &reserveNg)
		}
		activeCount := 0
		for _, b := range banknotes {
			if b.Status == "active" {
				activeCount++
			}
		}
		preMintAvailable := 0
		for _, p := range preMint {
			if p.Status == "available" {
				preMintAvailable++
			}
		}
		writeJSON(w, map[string]any{
			"total_supply_ng":    ledger.TotalSupply,
			"accounts":           len(ledger.Accounts),
			"reserve_ng":         reserveNg,
			"banknotes_active":   activeCount,
			"banknotes_total":    len(banknotes),
			"pre_mint_available": preMintAvailable,
			"burned_serials":     len(burned),
			"debt_investor":      debt.RepaidAt == "",
			"auditors":           len(auditors),
			"version":            "A2-ledger-v2",
		})
	})

	http.HandleFunc("/api/wallet/create", func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		pubkey, privkey, mnemonic, err := economy.GenerateKeypair()
		if err != nil {
			http.Error(w, "generate: "+err.Error(), 500)
			return
		}
		ledger := economy.LoadLedger(*dataDir)
		ledger.EnsureAccount(pubkey)
		ledger.Save(*dataDir)
		writeJSON(w, map[string]any{
			"pubkey":   pubkey,
			"privkey":  privkey,
			"mnemonic": mnemonic,
			"warning":  "Сохрани сид-фразу! Никому не показывай приватный ключ!",
		})
	})

	http.HandleFunc("/api/wallet/balance", func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		pubkey := r.URL.Query().Get("pubkey")
		if pubkey == "" {
			http.Error(w, "pubkey required", http.StatusBadRequest)
			return
		}
		ledger := economy.LoadLedger(*dataDir)
		bal := ledger.Balance(pubkey)
		banknotes, _ := economy.LoadBanknotesV2(*dataDir)
		myBanknotes := economy.GetHolderBanknotes(banknotes, pubkey)
		frozenTotal := int64(0)
		for _, b := range myBanknotes {
			frozenTotal += b.FrozenNg
		}
		writeJSON(w, map[string]any{
			"pubkey":            pubkey,
			"liquid_balance_ng": bal,
			"liquid_balance_tlr": economy.NGtoTLR(bal),
			"frozen_ng":         frozenTotal,
			"frozen_tlr":        economy.NGtoTLR(frozenTotal),
			"banknotes_count":   len(myBanknotes),
		})
	})

	http.HandleFunc("/api/economy/holdings", func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		pubkey := r.URL.Query().Get("pubkey")
		if pubkey == "" {
			http.Error(w, "pubkey required", http.StatusBadRequest)
			return
		}
		ledger := economy.LoadLedger(*dataDir)
		banknotes, _ := economy.LoadBanknotesV2(*dataDir)
		myBanknotes := economy.GetHolderBanknotes(banknotes, pubkey)
		inventory := economy.LoadInventory(*dataDir, pubkey)
		bal := ledger.Balance(pubkey)
		frozenTotal := int64(0)
		divHistory := []map[string]any{}
		for _, b := range myBanknotes {
			frozenTotal += b.FrozenNg
			for _, d := range b.DividendHistory {
				divHistory = append(divHistory, map[string]any{
					"serial": b.Serial,
					"round":  d.Round,
					"ng":     d.Ng,
					"date":   d.Date,
				})
			}
		}
		writeJSON(w, map[string]any{
			"pubkey":          pubkey,
			"liquid_ng":       bal,
			"liquid_tlr":      economy.NGtoTLR(bal),
			"frozen_ng":       frozenTotal,
			"frozen_tlr":      economy.NGtoTLR(frozenTotal),
			"banknotes":       myBanknotes,
			"inventory_packs": inventory,
			"dividends":       divHistory,
		})
	})

	http.HandleFunc("/api/economy/pre-mint", func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		preMint := economy.LoadPreMint(*dataDir)
		available := []economy.PreMintEntry{}
		reserved := []economy.PreMintEntry{}
		for _, p := range preMint {
			if p.Status == "available" {
				available = append(available, p)
			} else if p.Status == "genesis_reserved" {
				reserved = append(reserved, p)
			}
		}
		writeJSON(w, map[string]any{
			"available":      available,
			"reserved":       reserved,
			"total_available": len(available),
			"total_reserved": len(reserved),
		})
	})

	// ===== Auditor =====
	http.HandleFunc("/api/auditor/grant", func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		if !isRoyalNode(*dataDir) {
			http.Error(w, "Only royal node can grant auditor", http.StatusForbidden)
			return
		}
		defer r.Body.Close()
		var req struct {
			Pubkey string `json:"pubkey"`
			Role   string `json:"role"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad json", 400)
			return
		}
		if req.Pubkey == "" {
			http.Error(w, "pubkey required", 400)
			return
		}
		auditors := economy.LoadAuditors(*dataDir)
		auditors = append(auditors, economy.AuditorEntry{
			Pubkey:    req.Pubkey,
			Label:     req.Role,
			GrantedAt: time.Now().Format(time.RFC3339),
			Type:      "manual",
		})
		economy.SaveAuditors(*dataDir, auditors)
		writeJSON(w, map[string]any{"ok": true, "pubkey": req.Pubkey, "role": req.Role})
	})

	http.HandleFunc("/api/auditor/list", func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		auditors := economy.LoadAuditors(*dataDir)
		writeJSON(w, map[string]any{"count": len(auditors), "auditors": auditors})
	})

	http.HandleFunc("/api/auditor/refresh", func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		pubkey := r.URL.Query().Get("pubkey")
		if pubkey == "" {
			http.Error(w, "pubkey required", 400)
			return
		}
		auditors := economy.RefreshTopAuditors(*dataDir)
		writeJSON(w, map[string]any{"ok": true, "pubkey": pubkey, "count": len(auditors)})
	})

	http.HandleFunc("/auditor", func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		candidates := []string{
			filepath.Join(*dataDir, "auditor-dashboard.html"),
			filepath.Join(filepath.Dir(os.Args[0]), "auditor-dashboard.html"),
			"/home/tomas/ParanoidX/docker/auditor-dashboard.html",
		}
		bestMtime := time.Time{}
		var bestBytes []byte
		for _, p := range candidates {
			if st, err := os.Stat(p); err == nil {
				if b, err := os.ReadFile(p); err == nil && st.ModTime().After(bestMtime) {
					bestMtime = st.ModTime()
					bestBytes = b
				}
			}
		}
		if len(bestBytes) > 0 {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
			w.Write(bestBytes)
			return
		}
		http.Error(w, "Auditor dashboard not found", 404)
	})

	http.HandleFunc("/api/p2p/explore", func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		pubkey := r.URL.Query().Get("pubkey")
		if pubkey == "" {
			http.Error(w, "pubkey required", http.StatusBadRequest)
			return
		}
		if !economy.IsAuditor(*dataDir, pubkey) {
			http.Error(w, "только аудиторы могут просматривать держателей", http.StatusForbidden)
			return
		}
		holders := economy.ExploreHolders(*dataDir)
		writeJSON(w, map[string]any{"count": len(holders), "holders": holders})
	})

	// ===== Genesis =====
	http.HandleFunc("/api/genesis/info", func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		banknotes, _ := economy.LoadBanknotesV2(*dataDir)
		var genesisCards []map[string]any
		for _, b := range banknotes {
			if b.Rarity == "genesis" {
				genesisCards = append(genesisCards, map[string]any{
					"serial":            b.Serial,
					"denomination_ng":   b.DenominationNg,
					"denomination_tlr":  float64(b.DenominationNg) / float64(economy.NGPerTLR),
					"rarity":            b.Rarity,
					"name":              b.SpecialSeries,
					"status":            b.Status,
					"locked":            b.Status == "genesis_locked",
				})
			}
		}
		announcement := ""
		if b, err := os.ReadFile(filepath.Join(*dataDir, "genesis_announcement.txt")); err == nil {
			announcement = string(b)
		}
		writeJSON(w, map[string]any{"has_genesis": len(genesisCards) > 0, "count": len(genesisCards), "cards": genesisCards, "announcement": announcement})
	})

	// ===== Packs =====
	auctionMgr = economy.NewAuctionManager(*dataDir)

	http.HandleFunc("/api/pack/buy", func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		if r.Method != "POST" {
			http.Error(w, "POST required", 400)
			return
		}
		pubkey := r.URL.Query().Get("pubkey")
		if pubkey == "" {
			http.Error(w, "pubkey required", 400)
			return
		}
		preMint := economy.LoadPreMint(*dataDir)
		pack, err := packMgr.CreateBoosterPack(*dataDir, pubkey, preMint)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		ledger := economy.LoadLedger(*dataDir)
		if ledger.Balance(pubkey) < pack.PriceNg {
			http.Error(w, "недостаточно Liquid Taler", 400)
			return
		}
		ledger.Transfer(pubkey, "treasury", pack.PriceNg)
		ledger.Save(*dataDir)
		writeJSON(w, map[string]any{"pack_id": pack.PackID, "price_ng": pack.PriceNg, "price_tlr": economy.NGtoTLR(pack.PriceNg), "sealed": true, "banknote_count": len(pack.Banknotes)})
	})

	http.HandleFunc("/api/pack/open", func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		if r.Method != "POST" {
			http.Error(w, "POST required", 400)
			return
		}
		pubkey := r.URL.Query().Get("pubkey")
		packID := r.URL.Query().Get("pack_id")
		if pubkey == "" || packID == "" {
			http.Error(w, "pubkey and pack_id required", 400)
			return
		}
		pack, opened, err := packMgr.OpenPack(*dataDir, pubkey, packID)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		type cardInfo struct {
			Serial   string `json:"serial"`
			DenomNG  int64  `json:"denomination_ng"`
			Rarity   string `json:"rarity"`
			FrozenNG int64  `json:"frozen_ng"`
		}
		var cards []cardInfo
		for _, b := range opened {
			cards = append(cards, cardInfo{Serial: b.Serial, DenomNG: b.DenominationNg, Rarity: b.Rarity, FrozenNG: b.FrozenNg})
		}
		writeJSON(w, map[string]any{"pack_id": pack.PackID, "opened": true, "cards": cards})
	})

	http.HandleFunc("/api/pack/list", func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		pubkey := r.URL.Query().Get("pubkey")
		if pubkey == "" {
			http.Error(w, "pubkey required", 400)
			return
		}
		packs := packMgr.GetUserPacks(*dataDir, pubkey)
		writeJSON(w, map[string]any{"pubkey": pubkey, "packs": packs})
	})

	// ===== Buyback =====
	http.HandleFunc("/api/buyback/quote", func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		serial := r.URL.Query().Get("serial")
		if serial == "" {
			http.Error(w, "serial required", 400)
			return
		}
		banknotes, _ := economy.LoadBanknotesV2(*dataDir)
		for _, b := range banknotes {
			if b.Serial == serial {
				writeJSON(w, buybackMgr.Quote(b))
				return
			}
		}
		preMint := economy.LoadPreMint(*dataDir)
		for _, p := range preMint {
			if p.Serial == serial {
				writeJSON(w, buybackMgr.Quote(economy.BanknoteV2{
					Serial:         p.Serial,
					DenominationNg: p.DenominationNg,
					Rarity:         p.Rarity,
				}))
				return
			}
		}
		http.Error(w, "banknote not found", 404)
	})

	http.HandleFunc("/api/buyback/sell", func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		if r.Method != "POST" {
			http.Error(w, "POST required", 400)
			return
		}
		serial := r.URL.Query().Get("serial")
		pubkey := r.URL.Query().Get("pubkey")
		if serial == "" || pubkey == "" {
			http.Error(w, "serial and pubkey required", 400)
			return
		}
		banknotes, _ := economy.LoadBanknotesV2(*dataDir)
		var target *economy.BanknoteV2
		for _, b := range banknotes {
			if b.Serial == serial && b.Holder == pubkey && b.Status == "active" {
				target = &b
				break
			}
		}
		if target == nil {
			http.Error(w, "banknote not found or not yours", 404)
			return
		}
		record, err := buybackMgr.Execute(*dataDir, *target, pubkey)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		writeJSON(w, map[string]any{"serial": record.Serial, "price_ng": record.PriceNG, "price_tlr": economy.NGtoTLR(record.PriceNG), "re_mint_serial": record.ReMintSerial, "burned_at": record.BurnedAt})
	})

	// ===== Auction =====
	http.HandleFunc("/api/auction/active", func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		active := auctionMgr.GetActive()
		writeJSON(w, map[string]any{"count": len(active), "auctions": active})
	})

	http.HandleFunc("/api/auction/list", func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		if r.Method != "POST" {
			http.Error(w, "POST required", 400)
			return
		}
		pubkey := r.URL.Query().Get("pubkey")
		serial := r.URL.Query().Get("serial")
		startPrice := r.URL.Query().Get("start_price")
		if pubkey == "" || serial == "" || startPrice == "" {
			http.Error(w, "pubkey, serial, start_price required", 400)
			return
		}
		var startPriceNG int64
		fmt.Sscanf(startPrice, "%d", &startPriceNG)
		if startPriceNG <= 0 {
			http.Error(w, "invalid start_price", 400)
			return
		}
		banknotes, _ := economy.LoadBanknotesV2(*dataDir)
		var target economy.BanknoteV2
		found := false
		for _, b := range banknotes {
			if b.Serial == serial && b.Holder == pubkey && b.Status == "active" {
				target = b
				found = true
				break
			}
		}
		if !found {
			http.Error(w, "banknote not found or not yours", 404)
			return
		}
		auction, err := auctionMgr.List(target, pubkey, startPriceNG, 27*time.Hour)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		writeJSON(w, map[string]any{"listing_id": auction.ListingID, "serial": auction.Serial, "start_price": auction.StartPriceNG, "ends_at": auction.EndsAt.Format(time.RFC3339)})
	})

	http.HandleFunc("/api/auction/bid", func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		if r.Method != "POST" {
			http.Error(w, "POST required", 400)
			return
		}
		listingID := r.URL.Query().Get("listing_id")
		pubkey := r.URL.Query().Get("pubkey")
		bid := r.URL.Query().Get("bid")
		if listingID == "" || pubkey == "" || bid == "" {
			http.Error(w, "listing_id, pubkey, bid required", 400)
			return
		}
		var bidNG int64
		fmt.Sscanf(bid, "%d", &bidNG)
		if bidNG <= 0 {
			http.Error(w, "invalid bid", 400)
			return
		}
		if err := auctionMgr.Bid(listingID, pubkey, bidNG); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "listing_id": listingID, "bid_ng": bidNG})
	})

	http.HandleFunc("/api/auction/my", func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		pubkey := r.URL.Query().Get("pubkey")
		if pubkey == "" {
			http.Error(w, "pubkey required", 400)
			return
		}
		writeJSON(w, map[string]any{"pubkey": pubkey, "count": len(auctionMgr.GetMyListings(pubkey)), "items": auctionMgr.GetMyListings(pubkey)})
	})

	monitor := health.New(*dataDir, vaultSvc.Path, startTime)
	monitor.AlertURL = "http://127.0.0.1:5002/send_alert"

	// Disk Alert Manager (C56)
	health.GlobalDiskAlertManager = health.NewDiskAlertManager(*dataDir, fmt.Sprintf("http://127.0.0.1:5002/send_alert"))
	health.GlobalDiskAlertManager.Start()

	http.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		uptime := time.Since(startTime).Hours()
		report := monitor.Report()
		rpt := map[string]any{
			"healthy":        report.Healthy,
			"uptime_hours":   float64(int(uptime*10)) / 10,
			"bridge":         api.BridgeConnected,
			"build":          fmt.Sprintf("px-node-%s", buildVersion),
			"messages":       chatHub.MessageCount(),
			"dc_cloud":       len(dcCloud.ListContainers()),
			"dc_seeding":     len(dcCloud.SeedingStatus()),
		}
		if !report.Healthy {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		writeJSON(w, rpt)
	})

	// Detailed health checks endpoint
	http.HandleFunc("/api/health/checks", func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		report := monitor.Report()
		writeJSON(w, report)
	})

	// SSE endpoint streaming current monitor status
	var (
		monitorStatusMu    sync.Mutex
		consecutiveFails   = 0
		lastMonitorPoll    = time.Now()
	)
	http.HandleFunc("/api/admin/monitor-status", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				report := monitor.Report()

				status := "healthy"
				if !report.Healthy {
					status = "degraded"
				}

				monitorStatusMu.Lock()
				if status == "degraded" {
					consecutiveFails++
				} else {
					consecutiveFails = 0
				}
				lastMonitorPoll = time.Now()
				mcFails := consecutiveFails
				monitorStatusMu.Unlock()

				data := map[string]any{
					"status":               status,
					"bridge_connected":     api.BridgeConnected,
					"healthy":              report.Healthy,
					"last_poll":            lastMonitorPoll.Format(time.RFC3339),
					"consecutive_failures": mcFails,
					"uptime_hours":         fmt.Sprintf("%.1f", time.Since(startTime).Hours()),
					"checks":               report.Checks,
				}

				b, _ := json.Marshal(data)
				fmt.Fprintf(w, "data: %s\n\n", b)
				flusher.Flush()
			}
		}
	})

	// ===== Prometheus /metrics endpoint =====
	http.HandleFunc("/api/admin/metrics/prometheus", api.PrometheusMetricsHandler())

	slog.Info("listening", "addr", *listen, "data", *dataDir)

	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for {
			report := monitor.Report()
			monitor.AlertIfFailing(report)
			<-ticker.C
		}
	}()

	// Treasury analytics snapshot (C33) — every 5 minutes
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		api.RecordRoyalSnap(*dataDir)
		for range ticker.C {
			api.RecordRoyalSnap(*dataDir)
		}
	}()

	// ===== SimpleX Wallet Bot (bridge to CLI) =====
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		b := bridge.New(*dataDir)
		slog.Info("simplex wallet bot starting")
		b.RunContext(ctx)
		slog.Warn("simplex wallet bot stopped")
	}()

	handler := api.PerfMiddleware(middleware.SecurityMiddleware(http.DefaultServeMux))
	srv := &http.Server{Addr: *listen, Handler: handler}

	// Persistence save on shutdown
	api.GlobalChatHub.LoadPersisted(*dataDir)

	// Graceful shutdown on SIGTERM/SIGINT
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	dcCloud.Start()

	go func() {
		sig := <-sigCh
		slog.Info("shutdown", "signal", sig)
		cancel()
		dcCloud.Stop()
		api.GlobalChatHub.ClearMessages()
		api.GlobalChatHub.SaveAll(*dataDir)
		ctxShut, clear := context.WithTimeout(context.Background(), 5*time.Second)
		defer clear()
		srv.Shutdown(ctxShut)
	}()

	slog.Info("serving", "addr", srv.Addr, "data", *dataDir)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("listen failed", "error", err)
	}
	cancel()
	slog.Info("shutdown complete")
}

func getAvailMem() uint64 {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "MemAvailable:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				v, _ := strconv.ParseUint(fields[1], 10, 64)
				return v
			}
		}
	}
	return 0
}


