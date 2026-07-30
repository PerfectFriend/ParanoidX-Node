// Package radio provides the radio streaming and scheduling system
package radio

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"ParanoidX/internal/store"
)

// Language code
type Lang string

const (
	LangEN Lang = "en"
	LangRU Lang = "ru"
	LangES Lang = "es"
)

// StationType category
type StationType string

const (
	StationMusic  StationType = "music"
	StationNews   StationType = "news"
	StationTalk   StationType = "talk"
	StationMixed  StationType = "mixed"
)

// RadioStation represents a streaming radio station on the node.
type RadioStation struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Type        StationType `json:"type"`
	Lang        Lang        `json:"lang"`
	Description string      `json:"description"`
	Icon        string      `json:"icon,omitempty"`
	Enabled     bool        `json:"enabled"`
	CreatedAt   time.Time   `json:"created_at"`
	TrackCount  int         `json:"track_count"`
}

// RadioService manages stations, playlists, and announcements.
type RadioService struct {
	mu        sync.RWMutex
	DataDir   string
	stations  map[string]*RadioStation
	playlists map[string][]string
	store     *store.DB
}


// NewRadioService handles the NewRadioService HTTP request.
func NewRadioService(dataDir string, opts ...*store.DB) *RadioService {
	rs := &RadioService{
		DataDir:   filepath.Join(dataDir, "radio"),
		stations:  make(map[string]*RadioStation),
		playlists: make(map[string][]string),
	}
	if len(opts) > 0 {
		rs.store = opts[0]
	}
	os.MkdirAll(rs.DataDir, 0755)
	rs.load()
	if len(rs.stations) == 0 {
		rs.seedDefaults()
	}
	rs.syncRadioFolder()
	return rs
}

func (rs *RadioService) syncRadioFolder() {
	tracks, err := ScanRadioFolder(rs.DataDir)
	if err != nil || len(tracks) == 0 {
		return
	}
	s, ok := rs.stations["liberty-voice-en"]
	if !ok {
		s = &RadioStation{
			ID: "liberty-voice-en", Name: "Liberty Voice", Type: StationMusic,
			Lang: LangEN, Description: "Music from the island — shuffled playlist of all available tracks.",
			Enabled: true, CreatedAt: time.Now(),
		}
		rs.stations[s.ID] = s
	}
	s.TrackCount = len(tracks)
	if rs.store != nil {
		rs.store.SaveStation(&store.Station{
			ID: s.ID, Name: s.Name, Type: string(s.Type),
			Lang: string(s.Lang), Description: s.Description, Icon: s.Icon,
			Enabled: s.Enabled, CreatedAt: s.CreatedAt, TrackCount: s.TrackCount,
		})
	}
	rs.saveJSON()
	rs.SyncStationContent()
}

// SyncStationContent symlinks audio files from the main radio dir into station dirs.
func (rs *RadioService) SyncStationContent() {
	stationBase := filepath.Join(rs.DataDir, "stations")
	entries, err := os.ReadDir(rs.DataDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !isAudioFile(e.Name()) {
			continue
		}
		// Symlink into each station directory
		src := filepath.Join(rs.DataDir, e.Name())
		for id := range rs.stations {
			stationDir := filepath.Join(stationBase, id)
			os.MkdirAll(stationDir, 0755)
			link := filepath.Join(stationDir, e.Name())
			if _, err := os.Stat(link); os.IsNotExist(err) {
				os.Symlink(src, link)
			}
		}
	}
}

