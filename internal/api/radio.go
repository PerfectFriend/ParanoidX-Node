// Package api provides HTTP handlers and API endpoints for the simplex-node server
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ParanoidX/internal/radio"
	"ParanoidX/internal/radio/acestep"
)

// formulaPlaylistJSON returns a JSON-serializable formula playlist.
func formulaPlaylistJSON(dataDir string) []map[string]any {
	tracks := radio.BuildFormulaPlaylist(dataDir)
	out := make([]map[string]any, len(tracks))
	for i, t := range tracks {
		m := map[string]any{
			"id":          t.ID,
			"title":       t.Title,
			"duration":    t.Duration,
			"is_ad":       t.IsAd,
			"is_announce": t.IsAnnounce,
			"kind":        t.Kind,
			"file_path":   t.FilePath,
		}
		if t.FilePath != "" {
			m["stream_url"] = "/api/radio/track?path=" + t.FilePath
		}
		out[i] = m
	}
	return out
}

// RadioHandler returns an http.HandlerFunc for /api/radio/*
func RadioHandler(rs *radio.RadioService, as *radio.AnnouncementStore) http.HandlerFunc {
	// rs.DataDir is radio/ subfolder; PlaylistBuilder expects the parent
	baseDir := filepath.Dir(rs.DataDir)
	pb := radio.NewPlaylistBuilder(baseDir, as)
	return func(w http.ResponseWriter, r *http.Request) {
		action := r.URL.Query().Get("action")

		switch action {
		case "stations":
			stationsHandler(rs, w, r)
		case "playlist":
			playlistHandler(rs, pb, w, r)
		case "formula":
			formulaHandler(rs, w, r)
		case "m3u8":
			// rs.DataDir is radio/ subfolder; parent is needed
			baseDir := filepath.Dir(rs.DataDir)
			M3U8Handler(baseDir)(w, r)
		case "announce":
			announceHandler(as, w, r)
		case "announcements":
			announcementsHandler(as, w, r)
		case "history":
			historyHandler(as, w, r)
		case "upload":
			uploadHandler(rs, w, r)
		case "upload-file":
			uploadFileHandler(rs, w, r)
		default:
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"stations": rs.List(),
				"actions":  []string{"stations", "playlist", "formula", "m3u8", "announce", "announcements", "history", "upload", "upload-file"},
			})
		}
	}
}

// AcestepHandler returns an http.HandlerFunc for /api/radio/acestep/*
// Requires an acestep.Generator to be provided (may be nil).
func AcestepHandler(gen *acestep.Generator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if gen == nil {
			writeJSON(w, map[string]any{"error": "acestep not configured", "status": "unavailable"})
			return
		}

		action := r.URL.Query().Get("action")

		switch action {
		case "status":
			acestepStatusHandler(gen, w, r)
		case "generate":
			acestepGenerateHandler(gen, w, r)
		case "healthy":
			writeJSON(w, map[string]any{"healthy": gen.Healthy()})
		default:
			writeJSON(w, map[string]any{
				"status":  "acestep ready",
				"healthy": gen.Healthy(),
				"actions": []string{"status", "generate", "healthy"},
			})
		}
	}
}

// LiveBroadcastHandler provides access to the Acestep live broadcast stream.
func LiveBroadcastHandler(broadcast *acestep.LiveBroadcast) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if broadcast == nil {
			writeJSON(w, map[string]any{"error": "broadcast not available"})
			return
		}

		action := r.URL.Query().Get("action")
		switch action {
		case "status":
			writeJSON(w, broadcast.Status())
		case "stream":
			// Stream the next available track as audio/mpeg
			path := broadcast.NextTrack()
			if path == "" {
				http.Error(w, "no track available", 503)
				return
			}
			http.ServeFile(w, r, path)
		default:
			writeJSON(w, broadcast.Status())
		}
	}
}

func acestepStatusHandler(gen *acestep.Generator, w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"healthy": gen.Healthy(),
		"styles": []string{"gospel", "tragedy", "royal", "music"},
		"info": "Acestep AI Radio Generator — превращает промпты в аудио. " +
			"ADVE → gospel проповедь. NEWS → tragedy голос. KING → royal decree. MUSIC → island vibes.",
	})
}

type acestepGenerateReq struct {
	Prompt   string `json:"prompt"`
	Style    string `json:"style"`
	Duration int    `json:"duration"`
}

