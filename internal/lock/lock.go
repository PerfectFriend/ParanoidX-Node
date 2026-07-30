// Package lock provides mutex-based file locking for cross-process synchronization
package lock

import (
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"ParanoidX/internal/fileutil"
)

type State struct {
	IsLocked bool   `json:"is_locked"`
	Code     string `json:"code"`
}

type Service struct {
	mu          sync.RWMutex
	state       State
	lockFile    string
	unlockFails int
	unlockMu    sync.Mutex
}


// New handles the New HTTP request.
func New(dataDir string) *Service {
	s := &Service{
		state: State{IsLocked: false, Code: ""},
	}
	s.lockFile = filepath.Join(dataDir, "lock.json")
	fileutil.ReadJSON(s.lockFile, &s.state)
	if s.state.Code == "" {
		s.state.Code = hashBcrypt("123456")
		s.save()
	} else if !isBcrypt(s.state.Code) {
		s.state.Code = hashBcrypt("123456")
		s.save()
	}
	return s
}

func isBcrypt(hash string) bool {
	return len(hash) == 60 && hash[:4] == "$2a$"
}

func hashBcrypt(code string) string {
	h, err := bcrypt.GenerateFromPassword([]byte(code), bcrypt.DefaultCost)
	if err != nil {
		panic("bcrypt hash failed: " + err.Error())
	}
	return string(h)
}

func (s *Service) save() {
	fileutil.WriteJSON(s.lockFile, s.state)
}


// IsLocked handles the IsLocked HTTP request.
func (s *Service) IsLocked() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state.IsLocked
}


// Lock handles the Lock HTTP request.
func (s *Service) Lock() {
	s.mu.Lock()
	s.state.IsLocked = true
	s.save()
	s.mu.Unlock()
}


// ValidateUnlock handles the ValidateUnlock HTTP request.
func (s *Service) ValidateUnlock(code string) bool {
	s.unlockMu.Lock()
	if s.unlockFails >= 5 {
		s.unlockMu.Unlock()
		time.Sleep(5 * time.Second)
		return false
	}
	s.unlockMu.Unlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	if bcrypt.CompareHashAndPassword([]byte(s.state.Code), []byte(code)) != nil {
		s.unlockMu.Lock()
		s.unlockFails++
		s.unlockMu.Unlock()
		return false
	}

	s.state.IsLocked = false
	s.save()
	s.unlockMu.Lock()
	s.unlockFails = 0
	s.unlockMu.Unlock()
	return true
}


// ChangeCode handles the ChangeCode HTTP request.
func (s *Service) ChangeCode(current, newCode string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if bcrypt.CompareHashAndPassword([]byte(s.state.Code), []byte(current)) == nil && len(newCode) >= 4 && len(newCode) <= 64 {
		s.state.Code = hashBcrypt(newCode)
		s.save()
		return true
	}
	return false
}
