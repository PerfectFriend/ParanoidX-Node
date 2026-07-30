// Package radio provides the radio streaming and scheduling system
package radio

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"ParanoidX/internal/radio/acestep"
)

// TrackKind classifies audio by filename prefix.
type TrackKind string

const (
	KindMusic   TrackKind = "music"
	KindAd      TrackKind = "ad"
	KindNews    TrackKind = "news"
	KindKing    TrackKind = "king"
	KindAIGospel TrackKind = "ai_gospel"
	KindAINews  TrackKind = "ai_news"
	KindAIKing  TrackKind = "ai_king"
	KindAIMusic TrackKind = "ai_music"
)

// Track represents a playable audio file in the station's playlist.
type Track struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	FilePath   string    `json:"file_path"`
	Duration   int       `json:"duration_sec"`
	IsAd       bool      `json:"is_ad"`
	IsAnnounce bool      `json:"is_announce"`
	Kind       TrackKind `json:"kind"`
	Lang       Lang      `json:"lang"`
	AddedAt    time.Time `json:"added_at"`
	IsAI       bool      `json:"is_ai"`
}

// PlaylistBuilder constructs a dynamic playlist for a station,
// interleaving music tracks with pending announcements and ads.
// Optionally injects AI-generated tracks via Acestep.
type PlaylistBuilder struct {
	DataDir       string
	AnnounceStore *AnnouncementStore
	Acestep       *acestep.Generator
}


// NewPlaylistBuilder handles the NewPlaylistBuilder HTTP request.
func NewPlaylistBuilder(dataDir string, as *AnnouncementStore, opts ...*acestep.Generator) *PlaylistBuilder {
	pb := &PlaylistBuilder{
		DataDir:       filepath.Join(dataDir, "radio"),
		AnnounceStore: as,
	}
	if len(opts) > 0 {
		pb.Acestep = opts[0]
	}
	return pb
}

// Build returns the playlist for a given station at this moment.
func (pb *PlaylistBuilder) Build(stationID string) []Track {
	var playlist []Track

	// 1. Music tracks from station directory
	stationDir := filepath.Join(pb.DataDir, "stations", stationID)
	entries, err := os.ReadDir(stationDir)
	if err == nil {
		for _, e := range entries {
			if e.IsDir() || !isAudioFile(e.Name()) {
				continue
			}
			playlist = append(playlist, Track{
				ID:       fmt.Sprintf("%s-%s", stationID, e.Name()),
				Title:    trackTitle(e.Name()),
				FilePath: filepath.Join(stationDir, e.Name()),
				Duration: 60,
				IsAd:     false,
				IsAnnounce: false,
				AddedAt:  time.Now(),
			})
		}
	}

	// 2. Fallback: scan main radio dir for music if station dir is empty
	if len(playlist) == 0 {
		radioDir := pb.DataDir // DataDir is radio/ subfolder
		rootEntries, rootErr := os.ReadDir(radioDir)
		if rootErr == nil {
			for _, e := range rootEntries {
				if e.IsDir() || !isAudioFile(e.Name()) || strings.HasPrefix(strings.ToUpper(e.Name()), "ADVE") || strings.HasPrefix(strings.ToUpper(e.Name()), "NEWS") || strings.HasPrefix(strings.ToUpper(e.Name()), "KING") {
					continue
				}
				playlist = append(playlist, Track{
					ID:       fmt.Sprintf("%s-%s", stationID, e.Name()),
					Title:    trackTitle(e.Name()),
					FilePath: filepath.Join(radioDir, e.Name()),
					Duration: 60,
					IsAd:     false,
					IsAnnounce: false,
					AddedAt:  time.Now(),
				})
			}
		}
	}

	// 4. Placeholder when nothing is available
	if len(playlist) == 0 {
		playlist = append(playlist, Track{
			ID:          stationID + "-placeholder",
			Title:       fmt.Sprintf("📻 %s — No tracks yet", stationID),
			FilePath:    "",
			Duration:    30,
			IsAnnounce:  true,
			AddedAt:     time.Now(),
		})
	}

	// 3. Interleave pending announcements for this station
	lang := stationLang(stationID)
	pending := pb.AnnounceStore.Pending()
	for _, a := range pending {
		if len(a.Stations) > 0 && !contains(a.Stations, stationID) {
			continue
		}
		if a.Lang != "" && a.Lang != lang {
			continue
		}
		if a.PlayedAt != nil {
			continue
		}
		announceTrack := Track{
			ID:         "ann-" + a.ID,
			Title:      fmt.Sprintf("📢 %s: %s", announcerLabel(a.Announcer), a.Title),
			FilePath:   a.AudioFile,
			Duration:   30,
			IsAd:       a.Paid,
			IsAnnounce: true,
			Lang:       a.Lang,
			AddedAt:    a.CreatedAt,
		}
		// Insert announcement after every 3rd track
		insertAt := 3
		if len(playlist) > insertAt {
			playlist = append(playlist[:insertAt+1], append([]Track{announceTrack}, playlist[insertAt+1:]...)...)
		} else {
			playlist = append(playlist, announceTrack)
		}
	}

	return playlist
}

