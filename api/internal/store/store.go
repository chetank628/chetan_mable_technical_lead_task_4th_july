// Package store is the SQLite persistence layer for the API. It owns schema
// migration, prepared statements, and the typed stores used by the ingest
// worker (events + stage metrics) and the HTTP handlers (users + analytics).
//
// SQLite is chosen for zero-infra embedding; WAL mode lets the read-only stats
// queries run concurrently with the single-goroutine ingest writer.
package store

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite" // registers the CGO-free "sqlite" driver
)

// Store wraps the database handle and exposes the typed sub-stores. It is safe
// for concurrent use: database/sql manages an internal connection pool and the
// schema serialises writers via WAL.
type Store struct {
	db *sql.DB
}

// Open opens (creating if needed) the SQLite database at path, applies the
// performance pragmas, and runs migrations. Use ":memory:" for tests.
func Open(ctx context.Context, path string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// A single writer connection avoids "database is locked" churn for the
	// ingest path; reads still use the pool under WAL.
	db.SetMaxOpenConns(1)

	s := &Store{db: db}
	if err := s.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

// DB exposes the underlying handle for health pings.
func (s *Store) DB() *sql.DB { return s.db }

// Ping verifies database connectivity for readiness checks.
func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

// Close closes the underlying database.
func (s *Store) Close() error { return s.db.Close() }

// migrate creates the schema if it does not already exist. Migrations are
// idempotent so boot is safe to repeat.
func (s *Store) migrate(ctx context.Context) error {
	const schema = `
CREATE TABLE IF NOT EXISTS users (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	email         TEXT NOT NULL UNIQUE,
	password_hash TEXT NOT NULL,
	created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS events (
	id                INTEGER PRIMARY KEY AUTOINCREMENT,
	event_type        TEXT NOT NULL,
	user_id           TEXT,
	session_id        TEXT,
	ts                TIMESTAMP NOT NULL,
	url               TEXT,
	referrer          TEXT,
	user_agent        TEXT,
	ip                TEXT,
	amount            REAL,
	currency          TEXT,
	properties        TEXT,
	churn_probability REAL,
	received_at       TIMESTAMP NOT NULL,
	capture_ms        REAL NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_events_ts ON events(ts);
CREATE INDEX IF NOT EXISTS idx_events_type_ts ON events(event_type, ts);

CREATE TABLE IF NOT EXISTS stage_metrics (
	id               INTEGER PRIMARY KEY AUTOINCREMENT,
	run_id           TEXT NOT NULL,
	window_ts        TIMESTAMP NOT NULL,
	stage_name       TEXT NOT NULL,
	items_in         INTEGER NOT NULL,
	items_out        INTEGER NOT NULL,
	dropped          INTEGER NOT NULL,
	errors           INTEGER NOT NULL,
	batches          INTEGER NOT NULL,
	total_latency_ns INTEGER NOT NULL,
	p50_ns           INTEGER NOT NULL,
	p99_ns           INTEGER NOT NULL,
	wall_ns          INTEGER NOT NULL,
	throughput       REAL NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_stage_metrics_window ON stage_metrics(window_ts);
`
	_, err := s.db.ExecContext(ctx, schema)
	return err
}
