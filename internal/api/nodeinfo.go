// Package api provides HTTP handlers and API endpoints for the simplex-node server
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"ParanoidX/internal/economy"
	"ParanoidX/internal/middleware"
)

// ServiceStatus represents the health state of a single service.
type ServiceStatus struct {
	Healthy bool   `json:"healthy"`
	Detail  string `json:"detail,omitempty"`
}

// NodeInfoResponse aggregates all node information for /api/admin/info.
type NodeInfoResponse struct {
	Version     string                  `json:"version"`
	Build       string                  `json:"build"`
	Uptime      string                  `json:"uptime"`
	Started     string                  `json:"started"`
	GoVersion   string                  `json:"go_version"`
	CPU         int                     `json:"cpus"`
	Goroutines  int                     `json:"goroutines"`
	DataDir     string                  `json:"data_dir"`
	ListenAddr  string                  `json:"listen_addr"`

	Services    map[string]ServiceStatus `json:"services"`
	Docker      map[string]string        `json:"docker,omitempty"`
	Economy     *EconomySnapshot         `json:"economy,omitempty"`
	Chat        *ChatSnapshot            `json:"chat,omitempty"`
	Radio       *RadioSnapshot           `json:"radio,omitempty"`
	Silver      *SilverSnapshot          `json:"silver,omitempty"`
	Container   *ContainerSnapshot       `json:"container,omitempty"`
	Treasury    *TreasurySnapshot        `json:"treasury,omitempty"`

	Routes      []string                 `json:"routes,omitempty"`
	Timestamp   string                   `json:"timestamp"`
}

// EconomySnapshot captures the current state of the on-chain economy.
type EconomySnapshot struct {
	TotalSupplyNg int64 `json:"total_supply_ng"`
	Accounts      int   `json:"accounts"`
	ReserveNg     int64 `json:"reserve_ng"`
	Banknotes     int   `json:"banknotes_active"`
	PreMint       int   `json:"pre_mint_available"`
	DividendPool  int64 `json:"dividend_pool"`
	Auditors      int   `json:"auditors"`
}

// ChatSnapshot captures the current state of the chat system.
type ChatSnapshot struct {
	Messages       int  `json:"messages"`
	BridgeConnected bool `json:"bridge_connected"`
	SSEClients     int  `json:"sse_clients"`
	Reconnects     int64 `json:"bridge_reconnects"`
	Contacts       int  `json:"contacts"`
}

// RadioSnapshot captures the current state of the radio system.
type RadioSnapshot struct {
	Stations int `json:"stations"`
	Tracks   int `json:"tracks"`
}

// SilverSnapshot captures the current silver spot price and source.
type SilverSnapshot struct {
	PriceUSD float64 `json:"price_usd"`
	Source   string  `json:"source"`
	Updated  string  `json:"updated"`
}

// ContainerSnapshot captures the current state of the crypto container.
type ContainerSnapshot struct {
	Exists bool   `json:"exists"`
	Open   bool   `json:"open"`
	Files  int    `json:"files"`
}

// TreasurySnapshot captures the current treasury balance and deposit count.
type TreasurySnapshot struct {
	TotalUSDT   float64 `json:"total_usdt"`
	Deposits    int     `json:"deposit_count"`
}

var (
	nodeInfoMu     sync.RWMutex
	nodeInfoCache  *NodeInfoResponse
	nodeInfoCached time.Time
)


// NodeInfoHandler handles the NodeInfoHandler HTTP request.
// NodeInfoHandler returns comprehensive node information including services, economy, chat, radio, and Docker status.
func NodeInfoHandler(dataDir, listenAddr, version string, startTime time.Time) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}

		nodeInfoMu.RLock()
		cached := nodeInfoCache
		cachedTime := nodeInfoCached
		nodeInfoMu.RUnlock()

		if cached != nil && time.Since(cachedTime) < 5*time.Second {
			writeJSON(w, cached)
			return
		}

		info := collectNodeInfo(dataDir, listenAddr, version, startTime)

		nodeInfoMu.Lock()
		nodeInfoCache = info
		nodeInfoCached = time.Now()
		nodeInfoMu.Unlock()

		writeJSON(w, info)
	}
}

