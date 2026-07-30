package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DebugHandler returns a comprehensive snapshot of the entire node:
// system metrics, transport hub, bridge, services, config, network, goroutines.
// GET /api/debug
func DebugHandler(dataDir string, transportStats func() map[string]any) http.HandlerFunc {
	startTime := time.Now()
	return func(w http.ResponseWriter, r *http.Request) {
		accept := r.Header.Get("Accept")

		result := collectDebugData(dataDir, startTime, transportStats)

		if strings.Contains(accept, "text/html") {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			RenderDebugHTML(w, result)
			return
		}
		writeJSON(w, result)
	}
}

func collectDebugData(dataDir string, startTime time.Time, transportStats func() map[string]any) map[string]any {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	uptimeSecs := int(time.Since(startTime).Seconds())

	// ── System ──
	ramTotal, ramAvail := readMemInfo()
	l1, l5, l15 := readLoadAvg()
	cpuPct := l1 * 100 / float64(runtime.NumCPU())
	if cpuPct > 100 {
		cpuPct = 100
	}

	sys := map[string]any{
		"uptime_seconds": uptimeSecs,
		"uptime_human":   fmt.Sprintf("%dh%dm", uptimeSecs/3600, (uptimeSecs%3600)/60),
		"cpu_percent":    cpuPct,
		"cpu_cores":      runtime.NumCPU(),
		"load_1m":        l1,
		"load_5m":        l5,
		"load_15m":       l15,
		"go_version":     runtime.Version(),
		"goroutines":     runtime.NumGoroutine(),
		"cgo_calls":      runtime.NumCgoCall(),
		"memory": map[string]any{
			"alloc_mb":       m.Alloc / 1024 / 1024,
			"total_alloc_mb": m.TotalAlloc / 1024 / 1024,
			"sys_mb":         m.Sys / 1024 / 1024,
			"heap_mb":        m.HeapAlloc / 1024 / 1024,
			"stack_mb":       m.StackInuse / 1024 / 1024,
			"gc_cycles":      m.NumGC,
			"gc_pause_ns":    m.PauseTotalNs,
			"next_gc_mb":     m.NextGC / 1024 / 1024,
		},
	}
	if ramTotal > 0 {
		sys["ram"] = map[string]any{
			"total_mb":  ramTotal / 1024,
			"used_mb":   (ramTotal - ramAvail) / 1024,
			"avail_mb":  ramAvail / 1024,
			"used_pct":  fmt.Sprintf("%.1f%%", float64(ramTotal-ramAvail)/float64(ramTotal)*100),
		}
	}

	// ── Disk ──
	var disk fs
	if err := diskUsage("/", &disk); err == nil {
		sys["disk"] = map[string]any{
			"total_gb":  disk.Total / 1024 / 1024 / 1024,
			"used_gb":   disk.Used / 1024 / 1024 / 1024,
			"avail_gb":  disk.Avail / 1024 / 1024 / 1024,
			"used_pct":  fmt.Sprintf("%.1f%%", float64(disk.Used)/float64(disk.Total)*100),
		}
	}

	// ── Data Directory ──
	dataDirInfo := map[string]any{"path": dataDir}
	if fi, err := os.Stat(dataDir); err == nil {
		dataDirInfo["exists"] = true
		dataDirInfo["mode"] = fi.Mode().String()
		var dataSize int64
		filepath.Walk(dataDir, func(p string, fi os.FileInfo, err error) error {
			if err == nil && !fi.IsDir() {
				dataSize += fi.Size()
			}
			return nil
		})
		dataDirInfo["size_mb"] = dataSize / 1024 / 1024
	} else {
		dataDirInfo["exists"] = false
		dataDirInfo["error"] = err.Error()
	}
	sys["data_dir"] = dataDirInfo

	// ── Network ──
	sys["interfaces"] = readNetworkInterfaces()
	sys["bandwidth"] = readBandwidthStats()
	sys["env"] = readEnv()

	// ── Transport ──
	transport := map[string]any{"available": transportStats != nil}
	if transportStats != nil {
		transport["stats"] = transportStats()
	}

	// ── Bridge ──
	bridge := map[string]any{
		"connected":         BridgeConnected,
		"reconnect_count":   BridgeReconnectCount,
	}

	// ── Goroutine dump (top 10) ──
	grDump := readGoroutineDump()

	// ── Host ──
	hostname, _ := os.Hostname()

	return map[string]any{
		"ok":                true,
		"timestamp":         time.Now().UTC().Format(time.RFC3339),
		"hostname":          hostname,
		"pid":               os.Getpid(),
		"data_dir":          dataDir,
		"system":            sys,
		"transport":         transport,
		"bridge":            bridge,
		"goroutines_top":    grDump,
	}
}

func readMemInfoSys() (totalKB, availKB uint64) { return readMemInfo() }



