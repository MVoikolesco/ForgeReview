package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"forgereview/backend/internal/integration"
	"forgereview/backend/internal/workflow"

	_ "modernc.org/sqlite"
)

type SQLite struct{ db *sql.DB }

var (
	ErrWorkflowVersionNotFound = errors.New("workflow version not found")
	ErrWorkflowVersionNotDraft = errors.New("workflow version is not a draft")
	ErrInvalidWorkflowVersion  = errors.New("workflow version is invalid")
)

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
	if err = store.ensureExecutionInputColumn(context.Background()); err != nil {
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
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
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

// ListDefinitions returns workflow keys with their version lifecycle metadata.
func (s *SQLite) ListDefinitions(ctx context.Context) ([]workflow.DefinitionSummary, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT workflow_key,name,description,id,version,status,created_at FROM workflow_versions ORDER BY workflow_key,version`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []workflow.DefinitionSummary{}
	indexes := map[string]int{}
	for rows.Next() {
		var key, name, description string
		var version workflow.VersionSummary
		if err = rows.Scan(&key, &name, &description, &version.ID, &version.Version, &version.Status, &version.CreatedAt); err != nil {
			return nil, err
		}
		index, exists := indexes[key]
		if !exists {
			index = len(items)
			indexes[key] = index
			items = append(items, workflow.DefinitionSummary{Key: key, Versions: []workflow.VersionSummary{}})
		}
		// Rows are version ordered, so the final values describe the latest draft
		// or published revision while the complete lifecycle remains in Versions.
		items[index].Name = name
		items[index].Description = description
		items[index].Versions = append(items[index].Versions, version)
	}
	return items, rows.Err()
}

// Publish atomically validates and promotes a draft. Any currently published
// version for the same workflow key is archived in the same transaction.
func (s *SQLite) Publish(ctx context.Context, id int64, catalog workflow.Catalog) (workflow.VersionSummary, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return workflow.VersionSummary{}, err
	}
	defer tx.Rollback()

	var payload, workflowKey, status string
	var summary workflow.VersionSummary
	err = tx.QueryRowContext(ctx, `SELECT workflow_key,version,status,definition_json,created_at FROM workflow_versions WHERE id=?`, id).Scan(&workflowKey, &summary.Version, &status, &payload, &summary.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return workflow.VersionSummary{}, fmt.Errorf("%w: %d", ErrWorkflowVersionNotFound, id)
	}
	if err != nil {
		return workflow.VersionSummary{}, err
	}
	if status != workflow.VersionStatusDraft {
		return workflow.VersionSummary{}, fmt.Errorf("%w: %d", ErrWorkflowVersionNotDraft, id)
	}

	var definition workflow.Definition
	if err = json.Unmarshal([]byte(payload), &definition); err != nil {
		return workflow.VersionSummary{}, fmt.Errorf("%w: decode definition: %v", ErrInvalidWorkflowVersion, err)
	}
	if err = workflow.Validate(definition, catalog); err != nil {
		return workflow.VersionSummary{}, fmt.Errorf("%w: %v", ErrInvalidWorkflowVersion, err)
	}
	if _, err = tx.ExecContext(ctx, `UPDATE workflow_versions SET status=? WHERE workflow_key=? AND status=?`, workflow.VersionStatusArchived, workflowKey, workflow.VersionStatusPublished); err != nil {
		return workflow.VersionSummary{}, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE workflow_versions SET status=? WHERE id=? AND status=?`, workflow.VersionStatusPublished, id, workflow.VersionStatusDraft)
	if err != nil {
		return workflow.VersionSummary{}, err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return workflow.VersionSummary{}, err
	}
	if updated != 1 {
		return workflow.VersionSummary{}, fmt.Errorf("%w: %d", ErrWorkflowVersionNotDraft, id)
	}
	if err = tx.Commit(); err != nil {
		return workflow.VersionSummary{}, err
	}
	summary.ID = id
	summary.Status = workflow.VersionStatusPublished
	return summary, nil
}

