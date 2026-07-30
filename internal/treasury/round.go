// Package treasury implements treasury round management
package treasury

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"time"

	"ParanoidX/internal/economy"
	"ParanoidX/internal/fileutil"
)

const (
	AutoRoundInitialThreshold = 1000.0 // $1000 USDT triggers first auto-round
	AutoRoundScaleAfter       = 10      // after 10 rounds, threshold x10
)

type RoundResult struct {
	USDTIn          float64              `json:"usdt_in"`
	NewSilverNg     int64                `json:"new_silver_ng"`
	TreasuryShareNg int64                `json:"treasury_share_ng"`
	CurrentReserve  int64                `json:"current_reserve_ng"`
	Dividends       []DividendAllocation `json:"dividends"`
}

type DividendAllocation struct {
	Holder string  `json:"holder"`
	Serial string  `json:"serial"`
	Ng     int64   `json:"ng"`
	Denom  float64 `json:"denom"`
}

func roundLogPath(dataDir string) string {
	return filepath.Join(dataDir, "silver_rounds.log")
}

func reservePath(dataDir string) string {
	return filepath.Join(dataDir, "silver_reserve_ng.txt")
}

func registryPath(dataDir string) string {
	return filepath.Join(dataDir, "banknotes_registry.json")
}

func readReserve(dataDir string) int64 {
	b, err := os.ReadFile(reservePath(dataDir))
	if err != nil {
		return 0
	}
	var current int64
	fmt.Sscanf(string(b), "%d", &current)
	return current
}

func writeReserve(dataDir string, ng int64) {
	os.WriteFile(reservePath(dataDir), []byte(fmt.Sprintf("%d", ng)), 0600)
}

func readHolders(dataDir string) []map[string]any {
	var holders []map[string]any
	b, err := os.ReadFile(registryPath(dataDir))
	if err != nil {
		return holders
	}
	json.Unmarshal(b, &holders)
	return holders
}

func writeHolders(dataDir string, holders []map[string]any) {
	if len(holders) > 0 {
		fileutil.WriteJSON(registryPath(dataDir), holders)
	}
}

type RoundParams struct {
	DataDir string
	USDT    float64
	AnnounceDir string
}

// ExecuteRound processes a silver round: adds to reserve, distributes dividends.
func ExecuteRound(p RoundParams) (*RoundResult, error) {
	if p.USDT <= 0 {
		p.USDT = 1000.0
	}
	newSilverNg := int64(p.USDT * 1e9)
	if newSilverNg < 1000000000 {
		newSilverNg = 1000000000
	}

	reserve := readReserve(p.DataDir)
	reserve += newSilverNg
	writeReserve(p.DataDir, reserve)

	treasuryShare := newSilverNg * 20 / 100
	holders := readHolders(p.DataDir)

	totalShares := 0.0
	for _, h := range holders {
		if d, ok := h["denomination_tlr"].(float64); ok {
			totalShares += d
		}
	}

	dividends := []DividendAllocation{}
	accruedBySerial := map[string]int64{}

	for _, h := range holders {
		d, ok := h["denomination_tlr"].(float64)
		if !ok || totalShares <= 0 {
			continue
		}
		share := d / totalShares
		ng := int64(math.Round(float64(newSilverNg-treasuryShare) * share))
		holder := ""
		if hh, ok := h["holder"].(string); ok {
			holder = hh
		}
		serial := ""
		if s, ok := h["serial"].(string); ok {
			serial = s
		}
		if holder != "" {
			dividends = append(dividends, DividendAllocation{
				Holder: holder,
				Serial: serial,
				Ng:     ng,
				Denom:  d,
			})
		}
		if serial != "" {
			accruedBySerial[serial] += ng
		}
	}

	for i := range holders {
		s, ok := holders[i]["serial"].(string)
		if !ok {
			continue
		}
		if add, has := accruedBySerial[s]; has {
			prev := int64(0)
			if p, ok := holders[i]["accrued_ng"].(float64); ok {
				prev = int64(p)
			} else if p, ok := holders[i]["accrued_ng"].(int64); ok {
				prev = p
			}
			holders[i]["accrued_ng"] = prev + add
		}
	}
	writeHolders(p.DataDir, holders)

	appendLog(roundLogPath(p.DataDir),
		fmt.Sprintf("round %d: usdt=%.2f new_ng=%d treasury_share=%d dividends=%d",
			time.Now().Unix(), p.USDT, newSilverNg, treasuryShare, len(dividends)))

	if p.AnnounceDir != "" {
		announce := fmt.Sprintf(
			"SILVER ROUND\nTime: %s\nInflow: %.2f USDT\nTokenized: %d ng\nTreasury: %d ng (20%%)\nDividends: %d holders\n",
			time.Now().Format(time.RFC3339), p.USDT, newSilverNg, treasuryShare, len(dividends))
		os.WriteFile(filepath.Join(p.AnnounceDir, fmt.Sprintf("announcement-round-%d.txt", time.Now().Unix())), []byte(announce), 0600)
	}

	return &RoundResult{
		USDTIn:          p.USDT,
		NewSilverNg:     newSilverNg,
		TreasuryShareNg: treasuryShare,
		CurrentReserve:  reserve,
		Dividends:       dividends,
	}, nil
}

