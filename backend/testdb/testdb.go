// Package testdb gives tests their own database, so `make test` never wipes demo data.
package testdb

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

const devURL = "postgres://novapost:novapost@localhost:5432/novapost?sslmode=disable"

// URL returns TEST_DATABASE_URL, or creates and returns novapost_test next to the dev database.
func URL(t testing.TB) string {
	t.Helper()
	if u := os.Getenv("TEST_DATABASE_URL"); u != "" {
		return u
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, devURL)
	if err != nil {
		t.Fatalf("connect to dev postgres (run `make db`): %v", err)
	}
	defer conn.Close(ctx)
	var exists bool
	conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = 'novapost_test')`).Scan(&exists)
	if !exists {
		if _, err := conn.Exec(ctx, `CREATE DATABASE novapost_test`); err != nil && !strings.Contains(err.Error(), "already exists") {
			t.Fatalf("create test db: %v", err)
		}
	}
	return strings.Replace(devURL, "/novapost?", "/novapost_test?", 1)
}
