package api

import "sync"

// Hub fans out "invalidate" notifications to SSE subscribers.
type Hub struct {
	mu   sync.Mutex
	subs map[chan []string]struct{}
}

func NewHub() *Hub { return &Hub{subs: map[chan []string]struct{}{}} }

// Publish never blocks: a subscriber with a full buffer just misses this one.
func (h *Hub) Publish(keys ...string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- keys:
		default:
		}
	}
}

func (h *Hub) Subscribe() (<-chan []string, func()) {
	ch := make(chan []string, 8)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		delete(h.subs, ch)
		h.mu.Unlock()
	}
}
