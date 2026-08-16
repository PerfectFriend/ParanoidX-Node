// Package paranoidx implements the ParanoidX multi-layer proxy chain
package paranoidx

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Layer identifies a transport layer in the ParanoidX chain.
type Layer string

const (
	LayerV2Ray    Layer = "v2ray"
	LayerVPN      Layer = "vpn"
	LayerTor      Layer = "tor"
	LayerSimpleX  Layer = "simplex"
	LayerVMess    Layer = "vmess"
)

// LayerStatus represents the health state of a single transport layer.
type LayerStatus struct {
	Layer     Layer  `json:"layer"`
	Healthy   bool   `json:"healthy"`
	LatencyMs int64  `json:"latency_ms"`
	Message   string `json:"message"`
	Connected string `json:"connected_since"`
}

// HealthSnapshot is a point-in-time record of ParanoidX health.
type HealthSnapshot struct {
	Timestamp string `json:"timestamp"`
	Overall   bool   `json:"overall"`
	Layers    []struct {
		Layer   Layer `json:"layer"`
		Healthy bool  `json:"healthy"`
	} `json:"layers"`
}

// BridgeStatus aggregates health across all ParanoidX layers.
type BridgeStatus struct {
	mu          sync.RWMutex
	layers      map[Layer]*LayerStatus
	overall     bool
	lastUpdated time.Time
	history     []HealthSnapshot
	maxHistory  int
}

var globalStatus = &BridgeStatus{
	layers: map[Layer]*LayerStatus{
		LayerV2Ray:   {Layer: LayerV2Ray},
		LayerTor:     {Layer: LayerTor},
		LayerSimpleX: {Layer: LayerSimpleX},
		// VPN added dynamically via SetLayerStatus when enabled
	},
	maxHistory: 100,
}

// SetLayerStatus updates the health of a single layer.
func SetLayerStatus(layer Layer, healthy bool, latencyMs int64, message string) {
	globalStatus.mu.Lock()
	defer globalStatus.mu.Unlock()
	globalStatus.layers[layer] = &LayerStatus{
		Layer:     layer,
		Healthy:   healthy,
		LatencyMs: latencyMs,
		Message:   message,
		Connected: time.Now().Format(time.RFC3339),
	}
	globalStatus.lastUpdated = time.Now()
	globalStatus.overall = true
	for _, s := range globalStatus.layers {
		if !s.Healthy {
			// V2Ray and VPN are a mutual fallback pair: at least one must be healthy
			if s.Layer == LayerV2Ray || s.Layer == LayerVPN {
				var counterpart Layer
				if s.Layer == LayerV2Ray {
					counterpart = LayerVPN
				} else {
					counterpart = LayerV2Ray
				}
				if other, ok := globalStatus.layers[counterpart]; ok && other.Healthy {
					continue
				}
			}
			globalStatus.overall = false
			break
		}
	}
	// Record health history snapshot
	snap := HealthSnapshot{
		Timestamp: time.Now().Format(time.RFC3339),
		Overall:   globalStatus.overall,
	}
	for _, s := range globalStatus.layers {
		snap.Layers = append(snap.Layers, struct {
			Layer   Layer `json:"layer"`
			Healthy bool  `json:"healthy"`
		}{Layer: s.Layer, Healthy: s.Healthy})
	}
	globalStatus.history = append(globalStatus.history, snap)
	if len(globalStatus.history) > globalStatus.maxHistory {
		globalStatus.history = globalStatus.history[len(globalStatus.history)-globalStatus.maxHistory:]
	}
}

// GetStatus returns a snapshot of all layer statuses.
func GetStatus() map[Layer]*LayerStatus {
	globalStatus.mu.RLock()
	defer globalStatus.mu.RUnlock()
	out := make(map[Layer]*LayerStatus, len(globalStatus.layers))
	for k, v := range globalStatus.layers {
		out[k] = &LayerStatus{
			Layer:     v.Layer,
			Healthy:   v.Healthy,
			LatencyMs: v.LatencyMs,
			Message:   v.Message,
			Connected: v.Connected,
		}
	}
	return out
}

// IsOverallHealthy returns true when all layers report healthy.
func IsOverallHealthy() bool {
	globalStatus.mu.RLock()
	defer globalStatus.mu.RUnlock()
	return globalStatus.overall
}

// GetHistory returns health history snapshots.
func GetHistory(limit int) []HealthSnapshot {
	globalStatus.mu.RLock()
	defer globalStatus.mu.RUnlock()
	if limit <= 0 || limit > len(globalStatus.history) {
		limit = len(globalStatus.history)
	}
	out := make([]HealthSnapshot, limit)
	copy(out, globalStatus.history[len(globalStatus.history)-limit:])
	return out
}

// HistoryHandler returns an HTTP handler for /api/paranoidx/history.
func HistoryHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := 20
		if l := r.URL.Query().Get("limit"); l != "" {
			if v, err := parseInt(l); err == nil && v > 0 && v <= 100 {
				limit = v
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"history": GetHistory(limit),
			"count":   len(GetHistory(limit)),
		})
	}
}

func parseInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

// GetOverallStatus returns a consolidated status report.
func GetOverallStatus() map[string]any {
	globalStatus.mu.RLock()
	defer globalStatus.mu.RUnlock()
	layers := make([]map[string]any, 0, len(globalStatus.layers))
	for _, s := range globalStatus.layers {
		layers = append(layers, map[string]any{
			"layer":     s.Layer,
			"healthy":   s.Healthy,
			"latency_ms": s.LatencyMs,
			"message":   s.Message,
			"since":     s.Connected,
		})
	}
	return map[string]any{
		"overall_healthy": globalStatus.overall,
		"layers":          layers,
		"last_updated":    globalStatus.lastUpdated.Format(time.RFC3339),
	}
}
