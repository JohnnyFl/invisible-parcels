// Command backend reconciles NP client tracking with branch scans.
package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"novapost/backend/api"
	"novapost/backend/config"
	"novapost/backend/emuclient"
	"novapost/backend/sim"
	"novapost/backend/store"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()
	st, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	emu := &emuclient.Client{URL: cfg.EmulatorURL, HTTP: &http.Client{Timeout: 5 * time.Second}}
	s, err := sim.New(ctx, st, emu)
	if err != nil {
		log.Fatalf("sim: %v", err)
	}
	hub := api.NewHub()
	log.Fatal(api.NewServer(api.Deps{Cfg: cfg, Store: st, Hub: hub, Sim: s, Emu: emu}).Start(":" + cfg.Port))
}