// CreateExecution persists an execution before it can be dispatched. Input is
// retained for workers but never included in the execution status response.
func (s *SQLite) CreateExecution(ctx context.Context, versionID int64, input map[string]any) (int64, error) {
	payload, err := json.Marshal(input)
	if err != nil {
		return 0, err
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO workflow_executions(workflow_version_id,status,metadata_json,execution_input_json) VALUES(?, 'queued', '{}', ?)`, versionID, string(payload))
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// ClaimExecution atomically transitions a queued execution to running. A
// duplicate queue delivery receives false and must not execute it again.
func (s *SQLite) ClaimExecution(ctx context.Context, id int64) (workflow.Execution, bool, error) {
	result, err := s.db.ExecContext(ctx, `UPDATE workflow_executions SET status='running' WHERE id=? AND status='queued'`, id)
	if err != nil {
		return workflow.Execution{}, false, err
	}
	claimed, err := result.RowsAffected()
	if err != nil || claimed == 0 {
		return workflow.Execution{}, false, err
	}
	var execution workflow.Execution
	var input string
	err = s.db.QueryRowContext(ctx, `SELECT workflow_version_id,execution_input_json FROM workflow_executions WHERE id=?`, id).Scan(&execution.VersionID, &input)
	if err != nil {
		return workflow.Execution{}, false, err
	}
	execution.ID = id
	if err = json.Unmarshal([]byte(input), &execution.Input); err != nil {
		return workflow.Execution{}, false, fmt.Errorf("decode execution input: %w", err)
	}
	return execution, true, nil
}

func (s *SQLite) SaveExecution(ctx context.Context, versionID int64, report workflow.RunReport) (int64, error) {
	id, err := s.CreateExecution(ctx, versionID, nil)
	if err != nil {
		return 0, err
	}
	if _, _, err = s.ClaimExecution(ctx, id); err != nil {
		return 0, err
	}
	return id, s.CompleteExecution(ctx, id, report)
}

// CompleteExecution persists node reports and makes the final status visible.
func (s *SQLite) CompleteExecution(ctx context.Context, id int64, report workflow.RunReport) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE workflow_executions SET status=?,finished_at=CURRENT_TIMESTAMP WHERE id=? AND status='running'`, report.Status, id)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated != 1 {
		return fmt.Errorf("execution %d is not running", id)
	}
	for _, run := range report.Runs {
		metadata, err := json.Marshal(map[string]any{"inputs": run.Inputs, "outputs": run.Outputs, "error": run.Error, "duration_ms": run.DurationMS})
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO workflow_node_runs(workflow_execution_id,node_key,status,finished_at,metadata_json) VALUES(?,?,?,CURRENT_TIMESTAMP,?)`, id, run.NodeKey, run.Status, string(metadata)); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (s *SQLite) FailQueuedExecution(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE workflow_executions SET status='failed',finished_at=CURRENT_TIMESTAMP WHERE id=? AND status='queued'`, id)
	return err
}

