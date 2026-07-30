// Package health provides system health monitoring and disk alerts
package health

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Check struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // ok, warn, fail
	Detail  string `json:"detail,omitempty"`
	Latency string `json:"latency,omitempty"`
}

type Report struct {
	Timestamp string  `json:"timestamp"`
	Uptime    string  `json:"uptime"`
	Checks    []Check `json:"checks"`
	Healthy   bool    `json:"healthy"`
}

type Monitor struct {
	DataDir    string
	VaultPath  string
	StartTime  time.Time
	AlertURL   string // URL for admin bot alerts (e.g., http://127.0.0.1:5002/send_alert)

	diskHistory map[string][]float64 // path -> last 24 usage pct readings
	diskSamples map[string]int       // path -> count of samples
	diskMu      sync.Mutex
}


// New handles the New HTTP request.
func New(dataDir, vaultPath string, startTime time.Time) *Monitor {
	return &Monitor{
		DataDir:     dataDir,
		VaultPath:   vaultPath,
		StartTime:   startTime,
		diskHistory: make(map[string][]float64),
		diskSamples: make(map[string]int),
	}
}


// Report handles the Report HTTP request.
func (m *Monitor) Report() Report {
	var checks []Check
	checks = append(checks, m.checkDocker()...)
	checks = append(checks, m.checkDisk()...)
	checks = append(checks, m.checkDiskTrend()...)
	checks = append(checks, m.checkXRay()...)
	checks = append(checks, m.checkP2PTransport()...)
	checks = append(checks, m.checkParanoidX()...)
	checks = append(checks, m.checkTor()...)
	checks = append(checks, m.checkVault()...)
	checks = append(checks, m.checkSystem()...)
	checks = append(checks, m.checkDataDirSize()...)

	allOK := true
	for _, c := range checks {
		if c.Status == "fail" {
			allOK = false
		}
	}

	return Report{
		Timestamp: time.Now().Format(time.RFC3339),
		Uptime:    fmt.Sprintf("%dh%dm", int(time.Since(m.StartTime).Hours()), int(time.Since(m.StartTime).Minutes())%60),
		Checks:    checks,
		Healthy:   allOK,
	}
}

func (m *Monitor) checkDataDirSize() []Check {
	var checks []Check
	c := Check{Name: "data_dir_size"}
	var totalSize int64
	err := filepath.Walk(m.DataDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && strings.HasPrefix(info.Name(), ".") {
			return filepath.SkipDir
		}
		if !info.IsDir() && !strings.HasPrefix(info.Name(), ".") {
			totalSize += info.Size()
		}
		return nil
	})
	sizeMB := float64(totalSize) / (1024 * 1024)
	if err != nil {
		c.Status = "warn"
		c.Detail = fmt.Sprintf("could not walk data dir: %s", err.Error())
	} else if sizeMB > 10240 { // > 10GB
		c.Status = "fail"
		c.Detail = fmt.Sprintf("CRITICAL: %.0f MB", sizeMB)
	} else if sizeMB > 5120 { // > 5GB
		c.Status = "warn"
		c.Detail = fmt.Sprintf("WARNING: %.0f MB", sizeMB)
	} else {
		c.Status = "ok"
		c.Detail = fmt.Sprintf("%.0f MB", sizeMB)
	}
	checks = append(checks, c)
	return checks
}

func (m *Monitor) checkDocker() []Check {
	var checks []Check
	containers := []string{"ParanoidX-smp-server", "ParanoidX-xftp-server", "ParanoidX-tor", "ParanoidX-coturn"}
	for _, name := range containers {
		c := Check{Name: "docker_" + name}
		t := time.Now()
		out, err := exec.Command("docker", "ps", "--filter", "name="+name, "--format", "{{.Status}}").Output()
		c.Latency = time.Since(t).Round(time.Millisecond).String()
		if err != nil {
			c.Status = "fail"
			c.Detail = "docker command failed: " + err.Error()
		} else if strings.TrimSpace(string(out)) == "" {
			c.Status = "fail"
			c.Detail = "container not running"
		} else {
			c.Status = "ok"
			c.Detail = strings.TrimSpace(string(out))
		}
		checks = append(checks, c)
	}
	return checks
}