func acestepGenerateHandler(gen *acestep.Generator, w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST required", 405)
		return
	}

	var req acestepGenerateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]any{"error": "bad request: " + err.Error()})
		return
	}

	if req.Prompt == "" {
		writeJSON(w, map[string]any{"error": "prompt required"})
		return
	}

	style := acestep.Style(req.Style)
	if style == "" {
		style = acestep.StyleGospel
	}
	if req.Duration <= 0 {
		req.Duration = 30
	}

	path, err := gen.Generate(req.Prompt, style, req.Duration)
	if err != nil {
		writeJSON(w, map[string]any{"error": err.Error(), "style": style, "prompt": req.Prompt})
		return
	}

	writeJSON(w, map[string]any{
		"ok":      true,
		"file":    path,
		"style":   style,
		"prompt":  req.Prompt,
		"message": "🎵 Трек сгенерирован! Скоро в эфире на Острове Святой Марии.",
	})
}

// M3U8Handler generates an M3U8 live playlist for just_audio.
func M3U8Handler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tracks := radio.BuildFormulaPlaylist(dataDir)
		if len(tracks) == 0 {
			http.Error(w, "no tracks", 404)
			return
		}

		w.Header().Set("Content-Type", "application/x-mpegurl")
		w.Header().Set("Cache-Control", "no-cache")

		fmt.Fprintln(w, "#EXTM3U")
		fmt.Fprintln(w, "#EXT-X-VERSION:3")
		fmt.Fprintln(w, "#EXT-X-TARGETDURATION:120")
		fmt.Fprintln(w, "#EXT-X-MEDIA-SEQUENCE:0")
		fmt.Fprintln(w, "#EXT-X-DISCONTINUITY-SEQUENCE:0")

		for _, t := range tracks {
			relPath := strings.TrimPrefix(t.FilePath, dataDir+"/radio/stations/")
			url := fmt.Sprintf("/api/radio/track?path=%s", relPath)
			dur := t.Duration
			if dur <= 0 {
				dur = 60
			}
			fmt.Fprintf(w, "#EXTINF:%d,%s\n", dur, t.Title)
			fmt.Fprintf(w, "%s\n", url)
		}
		fmt.Fprintln(w, "#EXT-X-ENDLIST")
	}
}

func formulaHandler(rs *radio.RadioService, w http.ResponseWriter, r *http.Request) {
	// rs.DataDir is already "radio/" subfolder; we need the parent
	baseDir := filepath.Dir(rs.DataDir)
	playlist := formulaPlaylistJSON(baseDir)
	writeJSON(w, map[string]any{
		"playlist": playlist,
		"count":    len(playlist),
		"formula":  "music -> 3xADVE -> NEWS* -> 2xADVE -> KING* -> ADVE -> next music -> repeat",
	})
}

func stationsHandler(rs *radio.RadioService, w http.ResponseWriter, r *http.Request) {
	lang := r.URL.Query().Get("lang")
	stations := rs.List()
	if lang != "" {
		var filtered []radio.RadioStation
		for _, s := range stations {
			if string(s.Lang) == lang {
				filtered = append(filtered, s)
			}
		}
		stations = filtered
	}
	writeJSON(w, map[string]any{"stations": stations, "count": len(stations)})
}

func playlistHandler(rs *radio.RadioService, pb *radio.PlaylistBuilder, w http.ResponseWriter, r *http.Request) {
	stationID := r.URL.Query().Get("station")
	if stationID == "" {
		http.Error(w, "station parameter required", http.StatusBadRequest)
		return
	}
	station := rs.Get(stationID)
	if station == nil {
		http.Error(w, "station not found", http.StatusNotFound)
		return
	}
	playlist := pb.CurrentPlaylistJSON(stationID)
	writeJSON(w, map[string]any{
		"station":  station,
		"playlist": playlist,
		"count":    len(playlist),
	})
}

func announceHandler(as *radio.AnnouncementStore, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var a radio.Announcement
	if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if a.Title == "" || a.Body == "" {
		http.Error(w, "title and body required", http.StatusBadRequest)
		return
	}
	if a.Announcer == "" {
		a.Announcer = radio.AnnouncerSteward
	}
	a.ID = fmt.Sprintf("ann-%d", time.Now().UnixNano())
	a.CreatedAt = time.Now()
	as.Add(a)
	writeJSON(w, map[string]any{"ok": true, "id": a.ID})
}

