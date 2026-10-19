// Package config reads settings from env with localhost defaults for development.
package config

import (
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DatabaseURL        string
	EmulatorURL        string
	IngestSecret       string
	AppSecret          string
	AdminPassword      string
	SeniorPassword     string
	OperatorPassword   string
	NPPollInterval     time.Duration
	LookupLimitPerHour int
	Port               string
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func Load() Config {
	c := Config{
		DatabaseURL:  env("DATABASE_URL", "postgres://novapost:novapost@localhost:5432/novapost?sslmode=disable"),
		EmulatorURL:  env("EMULATOR_URL", "http://localhost:8090"),
		IngestSecret: env("INGEST_SECRET", "dev-ingest-secret"),
		AppSecret:    env("APP_SECRET", "dev-app-secret"),
		// role passwords are public demo passwords (see README); the two secrets below are not
		AdminPassword:    env("ADMIN_PASSWORD", "demo"),
		SeniorPassword:   env("SENIOR_PASSWORD", "demo"),
		OperatorPassword: env("OPERATOR_PASSWORD", "demo"),
		Port:             env("PORT", "8080"),
	}
	var err error
	if c.NPPollInterval, err = time.ParseDuration(env("NP_POLL_INTERVAL", "2s")); err != nil {
		log.Fatalf("NP_POLL_INTERVAL: %v", err)
	}
	if c.LookupLimitPerHour, err = strconv.Atoi(env("LOOKUP_LIMIT_PER_HOUR", "30")); err != nil {
		log.Fatalf("LOOKUP_LIMIT_PER_HOUR: %v", err)
	}
	if d := DefaultSecrets(c); len(d) > 0 {
		log.Printf("WARNING: default values in use for %s — set them outside local dev", strings.Join(d, ", "))
	}
	return c
}

// DefaultSecrets names the secrets still at their published dev defaults.
// Role passwords are left out: "demo" is published on purpose.
func DefaultSecrets(c Config) []string {
	var out []string
	for _, s := range []struct{ name, value, def string }{
		{"APP_SECRET", c.AppSecret, "dev-app-secret"},
		{"INGEST_SECRET", c.IngestSecret, "dev-ingest-secret"},
	} {
		if s.value == s.def {
			out = append(out, s.name)
		}
	}
	return out
}
