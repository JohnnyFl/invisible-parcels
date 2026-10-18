// Package sim owns the virtual clock and the "as is / with solution" mode.
// The backend is the single source; every clock change is mirrored to the emulator.
package sim

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"novapost/backend/emuclient"
	"novapost/backend/store"
	"novapost/contract"
)

type Mode string

const (
	ModeAsIs         Mode = "as_is"
	ModeWithSolution Mode = "with_solution"
)

type Sim struct {
	mu    sync.Mutex
	clock contract.Clock
	mode  Mode
	store *store.Store
	emu   *emuclient.Client
}

func New(ctx context.Context, st *store.Store, emu *emuclient.Client) (*Sim, error) {
	s := &Sim{
		clock: contract.Clock{RealBase: time.Now(), VirtualBase: contract.ScenarioStart},
		mode:  ModeWithSolution,
		store: st,
		emu:   emu,
	}
	if err := s.load(ctx, "clock", &s.clock); err != nil {
		return nil, err
	}
	if err := s.load(ctx, "mode", &s.mode); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Sim) load(ctx context.Context, key string, into any) error {
	var raw []byte
	err := s.store.Pool.QueryRow(ctx, `SELECT value FROM app_state WHERE key = $1`, key).Scan(&raw)
	if err == pgx.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, into)
}

func (s *Sim) save(ctx context.Context, key string, v any) error {
	raw, _ := json.Marshal(v)
	_, err := s.store.Pool.Exec(ctx, `INSERT INTO app_state (key, value) VALUES ($1, $2)
		ON CONFLICT (key) DO UPDATE SET value = $2`, key, raw)
	return err
}

func (s *Sim) Now() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.clock.Now(time.Now())
}

func (s *Sim) Clock() contract.Clock {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.clock
}

func (s *Sim) Mode() Mode {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mode
}

// setClock syncs the emulator first, so a failed sync leaves both sides unchanged.
func (s *Sim) setClock(ctx context.Context, next func(real time.Time, c contract.Clock) contract.Clock) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	real := time.Now()
	c := next(real, s.clock)
	if err := s.emu.SetClock(ctx, c.Now(real), c.Speed); err != nil {
		return err
	}
	s.clock = c
	return s.save(ctx, "clock", c)
}

func (s *Sim) SetSpeed(ctx context.Context, speed float64) error {
	return s.setClock(ctx, func(real time.Time, c contract.Clock) contract.Clock { return c.Rebase(real, speed) })
}

func (s *Sim) Jump(ctx context.Context, virtual time.Time) error {
	return s.setClock(ctx, func(real time.Time, c contract.Clock) contract.Clock {
		return contract.Clock{RealBase: real, VirtualBase: virtual, Speed: c.Speed}
	})
}

func (s *Sim) SetMode(ctx context.Context, m Mode) error {
	if m != ModeAsIs && m != ModeWithSolution {
		return fmt.Errorf("unknown mode %q", m)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mode = m
	return s.save(ctx, "mode", m)
}
