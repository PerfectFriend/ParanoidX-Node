// Package api provides HTTP handlers and API endpoints for the simplex-node server
package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"ParanoidX/internal/middleware"
)

// DBInfo describes a single database file and its backup count.
type DBInfo struct {
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	ModTime  string `json:"mod_time"`
	Backups  int    `json:"backups"`
}


// DBListHandler lists all SQLite database files in the data directory.
func DBListHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		exts := map[string]bool{".db": true, ".sqlite": true, ".sqlite3": true}
		entries, err := os.ReadDir(dataDir)
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		var dbs []DBInfo
		backupDir := filepath.Join(dataDir, "db_backups")
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if !exts[strings.ToLower(filepath.Ext(e.Name()))] {
				continue
			}
			fi, err := e.Info()
			if err != nil {
				continue
			}
			info := DBInfo{
				Name:    e.Name(),
				Size:    fi.Size(),
				ModTime: fi.ModTime().Format(time.RFC3339),
			}
			if backupEntries, err := os.ReadDir(filepath.Join(backupDir, e.Name())); err == nil {
				info.Backups = len(backupEntries)
			}
			dbs = append(dbs, info)
		}
		writeJSON(w, map[string]any{"ok": true, "databases": dbs})
	}
}


// DBBackupHandler creates a timestamped backup copy of a specified database file.
func DBBackupHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		name := r.URL.Query().Get("name")
		if name == "" {
			writeJSON(w, map[string]any{"ok": false, "error": "name required"})
			return
		}
		src := filepath.Join(dataDir, name)
		if _, err := os.Stat(src); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "db not found"})
			return
		}
		backupDir := filepath.Join(dataDir, "db_backups", name)
		if err := os.MkdirAll(backupDir, 0700); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		ts := time.Now().Format("20060102-150405")
		dst := filepath.Join(backupDir, fmt.Sprintf("%s.%s.bak", name, ts))
		input, err := os.ReadFile(src)
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		if err := os.WriteFile(dst, input, 0600); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "backup": dst, "size": len(input)})
	}
}


// DBBackupListHandler lists all backup files for a given database name.
func DBBackupListHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		name := r.URL.Query().Get("name")
		if name == "" {
			writeJSON(w, map[string]any{"ok": false, "error": "name required"})
			return
		}
		backupDir := filepath.Join(dataDir, "db_backups", name)
		entries, err := os.ReadDir(backupDir)
		if err != nil {
			writeJSON(w, map[string]any{"ok": true, "backups": []string{}})
			return
		}
		var backups []map[string]any
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			fi, err := e.Info()
			if err != nil {
				continue
			}
			backups = append(backups, map[string]any{
				"name":     e.Name(),
				"size":     fi.Size(),
				"mod_time": fi.ModTime().Format(time.RFC3339),
			})
		}
		sort.Slice(backups, func(i, j int) bool {
			return backups[i]["name"].(string) > backups[j]["name"].(string)
		})
		writeJSON(w, map[string]any{"ok": true, "backups": backups})
	}
}


// DBRestoreHandler restores a database from a named backup file.
func DBRestoreHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		var req struct {
			DBName     string `json:"db_name"`
			BackupName string `json:"backup_name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
			return
		}
		if req.DBName == "" || req.BackupName == "" {
			writeJSON(w, map[string]any{"ok": false, "error": "db_name and backup_name required"})
			return
		}
		backup := filepath.Join(dataDir, "db_backups", req.DBName, req.BackupName)
		if _, err := os.Stat(backup); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "backup not found"})
			return
		}
		dst := filepath.Join(dataDir, req.DBName)
		if _, err := os.Stat(dst); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "target db not found"})
			return
		}
		data, err := os.ReadFile(backup)
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		if err := os.WriteFile(dst, data, 0600); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "restored": req.DBName, "from": req.BackupName, "size": len(data)})
	}
}

// DBUploadHandler uploads a backup file.
func DBUploadHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		dbName := r.URL.Query().Get("name")
		if dbName == "" {
			writeJSON(w, map[string]any{"ok": false, "error": "name required"})
			return
		}
		if err := r.ParseMultipartForm(100 << 20); err != nil {
			http.Error(w, "parse form: "+err.Error(), 400)
			return
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "file: "+err.Error(), 400)
			return
		}
		defer file.Close()
		data, err := io.ReadAll(file)
		if err != nil {
			http.Error(w, "read: "+err.Error(), 500)
			return
		}
		dst := filepath.Join(dataDir, dbName)
		if err := os.WriteFile(dst, data, 0600); err != nil {
			http.Error(w, "write: "+err.Error(), 500)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "name": dbName, "size": len(data)})
	}
}
