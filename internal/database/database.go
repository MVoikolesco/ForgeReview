package database

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

// Migrations are embedded in the binary so API and worker use the same schema.
//
//go:embed migrations/*.sql
var migrationFS embed.FS

type DB struct{ SQL *sql.DB }

func Open(cfg config.Config) (*DB, error) {
	if cfg.DatabaseDriver != "sqlite" {
		return nil, fmt.Errorf("unsupported database driver %q", cfg.DatabaseDriver)
	}
	if cfg.DatabaseDSN != ":memory:" && !strings.HasPrefix(cfg.DatabaseDSN, "file:") {
		if err := os.MkdirAll(filepath.Dir(cfg.DatabaseDSN), 0o750); err != nil {
			return nil, fmt.Errorf("create database directory: %w", err)
		}
	}
	db, err := sql.Open("sqlite", cfg.DatabaseDSN)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys=ON; PRAGMA busy_timeout=10000;"); err != nil {
		db.Close()
		return nil, err
	}
	return &DB{SQL: db}, nil
}

func (d *DB) Close() error { return d.SQL.Close() }

func (d *DB) Migrate(ctx context.Context) error {
	if _, err := d.SQL.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY, applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP)`); err != nil {
		return err
	}
	entries, err := migrationFS.ReadDir("migrations")
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
		var applied int
		if err := d.SQL.QueryRowContext(ctx, "SELECT count(*) FROM schema_migrations WHERE name=?", name).Scan(&applied); err != nil {
			return err
		}
		if applied > 0 {
			continue
		}
		body, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		// Migration 009 rebuilds ai_connections, which is referenced by
		// ai_models. SQLite refuses that rebuild while FK enforcement is on.
		rebuildsReferencedTable := name == "009_ai_connection_secret.sql"
		if rebuildsReferencedTable {
			if _, err = d.SQL.ExecContext(ctx, "PRAGMA foreign_keys=OFF"); err != nil {
				return err
			}
		}
		tx, err := d.SQL.BeginTx(ctx, nil)
		if err != nil {
			if rebuildsReferencedTable {
				_, _ = d.SQL.ExecContext(ctx, "PRAGMA foreign_keys=ON")
			}
			return err
		}
		if _, err = tx.ExecContext(ctx, string(body)); err == nil {
			_, err = tx.ExecContext(ctx, "INSERT INTO schema_migrations(name) VALUES(?)", name)
		}
		if err != nil {
			_ = tx.Rollback()
			if rebuildsReferencedTable {
				_, _ = d.SQL.ExecContext(ctx, "PRAGMA foreign_keys=ON")
			}
			return fmt.Errorf("migration %s: %w", name, err)
		}
		if err = tx.Commit(); err != nil {
			if rebuildsReferencedTable {
				_, _ = d.SQL.ExecContext(ctx, "PRAGMA foreign_keys=ON")
			}
			return err
		}
		if rebuildsReferencedTable {
			if _, err = d.SQL.ExecContext(ctx, "PRAGMA foreign_keys=ON"); err != nil {
				return err
			}
		}
	}
	return nil
}

func (d *DB) RequireSchema(ctx context.Context) error {
	var name string
	if err := d.SQL.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name='reviews'").Scan(&name); err != nil {
		return fmt.Errorf("sqlite schema is not initialized; start API first: %w", err)
	}
	return nil
}
