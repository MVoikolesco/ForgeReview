package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"forgereview/backend/internal/integration"
	"forgereview/backend/internal/workflow"

	_ "modernc.org/sqlite"
)

type SQLite struct{ db *sql.DB }

func Open(path string) (*SQLite, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	store := &SQLite{db: db}
	if _, err = db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLite) Close() error { return s.db.Close() }

func (s *SQLite) CreateIntegration(ctx context.Context, item integration.Integration) error {
	if err := item.Validate(); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO integrations(integration_key,name,type,config_json,secret_reference,status) VALUES(?,?,?,?,?,?)`, item.Key, item.Name, item.Type, string(item.Config), item.SecretReference, item.Status)
	return err
}

func (s *SQLite) Integrations(ctx context.Context) ([]integration.Integration, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT integration_key,name,type,config_json,secret_reference,status FROM integrations ORDER BY integration_key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []integration.Integration{}
	for rows.Next() {
		var item integration.Integration
		var config string
		if err = rows.Scan(&item.Key, &item.Name, &item.Type, &config, &item.SecretReference, &item.Status); err != nil {
			return nil, err
		}
		item.Config = []byte(config)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *SQLite) Integration(ctx context.Context, key string) (integration.Integration, error) {
	var item integration.Integration
	var config string
	err := s.db.QueryRowContext(ctx, `SELECT integration_key,name,type,config_json,secret_reference,status FROM integrations WHERE integration_key=?`, key).Scan(&item.Key, &item.Name, &item.Type, &config, &item.SecretReference, &item.Status)
	if err != nil {
		return integration.Integration{}, err
	}
	item.Config = []byte(config)
	return item, nil
}

func (s *SQLite) Save(ctx context.Context, definition workflow.Definition) (int64, error) {
	payload, err := json.Marshal(definition)
	if err != nil {
		return 0, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var version int
	err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM workflow_versions WHERE workflow_key=?`, definition.Key).Scan(&version)
	if err != nil {
		return 0, err
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO workflow_versions(workflow_key, version, name, description, status, definition_json) VALUES(?,?,?,?, 'draft', ?)`, definition.Key, version+1, definition.Name, definition.Description, string(payload))
	if err != nil {
		return 0, err
	}
	id, _ := result.LastInsertId()
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

func (s *SQLite) Load(ctx context.Context, id int64) (workflow.Definition, error) {
	var payload string
	err := s.db.QueryRowContext(ctx, `SELECT definition_json FROM workflow_versions WHERE id=?`, id).Scan(&payload)
	if err != nil {
		return workflow.Definition{}, err
	}
	var definition workflow.Definition
	if err = json.Unmarshal([]byte(payload), &definition); err != nil {
		return workflow.Definition{}, fmt.Errorf("decode workflow: %w", err)
	}
	return definition, nil
}

func (s *SQLite) SaveExecution(ctx context.Context, versionID int64, report workflow.RunReport) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `INSERT INTO workflow_executions(workflow_version_id,status,finished_at,metadata_json) VALUES(?,?,CURRENT_TIMESTAMP,?)`, versionID, report.Status, "{}")
	if err != nil {
		return 0, err
	}
	id, _ := result.LastInsertId()
	for _, run := range report.Runs {
		metadata, err := json.Marshal(map[string]any{"inputs": run.Inputs, "outputs": run.Outputs, "error": run.Error, "duration_ms": run.DurationMS})
		if err != nil {
			return 0, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO workflow_node_runs(workflow_execution_id,node_key,status,finished_at,metadata_json) VALUES(?,?,?,CURRENT_TIMESTAMP,?)`, id, run.NodeKey, run.Status, string(metadata)); err != nil {
			return 0, err
		}
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

func (s *SQLite) Execution(ctx context.Context, id int64) (workflow.RunReport, error) {
	var status string
	if err := s.db.QueryRowContext(ctx, `SELECT status FROM workflow_executions WHERE id=?`, id).Scan(&status); err != nil {
		return workflow.RunReport{}, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT node_key,status,metadata_json FROM workflow_node_runs WHERE workflow_execution_id=? ORDER BY id`, id)
	if err != nil {
		return workflow.RunReport{}, err
	}
	defer rows.Close()
	report := workflow.RunReport{Status: status}
	for rows.Next() {
		var run workflow.NodeRun
		var metadata string
		if err = rows.Scan(&run.NodeKey, &run.Status, &metadata); err != nil {
			return workflow.RunReport{}, err
		}
		var details struct {
			Inputs     map[string][]any `json:"inputs"`
			Outputs    []workflow.Token `json:"outputs"`
			Error      string           `json:"error"`
			DurationMS int64            `json:"duration_ms"`
		}
		if err = json.Unmarshal([]byte(metadata), &details); err != nil {
			return workflow.RunReport{}, err
		}
		run.Inputs, run.Outputs, run.Error, run.DurationMS = details.Inputs, details.Outputs, details.Error, details.DurationMS
		report.Runs = append(report.Runs, run)
	}
	return report, rows.Err()
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
CREATE TABLE IF NOT EXISTS integrations (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 integration_key TEXT NOT NULL UNIQUE,
 name TEXT NOT NULL,
 type TEXT NOT NULL CHECK(type IN ('gitea','openai','ollama')),
 config_json TEXT NOT NULL,
 secret_reference TEXT NOT NULL,
 status TEXT NOT NULL CHECK(status IN ('active','disabled')),
 created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
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