func appendLog(path, line string) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintln(f, line)
}

// ProofOfReserve holds the current reserve state.
type ProofOfReserve struct {
	TotalSilverNg int64   `json:"total_silver_ng"`
	TotalIssuedNg int64   `json:"total_issued_ng"`
	ReserveRatio  float64 `json:"reserve_ratio"`
	LastUpdated   string  `json:"last_updated"`
}

// GetProofOfReserve calculates the current proof of reserve.
func GetProofOfReserve(dataDir string) *ProofOfReserve {
	reserve := readReserve(dataDir)
	holders := readHolders(dataDir)
	var issued int64
	for _, h := range holders {
		if d, ok := h["denomination_tlr"].(float64); ok {
			issued += int64(d * float64(economy.NGPerTLR))
		}
	}
	ratio := 0.0
	if issued > 0 {
		ratio = float64(reserve) / float64(issued)
	}
	return &ProofOfReserve{
		TotalSilverNg: reserve,
		TotalIssuedNg: issued,
		ReserveRatio:  ratio,
		LastUpdated:   time.Now().UTC().Format(time.RFC3339),
	}
}

// StartAutoRound runs in a goroutine, checking the monitor's TotalUSDT
// and triggering silver rounds when the threshold is met.
func (m *Monitor) StartAutoRound(dataDir, announceDir string) {
	threshold := AutoRoundInitialThreshold
	roundCount := 0
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	slog.Info("auto-round: started", "initial_threshold", threshold)
	for {
		<-ticker.C
		m.mu.Lock()
		total := m.TotalUSDT
		m.mu.Unlock()

		if total < threshold {
			continue
		}

		slog.Info("auto-round: threshold reached", "total_usdt", total, "threshold", threshold)
		result, err := ExecuteRound(RoundParams{
			DataDir:      dataDir,
			USDT:         total,
			AnnounceDir:  announceDir,
		})
		if err != nil {
			slog.Error("auto-round: execute failed", "error", err)
			continue
		}

		// Reset monitor's tracked deposits
		m.mu.Lock()
		m.TotalUSDT = 0
		m.Deposits = []Deposit{}
		m.Seen = make(map[string]bool)
		m.mu.Unlock()

		roundCount++
		slog.Info("auto-round: completed",
			"round", roundCount,
			"usdt", result.USDTIn,
			"new_ng", result.NewSilverNg,
			"reserve", result.CurrentReserve,
			"dividends", len(result.Dividends))

		// Dynamic threshold scaling
		if roundCount >= AutoRoundScaleAfter {
			threshold *= 10
			slog.Info("auto-round: threshold scaled", "new_threshold", threshold)
		}
	}
}
