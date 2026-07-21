package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"forgereview/backend/internal/workflow"
	_ "modernc.org/sqlite"
)

type SQLite struct{ db *sql.DB }

func Open(path string) (*SQLite, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil { return nil, err }
	store := &SQLite{db: db}
	if _, err = db.Exec(schema); err != nil { db.Close(); return nil, err }
	return store, nil
}

func (s *SQLite) Close() error { return s.db.Close() }

func (s *SQLite) Save(ctx context.Context, definition workflow.Definition) (int64, error) {
	payload, err := json.Marshal(definition)
	if err != nil { return 0, err }
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil { return 0, err }
	defer tx.Rollback()
	var version int
	err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM workflow_versions WHERE workflow_key=?`, definition.Key).Scan(&version)
	if err != nil { return 0, err }
	result, err := tx.ExecContext(ctx, `INSERT INTO workflow_versions(workflow_key, version, name, description, status, definition_json) VALUES(?,?,?,?, 'draft', ?)`, definition.Key, version+1, definition.Name, definition.Description, string(payload))
	if err != nil { return 0, err }
	id, _ := result.LastInsertId()
	if err = tx.Commit(); err != nil { return 0, err }
	return id, nil
}

func (s *SQLite) Load(ctx context.Context, id int64) (workflow.Definition, error) {
	var payload string
	err := s.db.QueryRowContext(ctx, `SELECT definition_json FROM workflow_versions WHERE id=?`, id).Scan(&payload)
	if err != nil { return workflow.Definition{}, err }
	var definition workflow.Definition
	if err = json.Unmarshal([]byte(payload), &definition); err != nil { return workflow.Definition{}, fmt.Errorf("decode workflow: %w", err) }
	return definition, nil
}

const schema = `
PRAGMA foreign_keys = ON;
CREATE TABLE IF NOT EXISTS workflow_versions (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 workflow_key TEXT NOT NULL,
 version INTEGER NOT NULL,
 name TEXT NOT NULL,
 description TEXT NOT NULL DEFAULT '',
 status TEXT NOT NULL CHECK(status IN ('draft','published','archived')),
 definition_json TEXT NOT NULL,
 created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
 UNIQUE(workflow_key, version)
);
CREATE UNIQUE INDEX IF NOT EXISTS workflow_published_version ON workflow_versions(workflow_key) WHERE status='published';
CREATE TABLE IF NOT EXISTS workflow_executions (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 workflow_version_id INTEGER NOT NULL REFERENCES workflow_versions(id),
 status TEXT NOT NULL,
 started_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
 finished_at TEXT,
 metadata_json TEXT NOT NULL DEFAULT '{}'
);
CREATE TABLE IF NOT EXISTS workflow_node_runs (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 workflow_execution_id INTEGER NOT NULL REFERENCES workflow_executions(id),
 node_key TEXT NOT NULL,
 scope_key TEXT NOT NULL DEFAULT 'root',
 status TEXT NOT NULL,
 attempt INTEGER NOT NULL DEFAULT 1,
 started_at TEXT,
 finished_at TEXT,
 metadata_json TEXT NOT NULL DEFAULT '{}'
);
`
