// Package api provides HTTP handlers and API endpoints for the simplex-node server
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)


// ServiceRestartHandler handles the ServiceRestartHandler HTTP request.
func ServiceRestartHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", 405)
			return
		}
		var req struct {
			Service string `json:"service"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
			return
		}
		valid := map[string]bool{"tor": true, "smp": true, "xftp": true, "coturn": true, "v2ray": true, "xray": true}
		if !valid[req.Service] {
			writeJSON(w, map[string]any{"ok": false, "error": "invalid service"})
			return
		}
		// Native xray restart (not Docker)
		if req.Service == "xray" {
			exec.Command("pkill", "-x", "xray").Run()
			xrayBin := os.Getenv("HOME") + "/bin/v2ray/xray"
			xrayCfg := os.Getenv("HOME") + "/bin/v2ray/config.json"
			exec.Command("nohup", xrayBin, "run", "-c", xrayCfg).Start()
			logAudit("service_restart", "admin", "restarted native xray")
			writeJSON(w, map[string]any{"status": "ok", "service": "xray"})
			return
		}
		containerName := "ParanoidX-" + req.Service
		cmd := exec.Command("docker", "restart", containerName)
		if out, err := cmd.CombinedOutput(); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error(), "output": string(out)})
			return
		}
		logAudit("service_restart", "admin", "restarted service: "+req.Service)
		writeJSON(w, map[string]any{"status": "ok", "service": req.Service})
	}
}


// ServiceStatusHandler handles the ServiceStatusHandler HTTP request.
func ServiceStatusHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "GET required", 405)
			return
		}
		composeDir := os.Getenv("SIMPLEX_SRC")
		if composeDir == "" {
			composeDir = filepath.Join(os.Getenv("HOME"), "ParanoidX")
		}
		composeDir = filepath.Join(composeDir, "docker")
		cmd := exec.Command("docker", "compose", "ps", "--format", "{{.Name}}\t{{.Status}}")
		cmd.Dir = composeDir
		out, err := cmd.Output()
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		containers := make(map[string]string)
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			parts := strings.SplitN(line, "\t", 2)
			if len(parts) == 2 {
				containers[parts[0]] = parts[1]
			}
		}
		writeJSON(w, map[string]any{"ok": true, "containers": containers, "count": len(containers)})
	}
}


// DockerContainerListHandler handles the DockerContainerListHandler HTTP request.
func DockerContainerListHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "GET required", 405)
			return
		}
		cmd := exec.Command("docker", "ps", "-a", "--format", "{{.ID}}\t{{.Image}}\t{{.Names}}\t{{.Status}}\t{{.Ports}}")
		out, err := cmd.Output()
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		containers := make([]map[string]string, 0)
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, "\t", 5)
			c := make(map[string]string)
			if len(parts) >= 1 {
				c["id"] = parts[0][:12]
			}
			if len(parts) >= 2 {
				c["image"] = parts[1]
			}
			if len(parts) >= 3 {
				c["name"] = parts[2]
			}
			if len(parts) >= 4 {
				c["status"] = parts[3]
			}
			if len(parts) >= 5 {
				c["ports"] = parts[4]
			}
			containers = append(containers, c)
		}
		writeJSON(w, map[string]any{"ok": true, "containers": containers, "count": len(containers)})
	}
}


// DockerContainerLogsHandler handles the DockerContainerLogsHandler HTTP request.
func DockerContainerLogsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "GET required", 405)
			return
		}
		name := r.URL.Query().Get("name")
		if name == "" {
			writeJSON(w, map[string]any{"ok": false, "error": "name required"})
			return
		}
		tailStr := r.URL.Query().Get("tail")
		tail := validateIntParam(tailStr, 50)
		if tail < 1 {
			tail = 50
		}
		cmd := exec.Command("docker", "logs", "--tail", fmt.Sprintf("%d", tail), "-t", name)
		out, err := cmd.CombinedOutput()
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error(), "output": string(out)})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "container": name, "logs": string(out), "tail": tail})
	}
}