func getRoutes() []string {
	return []string{
		"/api/version", "/api/status", "/api/health", "/api/addresses", "/api/disk-check",
		"/api/lock-status", "/api/lock", "/api/unlock", "/api/change-lock-code", "/api/rotate",
		"/api/dashboard-onion", "/api/ice-config", "/api/call-signal",
		"/api/metrics", "/api/restart",
		// Vault
		"/api/vault/list", "/api/vault/upload", "/api/vault/download", "/api/vault/delete",
		"/api/vault/save-note", "/api/vault/encrypt", "/api/vault/decrypt", "/api/vault/audio",
		// Treasury
		"/api/treasury/usdt-deposits", "/api/treasury/init-silver-round", "/api/treasury/claim-dividends",
		"/api/treasury/auto-round", "/api/treasury/register-banknote", "/api/treasury/simulate-deposit",
		"/api/treasury/state", "/api/treasury/proof-of-reserve",
		// Economy
		"/api/economy/state", "/api/economy/holdings", "/api/economy/wheel", "/api/economy/auto-mint",
		"/api/economy/crafting", "/api/economy/reinvest", "/api/economy/oracle", "/api/economy/deflate",
		"/api/economy/tokenomics", "/api/economy/onboarding",
		"/api/subscription",
		// Chat
		"/api/chat/history", "/api/chat/stream", "/api/chat/send", "/api/chat/clear", "/api/chat/delete",
		"/api/chat/delete/contact", "/api/chat/address", "/api/chat/address/create",
		"/api/chat/contacts", "/api/chat/contact", "/api/chat/connect", "/api/chat/qr",
		"/api/chat/status", "/api/chat/edit", "/api/chat/search", "/api/chat/contact/alias",
		"/api/chat/stats", "/api/chat/backup", "/api/chat/export", "/api/chat/contact/info",
		"/api/chat/clear-old", "/api/chat/pin", "/api/chat/react", "/api/chat/server-status",
		"/api/chat/broadcast", "/api/chat/last-message", "/api/chat/typing", "/api/chat/schedule",
		"/api/chat/auto-reply", "/api/chat/groups", "/api/chat/labels", "/api/chat/drafts",
		"/api/chat/webhook", "/api/chat/search/advanced", "/api/chat/templates",
		"/api/chat/analytics", "/api/chat/batch-forward", "/api/chat/auto-delete",
		"/api/inquisitor/report",
		// Invoice
		"/api/chat/invoice/create", "/api/chat/invoice/list", "/api/chat/invoice/pay",
		"/api/chat/invoice/stats", "/api/chat/invoice/export-csv",
		// Admin
		"/api/admin/audit-log", "/api/admin/metrics", "/api/admin/diagnostics",
		"/api/admin/status-page", "/api/admin/rate-limit-status", "/api/admin/content-filter",
		"/api/admin/docker", "/api/admin/metrics/system", "/api/admin/backup",
		"/api/admin/search-index",
		// Container
		"/api/container/init", "/api/container/open", "/api/container/close",
		"/api/container/status", "/api/panic",
		// Radio
		"/api/radio", "/api/radio/track", "/api/radio/stream", "/api/radio/acestep",
		// P2P
		"/api/node/announce", "/api/node/discover", "/api/node/heartbeat", "/api/node/list",
		"/api/node/status",
		"/api/tracker/announce", "/api/tracker/scrape", "/api/tracker/nodes",
		// Wallet
		"/api/wallet/create", "/api/wallet/balance", "/api/wallet/send", "/api/wallet/receive",
		"/api/wallet/history",
		// Account
		"/api/account/create", "/api/account/restore", "/api/account/verify",
		// Swap / Bridge
		"/api/swap/create", "/api/swap/claim", "/api/swap/refund", "/api/swap/list",
		"/api/bridge/create", "/api/bridge/confirm", "/api/bridge/complete", "/api/bridge/list",
		// Market
		"/api/market/list", "/api/market/sell", "/api/market/buy",
		"/api/escrow/create", "/api/escrow/release", "/api/escrow/cancel", "/api/escrow/list",
		"/api/escrow/buy", "/api/escrow/auto-resolve",
		// POS
		"/api/pos", "/api/pos/qr",
		// RWA
		"/api/rwa/register", "/api/rwa/list",
		// Royal
		"/api/royal/register", "/api/royal/nodes", "/api/royal/command", "/api/royal/heartbeat",
		"/api/royal/key",
		// Channels
		"/api/channels/list", "/api/channels/create", "/api/channels/access",
		"/api/channels/view", "/api/channels/post", "/api/channels/posts",
		// AI
		"/api/ai/chat", "/api/ai/health", "/api/ai/moderation", "/api/ai/explain-silver",
		"/api/ai/suggest-treasury", "/api/ai/economy-summary",
		"/api/steward", "/api/ai/constitution", "/api/ai/monitor",
		// Docs
		"/api/docs/list", "/api/docs/download", "/api/docs/view",
		// Franchise
		"/api/franchise/licenses", "/api/franchise/earmarks", "/api/franchise/mint-auth",
		"/api/franchise/templates", "/api/franchise/settlements", "/api/franchise/royalties",
		// Services
		"/api/services/registry", "/api/services/marketplace",
		// Transport
		"/api/transport/info", "/api/transport/health", "/api/transport/status", "/api/transport/send",
		// ICO
		"/api/ico/info", "/api/ico/invest", "/api/ico/status",
		// Genesis
		"/api/genesis/ico", "/api/genesis/lock", "/api/genesis/info",
		// Mining
		"/api/mining", "/api/argentum",
		// Advertising
		"/api/advertising",
		// Audit
		"/api/auditor/grant", "/api/auditor/list", "/api/auditor/refresh",
		// Billing
		"/api/billing/prices", "/api/billing/payments",
		// Roles
		"/api/set_role_chat", "/api/send_to_role",
		// P2P explore
		"/api/p2p/explore",
	}
}