type netIf struct {
	Name      string `json:"name"`
	IPs       string `json:"ips,omitempty"`
	MAC       string `json:"mac,omitempty"`
	Flags     string `json:"flags"`
	MTU       int    `json:"mtu"`
	IsLoop    bool   `json:"is_loopback"`
}

func readNetworkInterfaces() []netIf {
	cmd := exec.Command("ip", "-json", "addr")
	out, err := cmd.Output()
	if err != nil {
		// fallback: /proc/net/dev basic
		return readNetDevBrief()
	}
	var ifs []struct {
		IfName string `json:"ifname"`
		Flags  []string `json:"flags"`
		MTU    int    `json:"mtu"`
		Address string `json:"address"`
		AddrInfo []struct {
			Local string `json:"local"`
		} `json:"addr_info"`
	}
	if err := json.Unmarshal(out, &ifs); err != nil {
		return readNetDevBrief()
	}
	res := make([]netIf, 0, len(ifs))
	for _, i := range ifs {
		ips := ""
		for _, a := range i.AddrInfo {
			if ips != "" {
				ips += ", "
			}
			ips += a.Local
		}
		isLoop := false
		for _, f := range i.Flags {
			if f == "LOOPBACK" {
				isLoop = true
				break
			}
		}
		res = append(res, netIf{
			Name:   i.IfName,
			IPs:    ips,
			MAC:    i.Address,
			Flags:  strings.Join(i.Flags, ","),
			MTU:    i.MTU,
			IsLoop: isLoop,
		})
	}
	return res
}

var bandwidthMu sync.Mutex
var bandwidthLast map[string]struct{ rx, tx uint64 }

func readBandwidthStats() map[string]any {
	bandwidthMu.Lock()
	defer bandwidthMu.Unlock()

	b, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return map[string]any{"error": err.Error()}
	}

	now := time.Now()
	ifaces := map[string]struct{ rx, tx uint64 }{}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, ":") && !strings.HasPrefix(line, "Inter-") && !strings.HasPrefix(line, "face") {
			parts := strings.Fields(line)
			if len(parts) >= 10 {
				name := strings.TrimSuffix(parts[0], ":")
				rx, _ := strconv.ParseUint(parts[1], 10, 64)
				tx, _ := strconv.ParseUint(parts[9], 10, 64)
				ifaces[name] = struct{ rx, tx uint64 }{rx, tx}
			}
		}
	}

	rates := map[string]map[string]any{}
	if bandwidthLast != nil {
		elapsed := now.Sub(bandwidthLastTime).Seconds()
		if elapsed > 0 {
			for name, cur := range ifaces {
				if prev, ok := bandwidthLast[name]; ok {
					rxRate := float64(cur.rx-prev.rx) / elapsed
					txRate := float64(cur.tx-prev.tx) / elapsed
					if rxRate >= 0 && txRate >= 0 {
						rates[name] = map[string]any{
							"rx_bps":  int64(rxRate),
							"tx_bps":  int64(txRate),
							"rx_kbps": fmt.Sprintf("%.1f", rxRate/1024),
							"tx_kbps": fmt.Sprintf("%.1f", txRate/1024),
						}
					}
				}
			}
		}
	}

	bandwidthLast = ifaces
	bandwidthLastTime = now

	totals := map[string]map[string]any{}
	for name, cur := range ifaces {
		totals[name] = map[string]any{
			"rx_bytes": cur.rx,
			"tx_bytes": cur.tx,
			"rx_mb":    fmt.Sprintf("%.1f", float64(cur.rx)/1024/1024),
			"tx_mb":    fmt.Sprintf("%.1f", float64(cur.tx)/1024/1024),
		}
	}

	return map[string]any{
		"totals":  totals,
		"rates":   rates,
	}
}

var bandwidthLastTime time.Time

func readNetDevBrief() []netIf {
	b, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return nil
	}
	var ifs []netIf
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, ":") && !strings.HasPrefix(line, "Inter-") && !strings.HasPrefix(line, "face") {
			parts := strings.Fields(line)
			if len(parts) >= 1 {
				name := strings.TrimSuffix(parts[0], ":")
				ifs = append(ifs, netIf{Name: name, Flags: "up"})
			}
		}
	}
	return ifs
}

func readEnv() map[string]string {
	relevant := []string{"DATA_DIR", "HOME", "USER", "HOSTNAME", "PATH"}
	env := map[string]string{}
	for _, k := range relevant {
		if v := os.Getenv(k); v != "" {
			if k == "PATH" && len(v) > 60 {
				v = v[:60] + "..."
			}
			env[k] = v
		}
	}
	return env
}

func readGoroutineDump() []string {
	var buf strings.Builder
	pprof.Lookup("goroutine").WriteTo(&buf, 1)
	lines := strings.Split(buf.String(), "\n")
	if len(lines) > 20 {
		lines = lines[:20]
	}
	return lines
}

