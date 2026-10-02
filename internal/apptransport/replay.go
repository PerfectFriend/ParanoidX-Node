package apptransport

import (
	"sync"
	"time"
)

type ReplayGuard struct {
	mu     sync.Mutex
	seen   map[string]time.Time
	window time.Duration
	clock  func() time.Time
}

func NewReplayGuard(window time.Duration, clock func() time.Time) *ReplayGuard {
	if window <= 0 {
		window = 10 * time.Minute
	}
	if clock == nil {
		clock = time.Now
	}
	return &ReplayGuard{
		seen:   make(map[string]time.Time),
		window: window,
		clock:  clock,
	}
}

func (g *ReplayGuard) Check(env Envelope) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := g.clock()
	for nonce, seen := range g.seen {
		if now.Sub(seen) > g.window {
			delete(g.seen, nonce)
		}
	}
	if seen, ok := g.seen[env.Nonce]; ok && now.Sub(seen) <= g.window {
		return ErrReplay
	}
	g.seen[env.Nonce] = now
	return nil
}

func (g *ReplayGuard) Forget(nonce string) {
	if nonce == "" || g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.seen, nonce)
}

func (g *ReplayGuard) Size() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.seen)
}