func (rs *RadioService) seedDefaults() {
	// Ensure station directories exist
	stationBase := filepath.Join(rs.DataDir, "stations")
	os.MkdirAll(stationBase, 0755)
	defaults := []RadioStation{
		{
			ID: "liberty-voice-en", Name: "Liberty Voice", Type: StationMixed,
			Lang: LangEN, Description: "The official voice of Saint Mary Liberty Island — news, music, and royal decrees in English.",
			Enabled: true, CreatedAt: time.Now(),
		},
		{
			ID: "liberty-voice-ru", Name: "Голос Свободы", Type: StationMixed,
			Lang: LangRU, Description: "Официальный голос Острова Святой Марии — новости, музыка и королевские указы на русском.",
			Enabled: true, CreatedAt: time.Now(),
		},
		{
			ID: "liberty-voice-es", Name: "Voz de la Libertad", Type: StationMixed,
			Lang: LangES, Description: "La voz oficial de la Isla Santa María — noticias, música y decretos reales en español.",
			Enabled: true, CreatedAt: time.Now(),
		},
		{
			ID: "torquemada-monitor", Name: "Torquemada Monitor", Type: StationNews,
			Lang: LangRU, Description: "Системный мониторинг и отчеты Торквемады о состоянии узла.",
			Enabled: true, CreatedAt: time.Now(),
		},
		{
			ID: "steward-ai", Name: "Steward AI Broadcast", Type: StationTalk,
			Lang: LangEN, Description: "AI-generated market analysis, economic forecasts, and island updates by Steward.",
			Enabled: true, CreatedAt: time.Now(),
		},
	}
	for _, s := range defaults {
		os.MkdirAll(filepath.Join(stationBase, s.ID), 0755)
		rs.stations[s.ID] = &s
		if rs.store != nil {
			rs.store.SaveStation(&store.Station{
				ID: s.ID, Name: s.Name, Type: string(s.Type),
				Lang: string(s.Lang), Description: s.Description, Icon: s.Icon,
				Enabled: s.Enabled, CreatedAt: s.CreatedAt, TrackCount: s.TrackCount,
			})
		}
	}
	rs.saveJSON()
}


// List handles the List HTTP request.
func (rs *RadioService) List() []RadioStation {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	if rs.store != nil {
		list, err := rs.store.ListStations()
		if err == nil {
			out := make([]RadioStation, len(list))
			for i, s := range list {
				out[i] = RadioStation{
					ID: s.ID, Name: s.Name, Type: StationType(s.Type),
					Lang: Lang(s.Lang), Description: s.Description, Icon: s.Icon,
					Enabled: s.Enabled, CreatedAt: s.CreatedAt, TrackCount: s.TrackCount,
				}
			}
			return out
		}
	}
	out := make([]RadioStation, 0, len(rs.stations))
	for _, s := range rs.stations {
		out = append(out, *s)
	}
	return out
}


// Get handles the Get HTTP request.
func (rs *RadioService) Get(id string) *RadioStation {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	if rs.store != nil {
		st, err := rs.store.GetStation(id)
		if err == nil && st != nil {
			return &RadioStation{
				ID: st.ID, Name: st.Name, Type: StationType(st.Type),
				Lang: Lang(st.Lang), Description: st.Description, Icon: st.Icon,
				Enabled: st.Enabled, CreatedAt: st.CreatedAt, TrackCount: st.TrackCount,
			}
		}
	}
	s, ok := rs.stations[id]
	if !ok {
		return nil
	}
	return s
}


// Save handles the Save HTTP request.
func (rs *RadioService) Save(s *RadioStation) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.stations[s.ID] = s
	if rs.store != nil {
		rs.store.SaveStation(&store.Station{
			ID: s.ID, Name: s.Name, Type: string(s.Type),
			Lang: string(s.Lang), Description: s.Description, Icon: s.Icon,
			Enabled: s.Enabled, CreatedAt: s.CreatedAt, TrackCount: s.TrackCount,
		})
	}
	rs.saveJSON()
}

func (rs *RadioService) load() {
	if rs.store != nil {
		list, err := rs.store.ListStations()
		if err == nil {
			for _, st := range list {
				rs.stations[st.ID] = &RadioStation{
					ID: st.ID, Name: st.Name, Type: StationType(st.Type),
					Lang: Lang(st.Lang), Description: st.Description, Icon: st.Icon,
					Enabled: st.Enabled, CreatedAt: st.CreatedAt, TrackCount: st.TrackCount,
				}
			}
			return
		}
	}
	rs.loadJSON()
}

func (rs *RadioService) loadJSON() {
	b, err := os.ReadFile(filepath.Join(rs.DataDir, "stations.json"))
	if err != nil {
		return
	}
	var list []RadioStation
	json.Unmarshal(b, &list)
	for _, s := range list {
		rs.stations[s.ID] = &s
	}
}

func (rs *RadioService) saveJSON() {
	list := make([]RadioStation, 0, len(rs.stations))
	for _, s := range rs.stations {
		list = append(list, *s)
	}
	b, _ := json.MarshalIndent(list, "", "  ")
	os.WriteFile(filepath.Join(rs.DataDir, "stations.json"), b, 0644)
}
