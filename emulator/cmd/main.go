// Command emulator imitates Nova Poshta: NP v2.0 tracking (pull) and scan events (push).
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"novapost/contract"
	"novapost/emulator/api"
	"novapost/emulator/push"
	"novapost/emulator/world"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	w := world.New()
	clk := &api.ClockHolder{}
	clk.Set(contract.Clock{RealBase: time.Now(), VirtualBase: contract.ScenarioStart})
	p := &push.Pusher{
		URL:    env("BACKEND_URL", "http://localhost:8080"),
		Secret: env("INGEST_SECRET", "dev-ingest-secret"),
		Client: &http.Client{Timeout: 5 * time.Second},
	}
	go p.Run(context.Background(), w, clk.Now)
	log.Fatal(api.NewServer(w, clk, p.Clear).Start(":" + env("PORT", "8090")))
}