func (s *SQLite) BeginPublication(ctx context.Context, attempt workflow.PublicationAttempt) (workflow.PublicationAttempt, bool, error) {
	if attempt.IdempotencyKey == "" || attempt.ExecutionID < 1 || attempt.VersionID < 1 || attempt.NodeKey == "" {
		return workflow.PublicationAttempt{}, false, fmt.Errorf("publication attempt is incomplete")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return workflow.PublicationAttempt{}, false, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `INSERT INTO publication_attempts(idempotency_key,workflow_execution_id,workflow_version_id,node_key,status) VALUES(?,?,?,?, 'pending') ON CONFLICT(idempotency_key) DO NOTHING`, attempt.IdempotencyKey, attempt.ExecutionID, attempt.VersionID, attempt.NodeKey)
	if err != nil {
		return workflow.PublicationAttempt{}, false, err
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return workflow.PublicationAttempt{}, false, err
	}
	if inserted == 1 {
		attempt.Status = "pending"
		if err = tx.Commit(); err != nil {
			return workflow.PublicationAttempt{}, false, err
		}
		return attempt, true, nil
	}
	var receipt string
	if err = tx.QueryRowContext(ctx, `SELECT status,receipt_json FROM publication_attempts WHERE idempotency_key=?`, attempt.IdempotencyKey).Scan(&attempt.Status, &receipt); err != nil {
		return workflow.PublicationAttempt{}, false, err
	}
	if receipt != "" && receipt != "{}" {
		if err = json.Unmarshal([]byte(receipt), &attempt.Receipt); err != nil {
			return workflow.PublicationAttempt{}, false, err
		}
	}
	if attempt.Status == "retryable" {
		result, updateErr := tx.ExecContext(ctx, `UPDATE publication_attempts SET status='pending',attempt=attempt+1,error_message='' WHERE idempotency_key=? AND status='retryable'`, attempt.IdempotencyKey)
		if updateErr != nil {
			return workflow.PublicationAttempt{}, false, updateErr
		}
		updated, updateErr := result.RowsAffected()
		if updateErr != nil {
			return workflow.PublicationAttempt{}, false, updateErr
		}
		if updated == 1 {
			attempt.Status = "pending"
			if err = tx.Commit(); err != nil {
				return workflow.PublicationAttempt{}, false, err
			}
			return attempt, true, nil
		}
		if err = tx.QueryRowContext(ctx, `SELECT status,receipt_json FROM publication_attempts WHERE idempotency_key=?`, attempt.IdempotencyKey).Scan(&attempt.Status, &receipt); err != nil {
			return workflow.PublicationAttempt{}, false, err
		}
		if receipt != "" && receipt != "{}" {
			if err = json.Unmarshal([]byte(receipt), &attempt.Receipt); err != nil {
				return workflow.PublicationAttempt{}, false, err
			}
		}
	}
	if err = tx.Commit(); err != nil {
		return workflow.PublicationAttempt{}, false, err
	}
	return attempt, false, nil
}

func (s *SQLite) CompletePublication(ctx context.Context, key string, receipt integration.PublicationReceipt) error {
	payload, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `UPDATE publication_attempts SET status='completed',receipt_json=?,completed_at=CURRENT_TIMESTAMP,error_message='' WHERE idempotency_key=? AND status='pending'`, string(payload), key)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated != 1 {
		return fmt.Errorf("publication %q is not pending", key)
	}
	return nil
}

func (s *SQLite) RetryPublication(ctx context.Context, key string, cause error) error {
	// Provider responses are deliberately not stored: some providers may echo
	// credentials. The state is enough to permit a later controlled retry.
	result, err := s.db.ExecContext(ctx, `UPDATE publication_attempts SET status='retryable',error_message='publication failed' WHERE idempotency_key=? AND status='pending'`, key)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated != 1 {
		return fmt.Errorf("publication %q is not pending", key)
	}
	return nil
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
  metadata_json TEXT NOT NULL DEFAULT '{}',
  execution_input_json TEXT NOT NULL DEFAULT '{}'
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
CREATE TABLE IF NOT EXISTS publication_attempts (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 idempotency_key TEXT NOT NULL UNIQUE,
 workflow_execution_id INTEGER NOT NULL REFERENCES workflow_executions(id),
 workflow_version_id INTEGER NOT NULL REFERENCES workflow_versions(id),
 node_key TEXT NOT NULL,
 status TEXT NOT NULL CHECK(status IN ('pending','completed','retryable')),
 attempt INTEGER NOT NULL DEFAULT 1,
 receipt_json TEXT NOT NULL DEFAULT '{}',
 error_message TEXT NOT NULL DEFAULT '',
 created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
 completed_at TEXT
);
`

func (s *SQLite) ensureExecutionInputColumn(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `PRAGMA table_info(workflow_executions)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, primaryKey int
		var defaultValue any
		if err = rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			return err
		}
		if name == "execution_input_json" {
			return nil
		}
	}
	if err = rows.Err(); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `ALTER TABLE workflow_executions ADD COLUMN execution_input_json TEXT NOT NULL DEFAULT '{}'`)
	return err
}
