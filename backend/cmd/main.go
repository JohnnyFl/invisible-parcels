// Command backend reconciles NP client tracking with branch scans.
package main

import (
	"context"
	"log"

	"novapost/backend/api"
	"novapost/backend/config"
	"novapost/backend/store"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()
	st, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	hub := api.NewHub()
	log.Fatal(api.NewServer(api.Deps{Cfg: cfg, Store: st, Hub: hub}).Start(":" + cfg.Port))
}
