// Package push sends reported events to the backend webhook, like NP would.
package push

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"
	"sync"
	"time"

	"novapost/contract"
	"novapost/emulator/world"
)

type Pusher struct {
	URL    string
	Secret string
	Client *http.Client

	mu      sync.Mutex
	pending []contract.Event
}

// Send posts one ScanEvent.push envelope signed with HMAC.
func (p *Pusher) Send(ctx context.Context, events []contract.Event) error {
	body, err := json.Marshal(contract.Envelope[contract.EventBatch]{
		ModelName: "ScanEvent", CalledMethod: "push",
		MethodProperties: contract.EventBatch{Events: events},
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.URL+"/api/ingest/events", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Signature", contract.Sign(body, p.Secret))
	resp, err := p.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("ingest: %s", resp.Status)
	}
	return nil
}

// Clear drops queued events (scenario reset).
func (p *Pusher) Clear() {
	p.mu.Lock()
	p.pending = nil
	p.mu.Unlock()
}

// Run pushes due events every 200ms. Batches are shuffled and ~20% held back a
// tick, so the backend sees events out of order like real async logistics.
func (p *Pusher) Run(ctx context.Context, w *world.World, now func() time.Time) {
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		p.mu.Lock()
		batch := append(p.pending, w.Due(now())...)
		rand.Shuffle(len(batch), func(i, j int) { batch[i], batch[j] = batch[j], batch[i] })
		var send []contract.Event
		p.pending = nil
		for _, e := range batch {
			if rand.IntN(5) == 0 {
				p.pending = append(p.pending, e)
			} else {
				send = append(send, e)
			}
		}
		p.mu.Unlock()
		if len(send) == 0 {
			continue
		}
		if err := p.Send(ctx, send); err != nil {
			log.Printf("push: %v (will retry %d events)", err, len(send))
			p.mu.Lock()
			p.pending = append(p.pending, send...)
			p.mu.Unlock()
		}
	}
}
