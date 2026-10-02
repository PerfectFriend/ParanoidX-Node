package apptransport

import "sync"

type SignalState struct {
	mu    sync.Mutex
	rooms map[string]map[string]any
}

func NewSignalState() *SignalState {
	return &SignalState{rooms: make(map[string]map[string]any)}
}

func (s *SignalState) PostSignal(room string, payload map[string]any) {
	if room == "" {
		room = "default"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rooms[room] == nil {
		s.rooms[room] = make(map[string]any)
	}
	for k, v := range payload {
		s.rooms[room][k] = v
	}
}

func (s *SignalState) GetState(room string) map[string]any {
	if room == "" {
		room = "default"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.rooms[room]
	out := make(map[string]any, len(state))
	for k, v := range state {
		out[k] = v
	}
	return out
}

func (s *SignalState) RoomCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.rooms)
}
