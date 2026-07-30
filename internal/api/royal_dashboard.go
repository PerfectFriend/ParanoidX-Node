// Package api provides HTTP handlers and API endpoints for the simplex-node server
package api

import (
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"ParanoidX/internal/economy"
	"ParanoidX/internal/middleware"
)

//go:embed royal_ui.html
var royalUI embed.FS


// RoyalDashboardHandler handles the RoyalDashboardHandler HTTP request.
func RoyalDashboardHandler(dataDir string) http.HandlerFunc {
	html, err := royalUI.ReadFile("royal_ui.html")
	if err != nil {
		panic("royal_ui.html not embedded: " + err.Error())
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		if !isRoyalNodePath(dataDir) {
			http.Error(w, "forbidden", 403)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write(html)
	}
}

// ── C22: Royal SSE Events ──────────────────────────────────────────────────

type royalSSEClient struct {
	ch    chan string
	done  chan struct{}
}

var (
	royalSSEClients   []*royalSSEClient
	royalSSEClientsMu sync.Mutex
)

func broadcastRoyalSSE(data string) {
	royalSSEClientsMu.Lock()
	defer royalSSEClientsMu.Unlock()
	for _, c := range royalSSEClients {
		select {
		case c.ch <- data:
		default:
		}
	}
}


// RoyalSSEHandler handles the RoyalSSEHandler HTTP request.
func RoyalSSEHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) { return }
		if !isRoyalNodePath(dataDir) { http.Error(w, "forbidden", 403); return }

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", 500)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		client := &royalSSEClient{
			ch:   make(chan string, 16),
			done: make(chan struct{}),
		}
		royalSSEClientsMu.Lock()
		royalSSEClients = append(royalSSEClients, client)
		royalSSEClientsMu.Unlock()

		notify := r.Context().Done()
		ticker := time.NewTicker(30 * time.Second)

		// Send initial heartbeat
		fmt.Fprintf(w, "event: connected\ndata: {}\n\n")
		flusher.Flush()

		for {
			select {
			case <-notify:
				royalSSEClientsMu.Lock()
				for i, c := range royalSSEClients {
					if c == client {
						royalSSEClients = append(royalSSEClients[:i], royalSSEClients[i+1:]...)
						break
					}
				}
				royalSSEClientsMu.Unlock()
				close(client.done)
				return

			case msg := <-client.ch:
				fmt.Fprintf(w, "event: update\ndata: %s\n\n", msg)
				flusher.Flush()

			case <-ticker.C:
				// Periodic heartbeat
				oraclePrice := economy.DefaultSilverSpotUSDperOZ
				if GlobalOracleRef != nil {
					oraclePrice = GlobalOracleRef.GetPrice()
				}
				state := collectRoyalTreasuryState(dataDir, oraclePrice)
				b, _ := json.Marshal(state)
				fmt.Fprintf(w, "event: heartbeat\ndata: %s\n\n", string(b))
				flusher.Flush()

				// Evaluate alert rules
				if GlobalAlertState != nil {
					triggered := GlobalAlertState.Evaluate(dataDir, oraclePrice)
					for _, alert := range triggered {
						fmt.Fprintf(w, "event: alert\ndata: %s\n\n", alert)
						flusher.Flush()
						if SimplexCmd != nil && BridgeConnected {
							SimplexCmd(fmt.Sprintf("/_send 1 %s", alert))
						}
					}
				}
			}
		}
	}
}

func collectRoyalTreasuryState(dataDir string, oraclePrice float64) map[string]any {
	reserveNg := int64(0)
	if b, _ := os.ReadFile(filepath.Join(dataDir, "silver_reserve_ng.txt")); b != nil {
		fmt.Sscanf(string(b), "%d", &reserveNg)
	}
	ledger := economy.LoadLedger(dataDir)
	cfg := economy.DefaultTreasuryConfig()
	tier := economy.DetectTier(reserveNg, cfg)
	monthsLeft := int64(0)
	if cfg.MonthlyOpsNg > 0 {
		monthsLeft = reserveNg / cfg.MonthlyOpsNg
	}
	health := "healthy"
	if monthsLeft < 3 {
		health = "critical"
	} else if monthsLeft < 6 {
		health = "warn"
	}
	return map[string]any{
		"reserve_ng":        reserveNg,
		"supply_ng":         ledger.TotalSupply,
		"tier":              tier.String(),
		"health":            health,
		"months_until_depleted": monthsLeft,
		"oracle_price":      oraclePrice,
		"timestamp":         time.Now().UTC().Format(time.RFC3339),
	}
}
