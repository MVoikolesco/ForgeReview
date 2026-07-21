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

	_ "modernc.org/sqlite" // Register the pure-Go SQLite database driver.
)

// migrationFS embeds migrations so API and worker use the same schema.
//
//go:embed migrations/*.sql
var migrationFS embed.FS

// DB owns the configured SQLite connection used by application repositories.
type DB struct {
	SQL *sql.DB
}

// Open validates the configured database driver, creates the database directory,
// opens SQLite, and returns a ready DB connection.
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

// Close releases the underlying SQLite connection.
func (d *DB) Close() error {
	return d.SQL.Close()
}

// Migrate applies each pending embedded migration in filename order. It returns
// the first migration or transaction error encountered.
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

// RequireSchema verifies that the complete database-driven review pipeline is
// available. Workers never migrate and receive an actionable startup error.
func (d *DB) RequireSchema(ctx context.Context) error {
	for _, table := range []string{"reviews", "pipeline_versions", "pipeline_stages", "pipeline_executions", "stage_executions"} {
		var name string
		if err := d.SQL.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name); err != nil {
			return fmt.Errorf("sqlite pipeline schema is not initialized; start API first: missing %s: %w", table, err)
		}
	}
	var pipelines int
	if err := d.SQL.QueryRowContext(ctx, `SELECT count(*) FROM pipeline_definitions pd
		JOIN pipeline_versions pv ON pv.pipeline_definition_id=pd.id
		WHERE pd.is_default=1 AND pd.is_enabled=1 AND pv.status='published'`).Scan(&pipelines); err != nil || pipelines == 0 {
		return fmt.Errorf("sqlite pipeline catalog is not initialized; start API first")
	}
	return nil
}