func announcementsHandler(as *radio.AnnouncementStore, w http.ResponseWriter, r *http.Request) {
	pending := as.Pending()
	writeJSON(w, map[string]any{"pending": pending, "count": len(pending)})
}

func historyHandler(as *radio.AnnouncementStore, w http.ResponseWriter, r *http.Request) {
	history := as.History(50)
	writeJSON(w, map[string]any{"history": history, "count": len(history)})
}

func uploadHandler(rs *radio.RadioService, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	stationID := r.URL.Query().Get("station")
	if stationID == "" {
		http.Error(w, "station parameter required", http.StatusBadRequest)
		return
	}
	r.ParseMultipartForm(50 << 20) // 50 MB max
	file, header, err := r.FormFile("audio")
	if err != nil {
		http.Error(w, "audio file required: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	stationDir := filepath.Join(rs.DataDir, "stations", stationID)
	os.MkdirAll(stationDir, 0755)
	dst := filepath.Join(stationDir, header.Filename)
	f, err := os.Create(dst)
	if err != nil {
		http.Error(w, "write error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer f.Close()
	if _, err := f.ReadFrom(file); err != nil {
		http.Error(w, "save error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "file": header.Filename, "size": header.Size})
}

func uploadFileHandler(rs *radio.RadioService, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	r.ParseMultipartForm(50 << 20)
	file, header, err := r.FormFile("audio")
	if err != nil {
		http.Error(w, "audio file required: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()
	dst := filepath.Join(rs.DataDir, header.Filename)
	f, err := os.Create(dst)
	if err != nil {
		http.Error(w, "write error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer f.Close()
	if _, err := f.ReadFrom(file); err != nil {
		http.Error(w, "save error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// Resync station symlinks
	rs.SyncStationContent()
	writeJSON(w, map[string]any{"ok": true, "file": header.Filename, "size": header.Size})
}

// TrackStreamHandler serves audio files for the radio player (supports Range requests).
func TrackStreamHandler(dataDir string) http.HandlerFunc {
	radioDir := filepath.Join(dataDir, "radio")
	return func(w http.ResponseWriter, r *http.Request) {
		relPath := r.URL.Query().Get("path")
		if relPath == "" || strings.Contains(relPath, "..") {
			http.Error(w, "invalid path", http.StatusBadRequest)
			return
		}
		var fullPath string
		if filepath.IsAbs(relPath) {
			fullPath = relPath
		} else {
			fullPath = filepath.Join(radioDir, "stations", relPath)
		}
		// Security: ensure we're inside radioDir
		absRadio, _ := filepath.Abs(radioDir)
		absFull, _ := filepath.Abs(fullPath)
		if !strings.HasPrefix(absFull, absRadio) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		http.ServeFile(w, r, fullPath)
	}
}

// StreamHandler serves a continuous MP3 stream from the flat radio folder.
// It shuffles all audio files, streams them one by one, and auto-repeats.
// Supports ?shuffle=true (default) and ?repeat=true (default) query params.
func StreamHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		shuffle := r.URL.Query().Get("shuffle") != "false"
		repeat := r.URL.Query().Get("repeat") != "false"

		// Build initial shuffled playlist
		tracks := radio.BuildRadioPlaylist(dataDir, shuffle, repeat)
		if len(tracks) == 0 {
			http.Error(w, "no audio files found in radio directory", http.StatusNotFound)
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "audio/mpeg")
		w.Header().Set("Transfer-Encoding", "chunked")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		ctx := r.Context()
		index := 0

		for {
			if ctx.Err() != nil {
				return
			}

			// If we've gone through the entire playlist, rebuild and reshuffle
			if index >= len(tracks) {
				tracks = radio.BuildRadioPlaylist(dataDir, shuffle, repeat)
				index = 0
				if len(tracks) == 0 {
					return
				}
			}

			t := tracks[index]
			index++

			if t.FilePath == "" {
				continue
			}

			f, err := os.Open(t.FilePath)
			if err != nil {
				continue
			}
			defer f.Close()

			buf := make([]byte, 64*1024)
			for {
				if ctx.Err() != nil {
					return
				}
				n, err := f.Read(buf)
				if n > 0 {
					if _, werr := w.Write(buf[:n]); werr != nil {
						return
					}
					flusher.Flush()
				}
				if err != nil {
					break
				}
			}
			f.Close()
		}
	}
}

// OnionStreamHandler returns an M3U8 playlist with onion-routed track URLs
// for playing radio over Tor. Lists all audio files in the radio directory.
func OnionStreamHandler(dataDir, onionAddr string) http.HandlerFunc {
	radioDir := filepath.Join(dataDir, "radio")
	return func(w http.ResponseWriter, r *http.Request) {
		if onionAddr == "" {
			http.Error(w, "onion address not available", http.StatusServiceUnavailable)
			return
		}
		entries, err := os.ReadDir(radioDir)
		if err != nil {
			http.Error(w, "no radio directory", http.StatusNotFound)
			return
		}
		var tracks []string
		for _, e := range entries {
			if e.IsDir() || !isAudioFile(e.Name()) {
				continue
			}
			tracks = append(tracks, e.Name())
		}
		if len(tracks) == 0 {
			http.Error(w, "no audio files", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/x-mpegurl")
		w.Header().Set("Content-Disposition", "inline; filename=\"radio-onion.m3u8\"")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "#EXTM3U\n")
		fmt.Fprint(w, "#PLAYLIST:simplex-node Radio (Tor Onion)\n")
		for _, t := range tracks {
			title := strings.TrimSuffix(t, filepath.Ext(t))
			fmt.Fprintf(w, "#EXTINF:60,%s\n", title)
			fmt.Fprintf(w, "http://%s/api/radio/track?path=%s\n", onionAddr, t)
		}
	}
}

// ── Radio Content Schedule (C33) ───────────────────────────────────────────────

var GlobalContentScheduler *radio.EnhancedScheduler
var GlobalRadioAIGen *radio.AIContentGenerator


// RadioScheduleHandler handles the RadioScheduleHandler HTTP request.
func RadioScheduleHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			if GlobalContentScheduler == nil {
				writeJSON(w, map[string]any{"ok": true, "schedules": []any{}})
				return
			}
			writeJSON(w, map[string]any{"ok": true, "schedules": GlobalContentScheduler.List()})
		case "POST":
			var req struct {
				Type     string `json:"type"`
				CronMin  int    `json:"cron_min"`
				Prompt   string `json:"prompt"`
				Priority int    `json:"priority,omitempty"`
				Weight   int    `json:"weight,omitempty"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, map[string]any{"ok": false, "error": "bad json"})
				return
			}
			if req.Type == "" || req.CronMin <= 0 || req.Prompt == "" {
				writeJSON(w, map[string]any{"ok": false, "error": "type, cron_min, and prompt required"})
				return
			}
			validTypes := map[string]bool{"ad": true, "announce": true, "music": true, "news": true}
			if !validTypes[req.Type] {
				writeJSON(w, map[string]any{"ok": false, "error": "type must be ad, announce, music, or news"})
				return
			}
			if GlobalContentScheduler == nil {
				writeJSON(w, map[string]any{"ok": false, "error": "content scheduler not initialized"})
				return
			}
			entry := radio.ContentScheduleEntry{
				ID:       fmt.Sprintf("sched-%d", time.Now().UnixNano()),
				Type:     req.Type,
				CronMin:  req.CronMin,
				Prompt:   req.Prompt,
				Priority: req.Priority,
				Weight:   req.Weight,
			}
			if entry.Weight <= 0 {
				entry.Weight = 1
			}
			GlobalContentScheduler.Add(entry)
			logAudit("radio_schedule_create", "admin", "type="+req.Type+" prompt="+req.Prompt)
			writeJSON(w, map[string]any{"ok": true, "schedule": entry})
		case "DELETE":
			id := r.URL.Query().Get("id")
			if id == "" {
				writeJSON(w, map[string]any{"ok": false, "error": "id required"})
				return
			}
			if GlobalContentScheduler == nil {
				writeJSON(w, map[string]any{"ok": false, "error": "content scheduler not initialized"})
				return
			}
			if GlobalContentScheduler.Remove(id) {
				writeJSON(w, map[string]any{"ok": true, "removed": id})
			} else {
				writeJSON(w, map[string]any{"ok": false, "error": "schedule not found"})
			}
		default:
			http.Error(w, "GET/POST/DELETE", 405)
		}
	}
}

func isAudioFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".mp3", ".ogg", ".wav", ".flac", ".aac", ".m4a", ".webm":
		return true
	}
	return false
}