func collectNodeInfo(dataDir, listenAddr, version string, startTime time.Time) *NodeInfoResponse {
	now := time.Now()
	uptime := now.Sub(startTime)

	svcs := map[string]ServiceStatus{}

	svcs["server"] = ServiceStatus{Healthy: true, Detail: "listening on " + listenAddr}

	bridgeOK := BridgeConnected
	if bridgeOK {
		svcs["bridge"] = ServiceStatus{Healthy: true, Detail: fmt.Sprintf("connected, %d reconnects", BridgeReconnectCount)}
	} else {
		svcs["bridge"] = ServiceStatus{Healthy: false, Detail: "disconnected"}
	}

	if SimplexCmd != nil {
		svcs["simplex_chat"] = ServiceStatus{Healthy: true, Detail: "CLI connected"}
	} else {
		svcs["simplex_chat"] = ServiceStatus{Healthy: false, Detail: "not connected"}
	}

	if dumb := checkDockerRunning(); len(dumb) > 0 {
		allUp := true
		for _, st := range dumb {
			if !strings.Contains(st, "Up") {
				allUp = false
				break
			}
		}
		if allUp {
			svcs["docker"] = ServiceStatus{Healthy: true, Detail: fmt.Sprintf("%d containers running", len(dumb))}
		} else {
			svcs["docker"] = ServiceStatus{Healthy: false, Detail: "some containers not healthy"}
		}
	} else {
		svcs["docker"] = ServiceStatus{Healthy: false, Detail: "docker not available"}
	}

	radioDir := filepath.Join(dataDir, "radio")
	if fi, err := os.Stat(radioDir); err == nil && fi.IsDir() {
		svcs["radio"] = ServiceStatus{Healthy: true, Detail: radioDir}
	} else {
		svcs["radio"] = ServiceStatus{Healthy: false, Detail: "radio dir not found"}
	}

	if GlobalContainer != nil {
		exists := GlobalContainer.HasContainer()
		if exists {
			open := GlobalContainer.IsOpen()
			files := len(GlobalContainer.List())
			if open {
				svcs["crypto_container"] = ServiceStatus{Healthy: true, Detail: fmt.Sprintf("open, %d files", files)}
			} else {
				svcs["crypto_container"] = ServiceStatus{Healthy: false, Detail: fmt.Sprintf("locked, %d files", files)}
			}
		} else {
			svcs["crypto_container"] = ServiceStatus{Healthy: false, Detail: "not initialized"}
		}
	} else {
		svcs["crypto_container"] = ServiceStatus{Healthy: false, Detail: "not available"}
	}

	p2pPort := 17001
	if p := os.Getenv("P2P_PORT"); p != "" {
		if n, err := strconv.Atoi(p); err == nil {
			p2pPort = n
		}
	}
	svcs["p2p_transport"] = ServiceStatus{Healthy: true, Detail: fmt.Sprintf("port %d", p2pPort)}

	var eco *EconomySnapshot
	if ledger := economy.LoadLedger(dataDir); ledger != nil {
		banknotes, _ := economy.LoadBanknotesV2(dataDir)
		activeCount := 0
		for _, b := range banknotes {
			if b.Status == "active" {
				activeCount++
			}
		}
		preMint := economy.LoadPreMint(dataDir)
		preAvail := 0
		for _, p := range preMint {
			if p.Status == "available" {
				preAvail++
			}
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
		auditors := economy.LoadAuditors(dataDir)
		pool := ledger.Balance("dividend_pool")
		eco = &EconomySnapshot{
			TotalSupplyNg: ledger.TotalSupply,
			Accounts:      len(ledger.Accounts),
			ReserveNg:     reserveNg,
			Banknotes:     activeCount,
			PreMint:       preAvail,
			DividendPool:  pool,
			Auditors:      len(auditors),
		}
	}

	var chatSnap *ChatSnapshot
	if GlobalChatHub != nil {
		contactCount := 0
		if SimplexCmd != nil && BridgeConnected {
			resp, err := SimplexCmd("/_contacts 1")
			if err == nil {
				rType, _ := resp["type"].(string)
				if rType == "contactsList" {
					contacts, _ := resp["contacts"].([]any)
					contactCount = len(contacts)
				}
			}
		}
		chatSnap = &ChatSnapshot{
			Messages:        GlobalChatHub.MessageCount(),
			BridgeConnected: BridgeConnected,
			SSEClients:      GlobalChatHub.SSEClientCount(),
			Reconnects:      BridgeReconnectCount,
			Contacts:        contactCount,
		}
	}

	var radioSnap *RadioSnapshot
	radioDataDir := filepath.Join(dataDir, "radio")
	if entries, err := os.ReadDir(radioDataDir); err == nil {
		stations := 0
		tracks := 0
		for _, e := range entries {
			if e.IsDir() {
				stations++
				if trackEntries, err := os.ReadDir(filepath.Join(radioDataDir, e.Name())); err == nil {
					tracks += len(trackEntries)
				}
			}
		}
		radioSnap = &RadioSnapshot{Stations: stations, Tracks: tracks}
	}

	var silverSnap *SilverSnapshot
	if oraclePrice := getSilverPrice(dataDir); oraclePrice > 0 {
		silverSnap = &SilverSnapshot{
			PriceUSD: oraclePrice,
			Source:   "oracle",
			Updated:  now.Format(time.RFC3339),
		}
	}

	var containerSnap *ContainerSnapshot
	if GlobalContainer != nil {
		containerSnap = &ContainerSnapshot{
			Exists: GlobalContainer.HasContainer(),
			Open:   GlobalContainer.IsOpen(),
			Files:  len(GlobalContainer.List()),
		}
	}

	dockerMap := checkDockerRunning()

	info := &NodeInfoResponse{
		Version:    version,
		Build:      fmt.Sprintf("px-node-%s", version),
		Uptime:     fmt.Sprintf("%dh%dm", int(uptime.Hours()), int(uptime.Minutes())%60),
		Started:    startTime.Format(time.RFC3339),
		GoVersion:  runtime.Version(),
		CPU:        runtime.NumCPU(),
		Goroutines: runtime.NumGoroutine(),
		DataDir:    dataDir,
		ListenAddr: listenAddr,
		Services:   svcs,
		Docker:     dockerMap,
		Economy:    eco,
		Chat:       chatSnap,
		Radio:      radioSnap,
		Silver:     silverSnap,
		Container:  containerSnap,
		Routes:     getRoutes(),
		Timestamp:  now.Format(time.RFC3339),
	}

	return info
}

func checkDockerRunning() map[string]string {
	composeDir := os.Getenv("SIMPLEX_SRC")
	if composeDir == "" {
		composeDir = filepath.Join(os.Getenv("HOME"), "simplex-node")
	}
	composeDir = filepath.Join(composeDir, "docker")
	cmd := exec.Command("docker", "compose", "ps", "--format", "{{.Name}}\t{{.Status}}")
	cmd.Dir = composeDir
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	containers := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) == 2 {
			containers[parts[0]] = parts[1]
		}
	}
	return containers
}

func getSilverPrice(dataDir string) float64 {
	b, err := os.ReadFile(filepath.Join(dataDir, "silver_spot_usd.txt"))
	if err != nil {
		return 0
	}
	var price float64
	if _, err := fmt.Sscanf(string(b), "%f", &price); err != nil {
		return 0
	}
	return price
}

func init() {
	_ = json.Marshal
}
