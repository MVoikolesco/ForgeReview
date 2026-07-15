// Package store contains only administrative configuration. It deliberately has
// no tables for pull request content, diffs, generated prompts, or model replies.
package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gitea-agents/internal/config"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

type Store struct{ DB *sql.DB }

func Open(cfg config.Config) (*Store, error) {
	if cfg.DatabaseDriver != "sqlite" {
		return nil, fmt.Errorf("unsupported DATABASE_DRIVER %q (only sqlite is supported)", cfg.DatabaseDriver)
	}
	dsn := cfg.DatabaseDSN
	if dsn == "" {
		dsn = "./data/forgereview.db"
	}
	if !strings.HasPrefix(dsn, "file:") && dsn != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(dsn), 0750); err != nil {
			return nil, fmt.Errorf("create database directory: %w", err)
		}
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{DB: db}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys=ON; PRAGMA busy_timeout=10000;"); err != nil {
		db.Close()
		return nil, fmt.Errorf("configure sqlite: %w", err)
	}
	return s, nil
}
func (s *Store) Close() error { return s.DB.Close() }
func (s *Store) Initialize(ctx context.Context) error {
	if _, err := s.DB.ExecContext(ctx, "PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL;"); err != nil {
		return fmt.Errorf("configure sqlite journal: %w", err)
	}
	return s.Migrate(ctx)
}
func (s *Store) RequireSchema(ctx context.Context) error {
	var name string
	if err := s.DB.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name='ai_providers'").Scan(&name); err != nil {
		return fmt.Errorf("SQLite is not initialized; start the API first: %w", err)
	}
	return nil
}
func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.DB.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY, applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP)`); err != nil {
		return err
	}
	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		var n int
		if err := s.DB.QueryRowContext(ctx, "SELECT count(*) FROM schema_migrations WHERE name=?", name).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			continue
		}
		b, err := migrations.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		tx, err := s.DB.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, string(b)); err == nil {
			_, err = tx.ExecContext(ctx, "INSERT INTO schema_migrations(name) VALUES(?)", name)
		}
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %s: %w", name, err)
		}
		if err = tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
