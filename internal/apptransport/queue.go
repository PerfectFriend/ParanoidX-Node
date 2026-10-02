package apptransport

import (
	"sync"
	"time"
)

type Queue struct {
	mu    sync.RWMutex
	max   int
	items []QueueMessage
	subs  map[chan QueueMessage]struct{}
}

func NewQueue(max int) *Queue {
	if max <= 0 {
		max = 4096
	}
	return &Queue{
		max:   max,
		items: make([]QueueMessage, 0, max),
		subs:  make(map[chan QueueMessage]struct{}),
	}
}

func (q *Queue) Enqueue(env Envelope) (QueueMessage, error) {
	msg := QueueMessage{Envelope: env, ReceivedAt: time.Now().UTC()}
	q.mu.Lock()
	if len(q.items) >= q.max {
		copy(q.items, q.items[1:])
		q.items = q.items[:q.max-1]
	}
	q.items = append(q.items, msg)
	subs := make([]chan QueueMessage, 0, len(q.subs))
	for ch := range q.subs {
		subs = append(subs, ch)
	}
	q.mu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- msg:
		default:
		}
	}
	return msg, nil
}

func (q *Queue) HasCapacity() bool {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.items) < q.max
}

func (q *Queue) History(limit int) []QueueMessage {
	q.mu.RLock()
	defer q.mu.RUnlock()
	if limit <= 0 || limit > len(q.items) {
		limit = len(q.items)
	}
	start := len(q.items) - limit
	out := make([]QueueMessage, limit)
	copy(out, q.items[start:])
	return out
}

func (q *Queue) Subscribe() (<-chan QueueMessage, func()) {
	ch := make(chan QueueMessage, 64)
	q.mu.Lock()
	q.subs[ch] = struct{}{}
	q.mu.Unlock()
	return ch, func() {
		q.mu.Lock()
		delete(q.subs, ch)
		q.mu.Unlock()
	}
}

func (q *Queue) Len() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.items)
}

func (q *Queue) SubscriberCount() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.subs)
}
