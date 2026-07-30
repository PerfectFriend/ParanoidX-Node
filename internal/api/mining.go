// Package api provides HTTP handlers and API endpoints for the simplex-node server
package api

import (
	"encoding/json"
	"net/http"

	"ParanoidX/internal/economy"
	"ParanoidX/internal/middleware"
)


// MiningHandler handles the MiningHandler HTTP request.
func MiningHandler(dataDir string) http.HandlerFunc {
	vm := economy.LoadVaultMining(dataDir)

	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}

		switch r.Method {
		case "GET":
			pubkey := r.URL.Query().Get("pubkey")
			if pubkey != "" {
				provider, ok := vm.Providers[pubkey]
				if !ok {
					http.Error(w, "provider not found", http.StatusNotFound)
					return
				}
				writeJSON(w, provider)
				return
			}
			activeCount := 0
			var activeGB int64
			var totalPendingNg int64
			for _, p := range vm.Providers {
				if p.Active {
					activeCount++
					activeGB += p.AllocatedGB
				}
				totalPendingNg += p.PendingNg
			}
			writeJSON(w, map[string]any{
				"providers":           vm.Providers,
				"total_allocated_gb":  vm.TotalAllocated,
				"deferred_pool_ng":    vm.DeferredPoolNg,
				"last_payout":         vm.LastPayout,
				"last_payout_amount":  vm.LastPayoutAmount,
				"payout_cycle":        vm.PayoutCycle,
				"active_providers":    activeCount,
				"inactive_providers":  len(vm.Providers) - activeCount,
				"active_gb":           activeGB,
				"total_pending_ng":    totalPendingNg,
			})

		case "POST":
			pubkey := r.URL.Query().Get("pubkey")
			action := r.URL.Query().Get("action")
			if action == "" {
				action = "heartbeat"
			}

			switch action {
			case "register":
				var req struct {
					Pubkey      string `json:"pubkey"`
					AllocatedGB int64  `json:"allocated_gb"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					http.Error(w, "invalid json", http.StatusBadRequest)
					return
				}
				if req.Pubkey == "" {
					http.Error(w, "pubkey required", http.StatusBadRequest)
					return
				}
				p, err := vm.RegisterProvider(req.Pubkey, req.AllocatedGB)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				vm.Save(dataDir)
				writeJSON(w, p)

			case "heartbeat":
				if pubkey == "" {
					http.Error(w, "pubkey required", http.StatusBadRequest)
					return
				}
				err := vm.Heartbeat(pubkey)
				if err != nil {
					http.Error(w, err.Error(), http.StatusNotFound)
					return
				}
				vm.Save(dataDir)
				writeJSON(w, map[string]any{
					"pubkey":  pubkey,
					"status":  "active",
					"pending": vm.Providers[pubkey].PendingNg,
				})

			case "payout":
				amt, err := vm.ProcessDeferredPayouts(dataDir)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				vm.Save(dataDir)
				writeJSON(w, map[string]any{
					"paid": amt,
					"pool": vm.DeferredPoolNg,
				})

			default:
				http.Error(w, "unknown action", http.StatusBadRequest)
			}

		default:
			http.Error(w, "GET or POST only", 400)
		}
	}
}