func RenderDebugHTML(w http.ResponseWriter, data map[string]any) {
	b, _ := json.MarshalIndent(data, "", "  ")
	tmpl := `<!DOCTYPE html>
<html lang="en">
<head><meta charset="UTF-8"><title>BridgeX Debug</title>
<style>
  * { margin:0; padding:0; box-sizing:border-box; }
  body { font-family:monospace; background:#0d1117; color:#c9d1d9; padding:20px; }
  h1 { color:#ffd700; margin-bottom:5px; }
  .ts { color:#8b949e; margin-bottom:20px; }
  pre { background:#161b22; border:1px solid #30363d; border-radius:8px; padding:16px; overflow:auto; font-size:13px; line-height:1.5; }
  .key { color:#79c0ff; }
  .str { color:#a5d6ff; }
  .num { color:#79c0ff; }
  .bool { color:#ff7b72; }
  .null { color:#8b949e; }
  .section { margin-bottom:20px; }
  .section h2 { color:#ffd700; font-size:1.1em; border-bottom:1px solid #30363d; padding-bottom:6px; margin-bottom:10px; }
  .stat { display:inline-flex; background:#161b22; border:1px solid #30363d; border-radius:8px; padding:12px 16px; margin:4px; flex-direction:column; }
  .stat .val { font-size:1.4em; font-weight:700; color:#58a6ff; }
  .stat .lbl { font-size:0.75em; color:#8b949e; margin-top:2px; }
  .flex { display:flex; flex-wrap:wrap; margin-bottom:12px; }
  .badge { display:inline-block; padding:2px 8px; border-radius:4px; font-size:0.8em; margin:2px; }
  .badge.green { background:#166534; color:#4ade80; }
  .badge.yellow { background:#713f12; color:#fbbf24; }
  .badge.red { background:#7f1d1d; color:#f87171; }
  .badge.blue { background:#1e3a5f; color:#60a5fa; }
  a { color:#58a6ff; }
  .warn { color:#fbbf24; }
  .err { color:#f87171; }
</style></head>
<body>
<h1>🔍 BridgeX Debug</h1>
<p class="ts">%s | <a href="/api/debug">JSON</a></p>
<div class="flex" id="stats"></div>
<div id="raw"><pre>%s</pre></div>
<script>
(async function(){
  const d = %s;
  const s = document.getElementById('stats');
  function stat(val, lbl, cls){
    const e = document.createElement('div'); e.className='stat';
    e.innerHTML='<div class="val" style="color:'+(cls||'#58a6ff')+'">'+val+'</div><div class="lbl">'+lbl+'</div>';
    s.appendChild(e);
  }
  const sys = d.system || {};
  stat(sys.cpu_percent+'%', 'CPU', parseFloat(sys.cpu_percent)>80?'#f87171':parseFloat(sys.cpu_percent)>60?'#fbbf24':'#4ade80');
  if(sys.ram) stat(sys.ram.used_pct, 'RAM', parseFloat(sys.ram.used_pct)>80?'#f87171':parseFloat(sys.ram.used_pct)>60?'#fbbf24':'#58a6ff');
  if(sys.disk) stat(sys.disk.used_pct, 'Disk', parseFloat(sys.disk.used_pct)>90?'#f87171':parseFloat(sys.disk.used_pct)>80?'#fbbf24':'#4ade80');
  stat(sys.goroutines, 'Goroutines', '#c9d1d9');
  stat(sys.uptime_human, 'Uptime', '#ffd700');
  stat(d.bridge && d.bridge.connected ? 'UP' : 'DOWN', 'Bridge', d.bridge && d.bridge.connected ? '#4ade80' : '#f87171');
  if(d.transport && d.transport.stats) {
    stat(d.transport.stats.registered_apps||0, 'Transport Apps', '#60a5fa');
    stat(d.transport.stats.active_ws_connections||0, 'WS Conns', '#60a5fa');
    stat(d.transport.stats.health_score||0, 'Health', (d.transport.stats.health_score||0)>=80?'#4ade80':(d.transport.stats.health_score||0)>=50?'#fbbf24':'#f87171');
  }
  const pre = document.getElementById('raw').querySelector('pre');
  pre.innerHTML = syntaxHighlight(JSON.stringify(d, null, 2));
})();
function syntaxHighlight(json) {
  return json.replace(/&/g,'&').replace(/</g,'<').replace(/>/g,'>').replace(
    /("(?:[^"\\]|\\.)*")\s*:/g,'<span class="key">$1</span>:').replace(
    /"((?:[^"\\]|\\.)*)"/g,'<span class="str">"$1"</span>').replace(
    /(-?\d+(?:\.\d+)?)/g,'<span class="num">$1</span>').replace(
    /(true|false)/g,'<span class="bool">$1</span>').replace(
    /null/g,'<span class="null">null</span>');
}
</script>
</body></html>`
	now := time.Now().Format("2006-01-02 15:04:05 MST")
	fmt.Fprintf(w, tmpl, now, string(b), string(b))
}