func (m *Monitor) checkXRay() []Check {
	c := Check{Name: "xray_native"}
	t := time.Now()
	conn, err := net.DialTimeout("tcp", "127.0.0.1:10812", 3*time.Second)
	c.Latency = time.Since(t).Round(time.Millisecond).String()
	if err != nil {
		c.Status = "fail"
		c.Detail = "xray not reachable on 127.0.0.1:10812"
	} else {
		conn.Close()
		c.Status = "ok"
		c.Detail = "xray native VMESS on :10812"
	}
	return []Check{c}
}

func (m *Monitor) checkDisk() []Check {
	paths := map[string]string{
		"disk_root": "/",
	}
	var checks []Check
	for name, path := range paths {
		c := Check{Name: name}
		t := time.Now()
		out, err := exec.Command("df", "-h", path).Output()
		c.Latency = time.Since(t).Round(time.Millisecond).String()
		if err != nil {
			c.Status = "fail"
			c.Detail = err.Error()
		} else {
			lines := strings.Split(strings.TrimSpace(string(out)), "\n")
			if len(lines) >= 2 {
				fields := strings.Fields(lines[1])
				if len(fields) >= 4 {
					pctStr := strings.TrimSuffix(fields[4], "%")
					pct := 0
					fmt.Sscanf(pctStr, "%d", &pct)
					pctF := float64(pct)

					m.diskMu.Lock()
					m.diskHistory[path] = append(m.diskHistory[path], pctF)
					m.diskSamples[path]++
					if len(m.diskHistory[path]) > 24 {
						m.diskHistory[path] = m.diskHistory[path][len(m.diskHistory[path])-24:]
					}
					m.diskMu.Unlock()

					detail := fmt.Sprintf("%s used of %s (%s)", fields[2], fields[1], fields[4])
					if pct > 95 {
						c.Status = "fail"
						c.Detail = "CRITICAL: " + detail
					} else if pct > 80 {
						c.Status = "warn"
						c.Detail = "WARNING: " + detail
					} else {
						c.Status = "ok"
						c.Detail = detail
					}
				} else {
					c.Status = "ok"
					c.Detail = string(out)
				}
			}
		}
		checks = append(checks, c)
	}
	return checks
}

func (m *Monitor) checkDiskTrend() []Check {
	m.diskMu.Lock()
	defer m.diskMu.Unlock()
	var checks []Check
	for path, history := range m.diskHistory {
		if len(history) < 6 {
			continue
		}
		n := len(history)
		recent := history[n-3:]
		older := history[n-6 : n-3]
		var avgRecent, avgOlder float64
		for _, v := range recent {
			avgRecent += v
		}
		avgRecent /= 3
		for _, v := range older {
			avgOlder += v
		}
		avgOlder /= 3
		diff := avgRecent - avgOlder
		perSample := diff / 3

		detail := fmt.Sprintf("recent=%.1f%% older=%.1f%% diff=%.1f%% per_sample=%.2f%%", avgRecent, avgOlder, diff, perSample)
		c := Check{Name: "disk_trend_" + path}
		if perSample > 2 && avgRecent > 70 {
			c.Status = "warn"
			c.Detail = "usage increasing >2%/sample above 70%: " + detail
		} else {
			c.Status = "ok"
			c.Detail = detail
		}
		checks = append(checks, c)
	}
	return checks
}

func (m *Monitor) checkP2PTransport() []Check {
	c := Check{Name: "p2p_transport"}
	t := time.Now()
	conn, err := net.DialTimeout("tcp", "127.0.0.1:17001", 2*time.Second)
	c.Latency = time.Since(t).Round(time.Millisecond).String()
	if err != nil {
		c.Status = "warn"
		c.Detail = "P2P transport not reachable on :17001"
	} else {
		conn.Close()
		c.Status = "ok"
		c.Detail = "P2P transport listening on :17001"
	}
	return []Check{c}
}