// CurrentPlaylistJSON returns a JSON-serializable playlist for API responses.
func (pb *PlaylistBuilder) CurrentPlaylistJSON(stationID string) []map[string]any {
	tracks := pb.Build(stationID)
	out := make([]map[string]any, len(tracks))
	for i, t := range tracks {
		m := map[string]any{
			"id":        t.ID,
			"title":     t.Title,
			"duration":  t.Duration,
			"is_ad":     t.IsAd,
			"is_announce": t.IsAnnounce,
		}
		if t.FilePath != "" {
			m["stream_url"] = "/api/radio/track?path=" + t.FilePath
		}
		out[i] = m
	}
	return out
}

func isAudioFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".mp3", ".ogg", ".wav", ".flac", ".aac", ".m4a", ".webm":
		return true
	}
	return false
}

func trackTitle(name string) string {
	ext := filepath.Ext(name)
	base := name[:len(name)-len(ext)]
	// Strip prefix tag for display
	if idx := strings.Index(base, "-"); idx > 0 {
		prefix := base[:idx]
		switch strings.ToUpper(prefix) {
		case "ADVE", "NEWS", "KING":
			base = base[idx+1:]
		}
	}
	return strings.ReplaceAll(base, "-", " ")
}

func detectKind(name string) TrackKind {
	base := strings.ToUpper(name[:len(name)-len(filepath.Ext(name))])
	switch {
	case strings.HasPrefix(base, "ADVE"):
		return KindAd
	case strings.HasPrefix(base, "NEWS"):
		return KindNews
	case strings.HasPrefix(base, "KING"):
		return KindKing
	default:
		return KindMusic
	}
}

// BuildFormulaPlaylist scans the radio directory and builds a playlist
// following the station formula: music → 3 ads → all news → 2 ads →
// all KING → 1 ad → next music (shuffled, non-repeating) → repeat.
func BuildFormulaPlaylist(dataDir string) []Track {
	// dataDir is the base data directory (parent of "radio/")
	radioDir := filepath.Join(dataDir, "radio")
	entries, err := os.ReadDir(radioDir)
	if err != nil {
		return nil
	}
	var music, ads, news, king []Track
	for _, e := range entries {
		if e.IsDir() || !isAudioFile(e.Name()) {
			continue
		}
		kind := detectKind(e.Name())
		t := Track{
			ID:       "radio-" + e.Name(),
			Title:    trackTitle(e.Name()),
			FilePath: filepath.Join(radioDir, e.Name()),
			Duration: 60,
			Kind:     kind,
			IsAd:     kind == KindAd,
			IsAnnounce: kind != KindMusic,
			AddedAt:  time.Now(),
		}
		switch kind {
		case KindAd:
			ads = append(ads, t)
		case KindNews:
			news = append(news, t)
		case KindKing:
			king = append(king, t)
		default:
			music = append(music, t)
		}
	}
	ShuffleTracks(music)
	ShuffleTracks(ads)
	ShuffleTracks(news)
	ShuffleTracks(king)

	var playlist []Track
	musicIdx := 0
	adIdx := 0
	for musicIdx < len(music) {
		// One music track
		playlist = append(playlist, music[musicIdx])
		musicIdx++

		// 3 ads
		for i := 0; i < 3 && len(ads) > 0; i++ {
			playlist = append(playlist, ads[adIdx%len(ads)])
			adIdx++
		}
		// All news (only once per cycle)
		newsCopy := make([]Track, len(news))
		copy(newsCopy, news)
		playlist = append(playlist, newsCopy...)
		// 2 ads
		for i := 0; i < 2 && len(ads) > 0; i++ {
			playlist = append(playlist, ads[adIdx%len(ads)])
			adIdx++
		}
		// All KING (only once per cycle)
		kingCopy := make([]Track, len(king))
		copy(kingCopy, king)
		playlist = append(playlist, kingCopy...)
		// 1 ad
		if len(ads) > 0 {
			playlist = append(playlist, ads[adIdx%len(ads)])
			adIdx++
		}
	}
	return playlist
}

func stationLang(stationID string) Lang {
	switch {
	case strings.Contains(stationID, "-ru"):
		return LangRU
	case strings.Contains(stationID, "-es"):
		return LangES
	default:
		return LangEN
	}
}

func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

func announcerLabel(a Announcer) string {
	switch a {
	case AnnouncerKing:
		return "👑 Король"
	case AnnouncerTorquemada:
		return "🔧 Торквемада"
	case AnnouncerSteward:
		return "🤖 Стюард"
	}
	return string(a)
}

