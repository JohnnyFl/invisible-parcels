// Package poller pulls the client-side status from NP (the emulator) like a real
// integration would: the public NP API has no push.
package poller

import (
	"context"
	"log"
	"time"

	"novapost/backend/emuclient"
	"novapost/backend/store"
)

// Run polls every interval until ctx is done. Emulator errors are logged and the
// last snapshot is kept.
func Run(ctx context.Context, st *store.Store, emu *emuclient.Client, every time.Duration, onChange func()) {
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		ttns, err := st.UnissuedTTNs(ctx)
		if err != nil || len(ttns) == 0 {
			if err != nil && ctx.Err() == nil {
				log.Printf("poller: %v", err)
			}
			continue
		}
		sts, err := emu.Track(ctx, ttns)
		if err != nil {
			if ctx.Err() == nil {
				log.Printf("poller: %v", err)
			}
			continue
		}
		if err := st.SaveNPStatus(ctx, sts, time.Now()); err != nil {
			log.Printf("poller: save: %v", err)
			continue
		}
		onChange()
	}
}