func (m *Monitor) checkParanoidX() []Check {
	var checks []Check
	c := Check{Name: "paranoidx_overall"}
	resp, err := http.Get("http://127.0.0.1:8080/api/paranoidx/status")
	if err != nil {
		c.Status = "warn"
		c.Detail = "paranoidx API unreachable"
		checks = append(checks, c)
		return checks
	}
	defer resp.Body.Close()
	var px struct {
		OverallHealthy bool `json:"overall_healthy"`
		Layers         []struct {
			Layer   string `json:"layer"`
			Healthy bool   `json:"healthy"`
			Latency int    `json:"latency_ms"`
			Message string `json:"message"`
		} `json:"layers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&px); err != nil {
		c.Status = "warn"
		c.Detail = "paranoidx decode error"
		checks = append(checks, c)
		return checks
	}
	if px.OverallHealthy {
		c.Status = "ok"
		c.Detail = "all layers healthy"
	} else {
		c.Status = "warn"
		var down []string
		for _, l := range px.Layers {
			if !l.Healthy {
				down = append(down, l.Layer)
			}
		}
		c.Detail = "degraded layers: " + strings.Join(down, ", ")
	}
	checks = append(checks, c)
	for _, l := range px.Layers {
		lc := Check{Name: "px_" + l.Layer}
		if l.Healthy {
			lc.Status = "ok"
		} else {
			lc.Status = "warn"
		}
		lc.Detail = fmt.Sprintf("%s (%dms)", l.Message, l.Latency)
		checks = append(checks, lc)
	}
	return checks
}

func (m *Monitor) checkTor() []Check {
	var checks []Check

	cOnion := Check{Name: "tor_dashboard_onion"}
	dashOnion := "/home/tomas/.local/share/simplex-node/dashboard_onion.txt"
	if b, err := os.ReadFile(dashOnion); err == nil {
		cOnion.Status = "ok"
		cOnion.Detail = strings.TrimSpace(string(b))
	} else {
		cOnion.Status = "warn"
		cOnion.Detail = "dashboard onion not found"
	}
	checks = append(checks, cOnion)

	cSMP := Check{Name: "tor_smp_onion"}
	smpOnion := "/home/tomas/ParanoidX/docker/tor/hidden_services/smp/hostname"
	if b, err := os.ReadFile(smpOnion); err == nil {
		cSMP.Status = "ok"
		cSMP.Detail = strings.TrimSpace(string(b))
	} else {
		cSMP.Status = "warn"
		cSMP.Detail = "SMP onion not found"
	}
	checks = append(checks, cSMP)

	return checks
}

func (m *Monitor) checkVault() []Check {
	var checks []Check
	c := Check{Name: "vault"}
	ents, err := os.ReadDir(m.VaultPath)
	if err != nil {
		c.Status = "warn"
		c.Detail = "vault dir not accessible"
	} else {
		c.Status = "ok"
		c.Detail = fmt.Sprintf("%d files", len(ents))
	}
	checks = append(checks, c)
	return checks
}

func (m *Monitor) checkSystem() []Check {
	var checks []Check

	cMem := Check{Name: "memory"}
	out, err := exec.Command("free", "-h").Output()
	if err == nil {
		lines := strings.Split(string(out), "\n")
		if len(lines) >= 2 {
			fields := strings.Fields(lines[1])
			if len(fields) >= 3 {
				cMem.Status = "ok"
				cMem.Detail = fmt.Sprintf("%s used / %s total", fields[2], fields[1])
			}
		}
	}
	if cMem.Status == "" {
		cMem.Status = "warn"
		cMem.Detail = "could not read memory info"
	}
	checks = append(checks, cMem)

	cLoad := Check{Name: "load_avg"}
	out2, err := os.ReadFile("/proc/loadavg")
	if err == nil {
		parts := strings.Fields(string(out2))
		if len(parts) >= 3 {
			cLoad.Status = "ok"
			cLoad.Detail = fmt.Sprintf("%s %s %s", parts[0], parts[1], parts[2])
		}
	}
	if cLoad.Status == "" {
		cLoad.Status = "warn"
		cLoad.Detail = "could not read load avg"
	}
	checks = append(checks, cLoad)

	return checks
}


// AlertIfFailing handles the AlertIfFailing HTTP request.
func (m *Monitor) AlertIfFailing(report Report) {
	for _, c := range report.Checks {
		if c.Status == "fail" && m.AlertURL != "" {
			msg := fmt.Sprintf("[health] %s: %s — %s", c.Name, c.Status, c.Detail)
			http.Get(fmt.Sprintf("%s?text=%s", m.AlertURL, msg))
		}
	}
}
