package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/labstack/echo/v5"
)

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

// streamHandler is the SSE endpoint. It only tells clients which query keys to
// refetch; data itself goes through the normal JSON endpoints.
func streamHandler(h *Hub) echo.HandlerFunc {
	return func(c *echo.Context) error {
		w := c.Response()
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
		rc := http.NewResponseController(w)
		ch, unsub := h.Subscribe()
		defer unsub()
		ping := time.NewTicker(15 * time.Second)
		defer ping.Stop()
		fmt.Fprint(w, ": connected\n\n")
		rc.Flush()
		for {
			select {
			case <-c.Request().Context().Done():
				return nil
			case <-ping.C:
				fmt.Fprint(w, ": ping\n\n")
			case keys := <-ch:
				msg, _ := json.Marshal(map[string]any{"type": "invalidate", "keys": keys})
				fmt.Fprintf(w, "data: %s\n\n", msg)
			}
			if err := rc.Flush(); err != nil {
				return nil
			}
		}
	}
}