// SortTracksByAdded sorts tracks newest-first.
func SortTracksByAdded(tracks []Track) {
	sort.Slice(tracks, func(i, j int) bool {
		return tracks[i].AddedAt.After(tracks[j].AddedAt)
	})
}

// ShuffleTracks randomizes the order of tracks in-place.
func ShuffleTracks(tracks []Track) {
	rand.Shuffle(len(tracks), func(i, j int) {
		tracks[i], tracks[j] = tracks[j], tracks[i]
	})
}

// ScanRadioFolder scans the flat radio/audio directory for audio files.
// Returns a list of tracks found in the given directory.
func ScanRadioFolder(dir string) ([]Track, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var tracks []Track
	for _, e := range entries {
		if e.IsDir() || !isAudioFile(e.Name()) {
			continue
		}
		tracks = append(tracks, Track{
			ID:       "radio-" + e.Name(),
			Title:    trackTitle(e.Name()),
			FilePath: filepath.Join(dir, e.Name()),
			Duration: 60,
			IsAd:     false,
			IsAnnounce: false,
			AddedAt:  time.Now(),
		})
	}
	return tracks, nil
}

// BuildRadioPlaylist creates a shuffled playlist from all mp3 files
// found in the flat radio folder. If shuffle is true, randomizes order.
// If repeat is true, the playlist auto-repeats (appends a copy of itself).
func BuildRadioPlaylist(dataDir string, shuffle, repeat bool) []Track {
	radioDir := filepath.Join(dataDir, "radio")
	entries, err := os.ReadDir(radioDir)
	if err != nil {
		return nil
	}
	var tracks []Track
	for _, e := range entries {
		if e.IsDir() || !isAudioFile(e.Name()) {
			continue
		}
		kind := detectKind(e.Name())
		tracks = append(tracks, Track{
			ID:       "radio-" + e.Name(),
			Title:    trackTitle(e.Name()),
			FilePath: filepath.Join(radioDir, e.Name()),
			Duration: 60,
			Kind:     kind,
			IsAd:     kind == KindAd,
			AddedAt:  time.Now(),
		})
	}
	if shuffle {
		ShuffleTracks(tracks)
	}
	if repeat && len(tracks) > 0 {
		repeatCopy := make([]Track, len(tracks))
		copy(repeatCopy, tracks)
		if shuffle {
			ShuffleTracks(repeatCopy)
		}
		tracks = append(tracks, repeatCopy...)
	}
	return tracks
}

// InjectAITracks adds AI-generated tracks into the playlist if the Acestep
// generator is available. Injects gospel ads, tragic news, royal decrees,
// and AI music. Returns the number of AI tracks injected.
func InjectAITracks(gen *acestep.Generator, playlist []Track) ([]Track, int) {
	if gen == nil || !gen.Healthy() {
		return playlist, 0
	}

	var count int

	// Inject AI gospel ad after every 5th music track
	aiAd := gen.GetCached(string(acestep.StyleGospel))
	if aiAd == "" {
		return playlist, 0
	}

	// Inject AI tracks into playlist
	result := make([]Track, 0, len(playlist))
	musicCount := 0
	for i, t := range playlist {
		result = append(result, t)
		if t.Kind == KindMusic {
			musicCount++
			if musicCount%5 == 0 {
				result = append(result, Track{
					ID:       "ai-gospel-" + fmt.Sprint(time.Now().UnixNano()),
					Title:    "☆ AI Проповедь Спасения ☆",
					FilePath: aiAd,
					Duration: 30,
					Kind:     KindAIGospel,
					IsAI:     true,
					AddedAt:  time.Now(),
				})
				count++
			}
		}
		// Inject AI news before regular news
		if t.Kind == KindNews && i > 0 {
			aiNews := gen.GetCached(string(acestep.StyleTragedy))
			if aiNews != "" && count < 5 {
				result = append(result, Track{
					ID:       "ai-news-" + fmt.Sprint(time.Now().UnixNano()),
					Title:    "⛔ BREAKING NEWS AI ⛔",
					FilePath: aiNews,
					Duration: 20,
					Kind:     KindAINews,
					IsAI:     true,
					AddedAt:  time.Now(),
				})
				count++
			}
		}
	}

	// Inject AI music as intro if any
	if len(result) > 0 {
		aiMusic := gen.GetCached(string(acestep.StyleMusic))
		if aiMusic != "" {
			intro := Track{
				ID:       "ai-intro-" + fmt.Sprint(time.Now().UnixNano()),
				Title:    "🎵 Остров Святой Марии — Эфир 🎵",
				FilePath: aiMusic,
				Duration: 120,
				Kind:     KindAIMusic,
				IsAI:     true,
				AddedAt:  time.Now(),
			}
			result = append([]Track{intro}, result...)
			count++
		}
	}

	return result, count
}
