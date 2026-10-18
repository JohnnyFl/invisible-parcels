// Package store is the Postgres layer: an append-only event journal plus the
// parcels / route plan materialized from "created" events.
package store

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"

	"novapost/contract"
)

//go:embed migrations/*.sql
var migrations embed.FS

type Store struct {
	Pool *pgxpool.Pool
}

// Open connects, applies pending migrations and seeds locations.
func Open(ctx context.Context, url string) (*Store, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	s := &Store{Pool: pool}
	if err := s.migrate(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	for _, l := range contract.Locations {
		if _, err := pool.Exec(ctx, `INSERT INTO locations (id, kind, city, name) VALUES ($1,$2,$3,$4)
			ON CONFLICT (id) DO UPDATE SET kind=$2, city=$3, name=$4`, l.ID, l.Kind, l.City, l.Name); err != nil {
			pool.Close()
			return nil, err
		}
	}
	return s, nil
}

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.Pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (name text PRIMARY KEY)`); err != nil {
		return err
	}
	names, err := fs.Glob(migrations, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)
	for _, name := range names {
		var done bool
		if err := s.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE name=$1)`, name).Scan(&done); err != nil {
			return err
		}
		if done {
			continue
		}
		sql, _ := migrations.ReadFile(name)
		tx, err := s.Pool.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, string(sql)); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("%s: %w", name, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (name) VALUES ($1)`, name); err != nil {
			tx.Rollback(ctx)
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}

// Reset wipes all simulation data (keeps locations and app_state).
func (s *Store) Reset(ctx context.Context) error {
	_, err := s.Pool.Exec(ctx, `TRUNCATE events, parcels, route_legs, np_status, audit_log`)
	return err
}
