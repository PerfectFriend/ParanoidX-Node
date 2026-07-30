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

// Announcer identifies who generated the announcement.
type Announcer string

const (
	AnnouncerKing      Announcer = "king"
	AnnouncerTorquemada Announcer = "torquemada"
	AnnouncerSteward   Announcer = "steward"
)

// AnnouncementPriority for scheduling.
type AnnouncementPriority int

const (
	PriorityLow    AnnouncementPriority = 0
	PriorityNormal AnnouncementPriority = 1
	PriorityHigh   AnnouncementPriority = 2
	PriorityUrgent AnnouncementPriority = 3
)

// Announcement is a radio broadcast message.
type Announcement struct {
	ID         string               `json:"id"`
	Announcer  Announcer            `json:"announcer"`
	Title      string               `json:"title"`
	Body       string               `json:"body"`
	Lang       Lang                 `json:"lang"`
	Priority   AnnouncementPriority `json:"priority"`
	Stations   []string             `json:"stations,omitempty"` // empty = all
	AudioFile  string               `json:"audio_file,omitempty"`
	Paid       bool                 `json:"paid"`
	PaidAmount int64                `json:"paid_amount_ng,omitempty"`
	CreatedAt  time.Time            `json:"created_at"`
	ScheduledAt *time.Time          `json:"scheduled_at,omitempty"`
	PlayedAt   *time.Time           `json:"played_at,omitempty"`
}

// AnnouncementStore manages announcements (persisted to disk or SQLite).
type AnnouncementStore struct {
	mu            sync.RWMutex
	DataDir       string
	announcements []Announcement
	pending       []Announcement
	store         *store.DB
}


// NewAnnouncementStore handles the NewAnnouncementStore HTTP request.
func NewAnnouncementStore(dataDir string, opts ...*store.DB) *AnnouncementStore {
	as := &AnnouncementStore{
		DataDir: filepath.Join(dataDir, "radio"),
	}
	if len(opts) > 0 {
		as.store = opts[0]
	}
	os.MkdirAll(as.DataDir, 0755)
	as.load()
	return as
}


// Add handles the Add HTTP request.
func (as *AnnouncementStore) Add(a Announcement) {
	as.mu.Lock()
	defer as.mu.Unlock()
	as.announcements = append(as.announcements, a)
	if a.ScheduledAt == nil || a.ScheduledAt.Before(time.Now()) {
		as.pending = append(as.pending, a)
	}
	as.saveJSON()
}


// Pending handles the Pending HTTP request.
func (as *AnnouncementStore) Pending() []Announcement {
	as.mu.RLock()
	defer as.mu.RUnlock()
	out := make([]Announcement, len(as.pending))
	copy(out, as.pending)
	return out
}


// MarkPlayed handles the MarkPlayed HTTP request.
func (as *AnnouncementStore) MarkPlayed(id string) {
	as.mu.Lock()
	defer as.mu.Unlock()
	for i, a := range as.pending {
		if a.ID == id {
			now := time.Now()
			as.pending[i].PlayedAt = &now
			as.announcements = append(as.announcements, as.pending[i])
			as.pending = append(as.pending[:i], as.pending[i+1:]...)
			break
		}
	}
	as.saveJSON()
}


// History handles the History HTTP request.
func (as *AnnouncementStore) History(limit int) []Announcement {
	as.mu.RLock()
	defer as.mu.RUnlock()
	n := len(as.announcements)
	if limit > 0 && limit < n {
		n = limit
	}
	out := make([]Announcement, n)
	copy(out, as.announcements[:n])
	return out
}

func (as *AnnouncementStore) load() {
	if as.store != nil {
		// TODO: load announcements from SQLite in future cycle
		return
	}
	as.loadJSON()
}

func (as *AnnouncementStore) loadJSON() {
	b, err := os.ReadFile(filepath.Join(as.DataDir, "announcements.json"))
	if err != nil {
		return
	}
	json.Unmarshal(b, &as.announcements)
	for _, a := range as.announcements {
		if a.PlayedAt == nil {
			as.pending = append(as.pending, a)
		}
	}
}

func (as *AnnouncementStore) saveJSON() {
	list := as.announcements
	b, _ := json.MarshalIndent(list, "", "  ")
	os.WriteFile(filepath.Join(as.DataDir, "announcements.json"), b, 0644)
}
