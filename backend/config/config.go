// Package config reads settings from env with localhost defaults for development.
package config

import (
	"log"
	"os"
	"strconv"
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
		DatabaseURL:      env("DATABASE_URL", "postgres://novapost:novapost@localhost:5432/novapost?sslmode=disable"),
		EmulatorURL:      env("EMULATOR_URL", "http://localhost:8090"),
		IngestSecret:     env("INGEST_SECRET", "dev-ingest-secret"),
		AppSecret:        env("APP_SECRET", "dev-app-secret"),
		AdminPassword:    env("ADMIN_PASSWORD", "admin"),
		SeniorPassword:   env("SENIOR_PASSWORD", "senior"),
		OperatorPassword: env("OPERATOR_PASSWORD", "operator"),
		Port:             env("PORT", "8080"),
	}
	var err error
	if c.NPPollInterval, err = time.ParseDuration(env("NP_POLL_INTERVAL", "2s")); err != nil {
		log.Fatalf("NP_POLL_INTERVAL: %v", err)
	}
	if c.LookupLimitPerHour, err = strconv.Atoi(env("LOOKUP_LIMIT_PER_HOUR", "30")); err != nil {
		log.Fatalf("LOOKUP_LIMIT_PER_HOUR: %v", err)
	}
	if c.AdminPassword == "admin" || c.AppSecret == "dev-app-secret" {
		log.Println("WARNING: default passwords/secrets in use — set ADMIN_PASSWORD, SENIOR_PASSWORD, OPERATOR_PASSWORD, APP_SECRET, INGEST_SECRET outside local dev")
	}
	return c
}
