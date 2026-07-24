package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"forgereview/backend/internal/auth"
	"forgereview/backend/internal/integration"
	"forgereview/backend/internal/workflow"

	_ "modernc.org/sqlite"
)

type SQLite struct{ db *sql.DB }

type AuditEntry struct {
	ID        int64          `json:"id"`
	ActorID   int64          `json:"actor_id"`
	Action    string         `json:"action"`
	Target    string         `json:"target"`
	Metadata  map[string]any `json:"metadata"`
	CreatedAt string         `json:"created_at"`
}

// ExecutionMetrics is intentionally aggregate-only: no workflow, user,
// execution, provider, or request labels can become sensitive/high-cardinality.
type ExecutionMetrics struct {
	QueueDepth      int64            `json:"queue_depth"`
	StatusCounts    map[string]int64 `json:"status_counts"`
	RetryCount      int64            `json:"retry_count"`
	DeadLetterCount int64            `json:"dead_letter_count"`
	AverageDuration float64          `json:"average_duration_ms"`
}

var (
	ErrWorkflowVersionNotFound        = errors.New("workflow version not found")
	ErrWorkflowVersionNotDraft        = errors.New("workflow version is not a draft")
	ErrInvalidWorkflowVersion         = errors.New("workflow version is invalid")
	ErrIntegrationNotFound            = errors.New("integration not found")
	ErrIntegrationReferenced          = errors.New("integration has historical workflow references")
	ErrWorkflowVersionNotDeletable    = errors.New("published workflow version cannot be deleted")
	ErrWorkflowVersionDeletionBlocked = errors.New("workflow version deletion is blocked by retained dependencies")
	ErrWebhookDeliveryCollision       = errors.New("webhook delivery collision")
	ErrUserNotFound                   = errors.New("user not found")
	ErrLastActiveAdmin                = errors.New("cannot remove the last active admin")
)

func Open(path string) (*SQLite, error) {
	// busy_timeout applies to every pooled connection. Without it, a short
	// execution write can make an unrelated session read fail immediately with
	// SQLITE_BUSY. WAL keeps status/SSE readers from blocking the worker writer.
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	db, err := sql.Open("sqlite", path+separator+"_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	if _, err = db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		db.Close()
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
	if err = store.migrateIntegrationSecrets(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	if err = store.ensureExecutionTriggerColumn(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	if err = store.migrateFixedPullRequestCoordinates(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	if err = store.ensureUserActiveColumn(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	if err = store.migratePublicationUncertain(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	if err = store.migrateExecutionSensitiveData(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	if err = store.ensureExecutionRetryColumns(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLite) Close() error { return s.db.Close() }

func (s *SQLite) UserCount(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count)
	return count, err
}
func (s *SQLite) CreateUser(ctx context.Context, email, hash string, role auth.Role) (auth.User, error) {
	result, err := s.db.ExecContext(ctx, `INSERT INTO users(email,password_hash,role,active) VALUES(?,?,?,1)`, email, hash, role)
	if err != nil {
		return auth.User{}, err
	}
	id, err := result.LastInsertId()
	return auth.User{ID: id, Email: email, Role: role, Active: true}, err
}
func (s *SQLite) UserByEmail(ctx context.Context, email string) (auth.User, string, error) {
	var user auth.User
	var hash string
	err := s.db.QueryRowContext(ctx, `SELECT id,email,password_hash,role,active FROM users WHERE email=?`, email).Scan(&user.ID, &user.Email, &hash, &user.Role, &user.Active)
	return user, hash, err
}
func (s *SQLite) UserByID(ctx context.Context, id int64) (auth.User, error) {
	var user auth.User
	err := s.db.QueryRowContext(ctx, `SELECT id,email,role,active FROM users WHERE id=?`, id).Scan(&user.ID, &user.Email, &user.Role, &user.Active)
	return user, err
}
func (s *SQLite) Users(ctx context.Context) ([]auth.User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,email,role,active FROM users ORDER BY email`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := []auth.User{}
	for rows.Next() {
		var user auth.User
		if err = rows.Scan(&user.ID, &user.Email, &user.Role, &user.Active); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}
func (s *SQLite) UpdateUser(ctx context.Context, id int64, role auth.Role, active bool) (auth.User, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return auth.User{}, err
	}
	defer tx.Rollback()
	var current auth.User
	if err = tx.QueryRowContext(ctx, `SELECT id,email,role,active FROM users WHERE id=?`, id).Scan(&current.ID, &current.Email, &current.Role, &current.Active); errors.Is(err, sql.ErrNoRows) {
		return auth.User{}, ErrUserNotFound
	} else if err != nil {
		return auth.User{}, err
	}
	if current.Role == auth.RoleAdmin && current.Active && (role != auth.RoleAdmin || !active) {
		var admins int
		if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE role='admin' AND active=1`).Scan(&admins); err != nil {
			return auth.User{}, err
		}
		if admins <= 1 {
			return auth.User{}, ErrLastActiveAdmin
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE users SET role=?,active=? WHERE id=?`, role, active, id); err != nil {
		return auth.User{}, err
	}
	if err = tx.Commit(); err != nil {
		return auth.User{}, err
	}
	return auth.User{ID: id, Email: current.Email, Role: role, Active: active}, nil
}
func (s *SQLite) DeleteUserSessions(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id=?`, userID)
	return err
}
func (s *SQLite) CreateSession(ctx context.Context, nonce string, userID int64, expires time.Time) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO sessions(session_nonce,user_id,expires_at) VALUES(?,?,?)`, nonce, userID, expires.UTC().Format(time.RFC3339))
	return err
}
func (s *SQLite) DeleteSession(ctx context.Context, nonce string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE session_nonce=?`, nonce)
	return err
}
func (s *SQLite) SessionValid(ctx context.Context, nonce string, userID int64, now time.Time) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions WHERE session_nonce=? AND user_id=? AND expires_at>?`, nonce, userID, now.UTC().Format(time.RFC3339)).Scan(&count)
	return count == 1, err
}
func (s *SQLite) Audit(ctx context.Context, actorID int64, action, target string, metadata map[string]any) error {
	payload, err := json.Marshal(safeAuditMetadata(metadata))
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO audit_log(actor_id,action,target,metadata_json) VALUES(?,?,?,?)`, actorID, action, target, string(payload))
	return err
}
func (s *SQLite) AuditEntries(ctx context.Context, limit int) ([]AuditEntry, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,actor_id,action,target,metadata_json,created_at FROM audit_log ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []AuditEntry{}
	for rows.Next() {
		var item AuditEntry
		var payload string
		if err = rows.Scan(&item.ID, &item.ActorID, &item.Action, &item.Target, &payload, &item.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(payload), &item.Metadata)
		items = append(items, item)
	}
	return items, rows.Err()
}
func safeAuditMetadata(values map[string]any) map[string]any {
	safe := map[string]any{}
	for key, value := range values {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "secret") || strings.Contains(lower, "password") || strings.Contains(lower, "token") || strings.Contains(lower, "cipher") || strings.Contains(lower, "authorization") {
			continue
		}
		switch v := value.(type) {
		case string:
			if len(v) <= 256 {
				safe[key] = v
			}
		case bool, float64, int, int64:
			safe[key] = v
		}
	}
	return safe
}

func (s *SQLite) CreateIntegration(ctx context.Context, item integration.Integration) error {
	if err := item.Validate(); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO integrations(integration_key,name,type,config_json,secret_ciphertext,status) VALUES(?,?,?,?,?,?)`, item.Key, item.Name, item.Type, string(item.Config), item.SecretCiphertext, item.Status)
	return err
}

func (s *SQLite) Integrations(ctx context.Context) ([]integration.Integration, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT integration_key,name,type,config_json,secret_ciphertext,status FROM integrations ORDER BY integration_key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []integration.Integration{}
	for rows.Next() {
		var item integration.Integration
		var config string
		if err = rows.Scan(&item.Key, &item.Name, &item.Type, &config, &item.SecretCiphertext, &item.Status); err != nil {
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
	err := s.db.QueryRowContext(ctx, `SELECT integration_key,name,type,config_json,secret_ciphertext,status FROM integrations WHERE integration_key=?`, key).Scan(&item.Key, &item.Name, &item.Type, &config, &item.SecretCiphertext, &item.Status)
	if err != nil {
		return integration.Integration{}, err
	}
	item.Config = []byte(config)
	return item, nil
}

func (s *SQLite) UpdateIntegration(ctx context.Context, item integration.Integration) error {
	if err := item.Validate(); err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `UPDATE integrations SET name=?,type=?,config_json=?,secret_ciphertext=?,status=? WHERE integration_key=?`, item.Name, item.Type, string(item.Config), item.SecretCiphertext, item.Status, item.Key)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return ErrIntegrationNotFound
	}
	return nil
}

func (s *SQLite) DisableIntegration(ctx context.Context, key string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE integrations SET status='disabled' WHERE integration_key=?`, key)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return ErrIntegrationNotFound
	}
	return nil
}

func (s *SQLite) DeleteIntegration(ctx context.Context, key string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var references int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM workflow_versions WHERE instr(definition_json, ?) > 0`, key).Scan(&references); err != nil {
		return err
	}
	if references > 0 {
		return ErrIntegrationReferenced
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM integrations WHERE integration_key=?`, key)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return ErrIntegrationNotFound
	}
	return tx.Commit()
}

func (s *SQLite) Repositories(ctx context.Context, key string) ([]integration.Repository, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT integration_key,owner,repo FROM integration_repositories WHERE integration_key=? ORDER BY owner,repo`, key)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []integration.Repository{}
	for rows.Next() {
		var item integration.Repository
		if err = rows.Scan(&item.IntegrationKey, &item.Owner, &item.Name); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *SQLite) ReplaceRepositories(ctx context.Context, key string, items []integration.Repository) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var kind, status string
	if err = tx.QueryRowContext(ctx, `SELECT type,status FROM integrations WHERE integration_key=?`, key).Scan(&kind, &status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrIntegrationNotFound
		}
		return err
	}
	if kind != integration.TypeGitea || status != integration.StatusActive {
		return fmt.Errorf("repositories require an active Gitea connection")
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM integration_repositories WHERE integration_key=?`, key); err != nil {
		return err
	}
	for _, item := range items {
		item.IntegrationKey = key
		if err = item.Validate(); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO integration_repositories(integration_key,owner,repo) VALUES(?,?,?)`, key, item.Owner, item.Name); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ReplaceModelProfiles atomically makes the selected provider models reusable
// profiles. Keys are deterministic per integration/model to avoid secret data.
func (s *SQLite) ReplaceModelProfiles(ctx context.Context, key string, models []string) ([]integration.ModelProfile, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var kind, status string
	if err = tx.QueryRowContext(ctx, `SELECT type,status FROM integrations WHERE integration_key=?`, key).Scan(&kind, &status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrIntegrationNotFound
		}
		return nil, err
	}
	if (kind != integration.TypeOpenAI && kind != integration.TypeOllama) || status != integration.StatusActive {
		return nil, fmt.Errorf("models require an active LLM connection")
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM model_profiles WHERE integration_key=?`, key); err != nil {
		return nil, err
	}
	profiles := make([]integration.ModelProfile, 0, len(models))
	seen := map[string]bool{}
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" || seen[model] {
			continue
		}
		seen[model] = true
		profile := integration.ModelProfile{Key: profileKey(key, model), Name: model, IntegrationKey: key, Model: model, Status: integration.StatusActive}
		if err = profile.Validate(); err != nil {
			return nil, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO model_profiles(profile_key,name,integration_key,model,status) VALUES(?,?,?,?,?)`, profile.Key, profile.Name, key, model, profile.Status); err != nil {
			return nil, err
		}
		profiles = append(profiles, profile)
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return profiles, nil
}

func profileKey(connection, model string) string {
	return strings.NewReplacer("/", "-", ":", "-", " ", "-").Replace(connection + "-" + model)
}

func (s *SQLite) CreateModelProfile(ctx context.Context, profile integration.ModelProfile) error {
	if err := profile.Validate(); err != nil {
		return err
	}
	item, err := s.Integration(ctx, profile.IntegrationKey)
	if err != nil {
		return fmt.Errorf("model profile integration %q is not configured", profile.IntegrationKey)
	}
	if item.Type != integration.TypeOpenAI && item.Type != integration.TypeOllama {
		return fmt.Errorf("model profile integration %q must be an LLM connection", profile.IntegrationKey)
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO model_profiles(profile_key,name,integration_key,model,status) VALUES(?,?,?,?,?)`, profile.Key, profile.Name, profile.IntegrationKey, profile.Model, profile.Status)
	return err
}

func (s *SQLite) ModelProfiles(ctx context.Context) ([]integration.ModelProfile, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT profile_key,name,integration_key,model,status FROM model_profiles ORDER BY profile_key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	profiles := []integration.ModelProfile{}
	for rows.Next() {
		var profile integration.ModelProfile
		if err = rows.Scan(&profile.Key, &profile.Name, &profile.IntegrationKey, &profile.Model, &profile.Status); err != nil {
			return nil, err
		}
		profiles = append(profiles, profile)
	}
	return profiles, rows.Err()
}

func (s *SQLite) ModelProfile(ctx context.Context, key string) (integration.ModelProfile, error) {
	var profile integration.ModelProfile
	err := s.db.QueryRowContext(ctx, `SELECT profile_key,name,integration_key,model,status FROM model_profiles WHERE profile_key=?`, key).Scan(&profile.Key, &profile.Name, &profile.IntegrationKey, &profile.Model, &profile.Status)
	return profile, err
}

func (s *SQLite) Save(ctx context.Context, definition workflow.Definition) (int64, error) {
	if err := workflow.ValidateNoFixedPullRequestConfig(definition); err != nil {
		return 0, err
	}
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

// EnsureOfficialReviewWorkflow creates and publishes the seeded official review
// pipeline only when its key has no published version. User workflows and
// historical official drafts are intentionally left untouched.
func (s *SQLite) EnsureOfficialReviewWorkflow(ctx context.Context, catalog workflow.Catalog) (workflow.VersionSummary, bool, error) {
	definition := workflow.OfficialReviewDefinition()
	if err := workflow.Validate(definition, catalog); err != nil {
		return workflow.VersionSummary{}, false, fmt.Errorf("validate official review workflow: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return workflow.VersionSummary{}, false, err
	}
	defer tx.Rollback()

	var existing workflow.VersionSummary
	var existingPayload string
	err = tx.QueryRowContext(ctx, `SELECT id,version,status,created_at,definition_json FROM workflow_versions WHERE workflow_key=? AND status=?`, definition.Key, workflow.VersionStatusPublished).Scan(&existing.ID, &existing.Version, &existing.Status, &existing.CreatedAt, &existingPayload)
	if err == nil {
		var stored workflow.Definition
		legacyJSON, _ := json.Marshal(workflow.PreviousOfficialReviewDefinition())
		storedJSON := []byte(existingPayload)
		if json.Unmarshal(storedJSON, &stored) != nil {
			return workflow.VersionSummary{}, false, fmt.Errorf("decode existing official workflow")
		}
		normalized, _ := json.Marshal(stored)
		if string(normalized) != string(legacyJSON) {
			if err = tx.Commit(); err != nil {
				return workflow.VersionSummary{}, false, err
			}
			return existing, false, nil
		}
		if _, err = tx.ExecContext(ctx, `UPDATE workflow_versions SET status=? WHERE id=?`, workflow.VersionStatusArchived, existing.ID); err != nil {
			return workflow.VersionSummary{}, false, err
		}
		err = sql.ErrNoRows // continue through the append-only seed path
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return workflow.VersionSummary{}, false, err
	}

	payload, err := json.Marshal(definition)
	if err != nil {
		return workflow.VersionSummary{}, false, err
	}
	var latestVersion int
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM workflow_versions WHERE workflow_key=?`, definition.Key).Scan(&latestVersion); err != nil {
		return workflow.VersionSummary{}, false, err
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO workflow_versions(workflow_key,version,name,description,status,definition_json) VALUES(?,?,?,?,?,?)`, definition.Key, latestVersion+1, definition.Name, definition.Description, workflow.VersionStatusPublished, string(payload))
	if err != nil {
		return workflow.VersionSummary{}, false, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return workflow.VersionSummary{}, false, err
	}
	if err = tx.QueryRowContext(ctx, `SELECT id,version,status,created_at FROM workflow_versions WHERE id=?`, id).Scan(&existing.ID, &existing.Version, &existing.Status, &existing.CreatedAt); err != nil {
		return workflow.VersionSummary{}, false, err
	}
	if err = tx.Commit(); err != nil {
		return workflow.VersionSummary{}, false, err
	}
	return existing, true, nil
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

// VersionStatus returns the immutable lifecycle state for a saved version.
func (s *SQLite) VersionStatus(ctx context.Context, id int64) (string, error) {
	var status string
	err := s.db.QueryRowContext(ctx, `SELECT status FROM workflow_versions WHERE id=?`, id).Scan(&status)
	return status, err
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
	return s.CreateTriggeredExecution(ctx, versionID, "", input)
}

func (s *SQLite) CreateTriggeredExecution(ctx context.Context, versionID int64, triggerNodeKey string, input map[string]any) (int64, error) {
	payload, err := json.Marshal(input)
	if err != nil {
		return 0, err
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO workflow_executions(workflow_version_id,status,metadata_json,execution_input_json,trigger_node_key) VALUES(?, 'queued', '{}', '{}', ?)`, versionID, triggerNodeKey)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	if _, err = s.db.ExecContext(ctx, `INSERT INTO workflow_execution_sensitive(execution_id,input_json) VALUES(?,?)`, id, string(payload)); err != nil {
		return 0, err
	}
	_ = s.appendExecutionEvent(ctx, id, "execution", "queued", nil)
	return id, nil
}

// ClaimExecution atomically transitions a queued execution to running. A
// duplicate queue delivery receives false and must not execute it again.
func (s *SQLite) ClaimExecution(ctx context.Context, id int64) (workflow.Execution, bool, error) {
	var nextRetry string
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(next_retry_at,'') FROM workflow_executions WHERE id=? AND status='queued'`, id).Scan(&nextRetry); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return workflow.Execution{}, false, nil
		}
		return workflow.Execution{}, false, err
	}
	if nextRetry != "" {
		if at, err := time.Parse(time.RFC3339Nano, nextRetry); err == nil && at.After(time.Now().UTC()) {
			return workflow.Execution{}, false, nil
		}
	}
	result, err := s.db.ExecContext(ctx, `UPDATE workflow_executions SET status='running',started_at=CURRENT_TIMESTAMP WHERE id=? AND status='queued'`, id)
	if err != nil {
		return workflow.Execution{}, false, err
	}
	claimed, err := result.RowsAffected()
	if err != nil || claimed == 0 {
		return workflow.Execution{}, false, err
	}
	var execution workflow.Execution
	var input string
	err = s.db.QueryRowContext(ctx, `SELECT e.workflow_version_id,e.trigger_node_key,COALESCE(p.input_json,'{}') FROM workflow_executions e LEFT JOIN workflow_execution_sensitive p ON p.execution_id=e.id WHERE e.id=?`, id).Scan(&execution.VersionID, &execution.TriggerNodeKey, &input)
	if err != nil {
		return workflow.Execution{}, false, err
	}
	execution.ID = id
	if err = json.Unmarshal([]byte(input), &execution.Input); err != nil {
		return workflow.Execution{}, false, fmt.Errorf("decode execution input: %w", err)
	}
	_ = s.appendExecutionEvent(ctx, id, "execution", "running", nil)
	return execution, true, nil
}

const maxExecutionRetries = 3

// HandleExecutionFailure records a safe failure class. Only transient failures
// are retried, and every other class (or exhausted retry) is terminal DLQ.
func (s *SQLite) HandleExecutionFailure(ctx context.Context, id int64, class string) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var retries int
	if err = tx.QueryRowContext(ctx, `SELECT retry_count FROM workflow_executions WHERE id=? AND status='running'`, id).Scan(&retries); err != nil {
		return false, err
	}
	if class == "transient" && retries < maxExecutionRetries {
		next := time.Now().UTC().Add(retryDelay(id, retries+1)).Format(time.RFC3339Nano)
		if _, err = tx.ExecContext(ctx, `UPDATE workflow_executions SET status='queued',retry_count=retry_count+1,last_failure_class=?,next_retry_at=? WHERE id=?`, class, next, id); err != nil {
			return false, err
		}
		if err = tx.Commit(); err != nil {
			return false, err
		}
		_ = s.appendExecutionEvent(ctx, id, "execution", "queued", nil)
		return true, nil
	}
	if _, err = tx.ExecContext(ctx, `UPDATE workflow_executions SET status='dead_letter',last_failure_class=?,finished_at=CURRENT_TIMESTAMP WHERE id=?`, class, id); err != nil {
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	_ = s.appendExecutionEvent(ctx, id, "execution", "dead_letter", nil)
	return false, nil
}

func retryDelay(id int64, retry int) time.Duration {
	base := time.Second * time.Duration(1<<(retry-1))
	// A deterministic bounded jitter avoids synchronized retries without making
	// tests or operator reconstruction depend on process-local randomness.
	jitter := time.Duration((id*1103515245+int64(retry)*12345)%500) * time.Millisecond
	return base + jitter
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

// SaveNodeProgress upserts one node/scope attempt so live running state can be
// queried without creating a second row when the terminal update arrives.
func (s *SQLite) SaveNodeProgress(ctx context.Context, id int64, run workflow.NodeRun) error {
	metadata, scopeKey, err := nodeRunValues(run)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO workflow_node_runs(workflow_execution_id,node_key,scope_key,status,started_at,finished_at,metadata_json)
		VALUES(?,?,?,?,CURRENT_TIMESTAMP,CASE WHEN ?='running' THEN NULL ELSE CURRENT_TIMESTAMP END,?)
		ON CONFLICT(workflow_execution_id,node_key,scope_key,attempt) DO UPDATE SET
		status=excluded.status,finished_at=excluded.finished_at,metadata_json=excluded.metadata_json`, id, run.NodeKey, scopeKey, run.Status, run.Status, string(metadata))
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated != 1 {
		return fmt.Errorf("execution %d node progress was not persisted", id)
	}
	if err = s.saveNodeSensitive(ctx, id, run, scopeKey); err != nil {
		return err
	}
	_ = s.appendExecutionEvent(ctx, id, "node", run.Status, &workflow.ExecutionNodeState{NodeKey: run.NodeKey, ScopeKey: scopeKey, Status: run.Status})
	return nil
}

func nodeRunValues(run workflow.NodeRun) ([]byte, string, error) {
	// The status/event plane retains identity and lifecycle only. The detailed
	// runtime payload is written to the seven-day sensitive store instead.
	metadata, err := json.Marshal(map[string]any{"duration_ms": run.DurationMS})
	if err != nil {
		return nil, "", err
	}
	scopeKey := run.ScopeKey
	if scopeKey == "" {
		scopeKey = "root"
	}
	return metadata, scopeKey, nil
}

// CompleteExecution upserts final node reports and makes the final status visible.
func (s *SQLite) CompleteExecution(ctx context.Context, id int64, report workflow.RunReport) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE workflow_executions SET status=?,finished_at=CURRENT_TIMESTAMP WHERE id=? AND status IN ('running','cancellation_requested')`, report.Status, id)
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
		metadata, scopeKey, err := nodeRunValues(run)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO workflow_node_runs(workflow_execution_id,node_key,scope_key,status,started_at,finished_at,metadata_json)
			VALUES(?,?,?,?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,?)
			ON CONFLICT(workflow_execution_id,node_key,scope_key,attempt) DO UPDATE SET status=excluded.status,finished_at=CURRENT_TIMESTAMP,metadata_json=excluded.metadata_json`, id, run.NodeKey, scopeKey, run.Status, string(metadata)); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	_ = s.appendExecutionEvent(ctx, id, "execution", report.Status, nil)
	return nil
}

func (s *SQLite) FailQueuedExecution(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE workflow_executions SET status='failed',finished_at=CURRENT_TIMESTAMP WHERE id=? AND status='queued'`, id)
	return err
}

func (s *SQLite) QueuedExecutionIDs(ctx context.Context) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM workflow_executions WHERE status='queued' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// RecoverableExecutionIDs returns due queued work and atomically returns stale
// running work to the durable queue. It is safe to call repeatedly; claim is
// still the single execution owner boundary.
func (s *SQLite) RecoverableExecutionIDs(ctx context.Context, staleAfter time.Duration) ([]int64, error) {
	if staleAfter <= 0 {
		return nil, fmt.Errorf("stale execution threshold must be positive")
	}
	// SQLite CURRENT_TIMESTAMP uses a space-separated timestamp. Compare in
	// SQLite rather than against RFC3339 text so an active execution is not
	// spuriously marked stale due to lexical timestamp-format differences.
	if _, err := s.db.ExecContext(ctx, `UPDATE workflow_executions SET status='queued',last_failure_class='uncertain' WHERE status='running' AND started_at < datetime('now', ?)`, fmt.Sprintf("-%d seconds", int64(staleAfter.Seconds()))); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,COALESCE(next_retry_at,'') FROM workflow_executions WHERE status='queued' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []int64{}
	now := time.Now().UTC()
	for rows.Next() {
		var id int64
		var next string
		if err = rows.Scan(&id, &next); err != nil {
			return nil, err
		}
		if next == "" {
			ids = append(ids, id)
			continue
		}
		at, parseErr := time.Parse(time.RFC3339Nano, next)
		if parseErr == nil && !at.After(now) {
			ids = append(ids, id)
		}
	}
	return ids, rows.Err()
}

func (s *SQLite) ReplayDeadLetterExecution(ctx context.Context, id int64) (int64, error) {
	var status string
	if err := s.db.QueryRowContext(ctx, `SELECT status FROM workflow_executions WHERE id=?`, id).Scan(&status); err != nil {
		return 0, err
	}
	if status != "dead_letter" {
		return 0, fmt.Errorf("execution is not in dead letter")
	}
	return s.ReprocessExecution(ctx, id)
}

var ErrExecutionNotCancellable = errors.New("execution can no longer be cancelled")

// CancelExecution is intentionally rejected after a publication attempt exists.
// Once external publication begins, the workflow must complete through its
// idempotency boundary rather than pretending the provider call was cancelled.
func (s *SQLite) CancelExecution(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var status string
	if err = tx.QueryRowContext(ctx, `SELECT status FROM workflow_executions WHERE id=?`, id).Scan(&status); err != nil {
		return err
	}
	if status != "queued" && status != "running" {
		return ErrExecutionNotCancellable
	}
	var publications int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM publication_attempts WHERE workflow_execution_id=?`, id).Scan(&publications); err != nil {
		return err
	}
	if publications > 0 {
		return ErrExecutionNotCancellable
	}
	next := "cancelled"
	if status == "running" {
		next = "cancellation_requested"
	}
	if _, err = tx.ExecContext(ctx, `UPDATE workflow_executions SET status=?,finished_at=CASE WHEN ?='cancelled' THEN CURRENT_TIMESTAMP ELSE finished_at END WHERE id=?`, next, next, id); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return s.appendExecutionEvent(ctx, id, "execution", next, nil)
}

func (s *SQLite) CancellationRequested(ctx context.Context, id int64) (bool, error) {
	var status string
	err := s.db.QueryRowContext(ctx, `SELECT status FROM workflow_executions WHERE id=?`, id).Scan(&status)
	return status == "cancellation_requested", err
}

// Reprocess creates a new durable execution from the retained input. It never
// claims to resume an interrupted in-memory execution.
func (s *SQLite) ReprocessExecution(ctx context.Context, id int64) (int64, error) {
	var versionID int64
	var trigger, input string
	err := s.db.QueryRowContext(ctx, `SELECT e.workflow_version_id,e.trigger_node_key,COALESCE(p.input_json,'') FROM workflow_executions e LEFT JOIN workflow_execution_sensitive p ON p.execution_id=e.id WHERE e.id=?`, id).Scan(&versionID, &trigger, &input)
	if err != nil {
		return 0, err
	}
	if input == "" {
		return 0, fmt.Errorf("execution input expired; reprocess is unavailable")
	}
	values := map[string]any{}
	if json.Unmarshal([]byte(input), &values) != nil {
		return 0, fmt.Errorf("stored execution input is invalid")
	}
	return s.CreateTriggeredExecution(ctx, versionID, trigger, values)
}

func (s *SQLite) PublishedVersion(ctx context.Context, workflowKey string) (int64, workflow.Definition, error) {
	var id int64
	var payload string
	err := s.db.QueryRowContext(ctx, `SELECT id,definition_json FROM workflow_versions WHERE workflow_key=? AND status='published'`, workflowKey).Scan(&id, &payload)
	if err != nil {
		return 0, workflow.Definition{}, err
	}
	var definition workflow.Definition
	if err = json.Unmarshal([]byte(payload), &definition); err != nil {
		return 0, workflow.Definition{}, err
	}
	return id, definition, nil
}

// DeleteWorkflowVersion removes one non-published immutable version only when
// retained execution, publication, webhook, and audit evidence does not refer
// to it. Historical evidence is never deleted.
func (s *SQLite) DeleteWorkflowVersion(ctx context.Context, id, actorID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var workflowKey, status string
	if err = tx.QueryRowContext(ctx, `SELECT workflow_key,status FROM workflow_versions WHERE id=?`, id).Scan(&workflowKey, &status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrWorkflowVersionNotFound
		}
		return err
	}
	if status == workflow.VersionStatusPublished {
		return ErrWorkflowVersionNotDeletable
	}
	var count int
	reasons := []string{}
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM workflow_executions WHERE workflow_version_id=?`, id).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		reasons = append(reasons, "execution history")
	}
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM publication_attempts WHERE workflow_version_id=?`, id).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		reasons = append(reasons, "publication attempts")
	}
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM webhook_registrations WHERE workflow_key=?`, workflowKey).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		reasons = append(reasons, "webhook registrations")
	}
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_log WHERE target=?`, fmt.Sprintf("workflow_version:%d", id)).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		reasons = append(reasons, "audit history")
	}
	if len(reasons) > 0 {
		return fmt.Errorf("%w: %s", ErrWorkflowVersionDeletionBlocked, strings.Join(reasons, ", "))
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM workflow_versions WHERE id=?`, id); err != nil {
		return err
	}
	if actorID > 0 {
		if _, err = tx.ExecContext(ctx, `INSERT INTO audit_log(actor_id,action,target,metadata_json) VALUES(?,?,?,?)`, actorID, "workflow_version.deleted", fmt.Sprintf("workflow_version:%d", id), `{}`); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLite) UpsertWebhookRegistration(ctx context.Context, item workflow.WebhookRegistration) error {
	if strings.TrimSpace(item.Key) == "" || strings.TrimSpace(item.Name) == "" || strings.TrimSpace(item.WorkflowKey) == "" || strings.TrimSpace(item.TriggerNodeKey) == "" || strings.TrimSpace(item.SecretCiphertext) == "" {
		return fmt.Errorf("webhook registration key, name, workflow, trigger, and secret are required")
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO webhook_registrations(registration_key,name,workflow_key,trigger_node_key,secret_ciphertext,active) VALUES(?,?,?,?,?,?)
		ON CONFLICT(registration_key) DO UPDATE SET name=excluded.name,workflow_key=excluded.workflow_key,trigger_node_key=excluded.trigger_node_key,secret_ciphertext=excluded.secret_ciphertext,active=excluded.active`, item.Key, item.Name, item.WorkflowKey, item.TriggerNodeKey, item.SecretCiphertext, item.Active)
	return err
}

func (s *SQLite) WebhookRegistration(ctx context.Context, key string) (workflow.WebhookRegistration, error) {
	var item workflow.WebhookRegistration
	err := s.db.QueryRowContext(ctx, `SELECT registration_key,name,workflow_key,trigger_node_key,secret_ciphertext,active FROM webhook_registrations WHERE registration_key=?`, key).Scan(&item.Key, &item.Name, &item.WorkflowKey, &item.TriggerNodeKey, &item.SecretCiphertext, &item.Active)
	item.SecretConfigured = item.SecretCiphertext != ""
	return item, err
}

func (s *SQLite) WebhookRegistrations(ctx context.Context) ([]workflow.WebhookRegistration, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT registration_key,name,workflow_key,trigger_node_key,secret_ciphertext,active FROM webhook_registrations ORDER BY registration_key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []workflow.WebhookRegistration{}
	for rows.Next() {
		var item workflow.WebhookRegistration
		if err = rows.Scan(&item.Key, &item.Name, &item.WorkflowKey, &item.TriggerNodeKey, &item.SecretCiphertext, &item.Active); err != nil {
			return nil, err
		}
		item.SecretConfigured = item.SecretCiphertext != ""
		items = append(items, item)
	}
	return items, rows.Err()
}

// CreateWebhookExecution atomically records delivery identity and durable input.
// Equal retries return the original execution; reused IDs with different bodies collide.
func (s *SQLite) CreateWebhookExecution(ctx context.Context, registrationKey, deliveryID, bodyHash string, versionID int64, triggerNodeKey string, input map[string]any) (int64, bool, error) {
	payload, err := json.Marshal(input)
	if err != nil {
		return 0, false, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback()
	var existingHash string
	var existingID int64
	err = tx.QueryRowContext(ctx, `SELECT body_sha256,workflow_execution_id FROM webhook_deliveries WHERE registration_key=? AND delivery_id=?`, registrationKey, deliveryID).Scan(&existingHash, &existingID)
	if err == nil {
		if existingHash != bodyHash {
			return 0, false, ErrWebhookDeliveryCollision
		}
		return existingID, true, tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, false, err
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO workflow_executions(workflow_version_id,status,metadata_json,execution_input_json,trigger_node_key) VALUES(?,'queued','{}','{}',?)`, versionID, triggerNodeKey)
	if err != nil {
		return 0, false, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, false, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO workflow_execution_sensitive(execution_id,input_json) VALUES(?,?)`, id, string(payload)); err != nil {
		return 0, false, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO webhook_deliveries(registration_key,delivery_id,body_sha256,workflow_execution_id) VALUES(?,?,?,?)`, registrationKey, deliveryID, bodyHash, id); err != nil {
		return 0, false, err
	}
	return id, false, tx.Commit()
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
	var executionStatus string
	err = tx.QueryRowContext(ctx, `SELECT status FROM workflow_executions WHERE id=?`, attempt.ExecutionID).Scan(&executionStatus)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return workflow.PublicationAttempt{}, false, err
	}
	if err == nil && executionStatus != "running" && executionStatus != "queued" {
		return workflow.PublicationAttempt{}, false, fmt.Errorf("execution is not publishable")
	}
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
func (s *SQLite) UncertainPublication(ctx context.Context, key string, cause error) error {
	result, err := s.db.ExecContext(ctx, `UPDATE publication_attempts SET status='uncertain',error_message='publication outcome uncertain' WHERE idempotency_key=? AND status='pending'`, key)
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
func (s *SQLite) ReconcilePublication(ctx context.Context, key string, receipt *integration.PublicationReceipt) error {
	if receipt != nil {
		payload, err := json.Marshal(*receipt)
		if err != nil {
			return err
		}
		result, err := s.db.ExecContext(ctx, `UPDATE publication_attempts SET status='completed',receipt_json=?,completed_at=CURRENT_TIMESTAMP,error_message='' WHERE idempotency_key=? AND status='uncertain'`, string(payload), key)
		if err != nil {
			return err
		}
		changed, _ := result.RowsAffected()
		if changed != 1 {
			return fmt.Errorf("publication %q is not uncertain", key)
		}
		return nil
	}
	result, err := s.db.ExecContext(ctx, `UPDATE publication_attempts SET status='retryable',error_message='' WHERE idempotency_key=? AND status='uncertain'`, key)
	if err != nil {
		return err
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		return fmt.Errorf("publication %q is not uncertain", key)
	}
	return nil
}

func (s *SQLite) Execution(ctx context.Context, id int64) (workflow.ExecutionStatus, error) {
	report := workflow.ExecutionStatus{Runs: []workflow.ExecutionNodeState{}}
	if err := s.db.QueryRowContext(ctx, `SELECT id,status,started_at,COALESCE(finished_at,'') FROM workflow_executions WHERE id=?`, id).Scan(&report.ID, &report.Status, &report.StartedAt, &report.FinishedAt); err != nil {
		return workflow.ExecutionStatus{}, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT node_key,scope_key,status,metadata_json FROM workflow_node_runs WHERE workflow_execution_id=? ORDER BY id`, id)
	if err != nil {
		return workflow.ExecutionStatus{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var run workflow.ExecutionNodeState
		var ignored string
		if err = rows.Scan(&run.NodeKey, &run.ScopeKey, &run.Status, &ignored); err != nil {
			return workflow.ExecutionStatus{}, err
		}
		report.Runs = append(report.Runs, run)
	}
	return report, rows.Err()
}

// CardExecutionLogs provides a detailed but safe diagnostic view for one card.
// The sensitive payload contains raw inputs/outputs, so this method selectively
// copies only fixed operational facts from it.
func (s *SQLite) CardExecutionLogs(ctx context.Context, executionID int64, nodeKey string) ([]workflow.ExecutionCardLog, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT r.id,r.node_key,r.scope_key,r.status,COALESCE(r.started_at,''),COALESCE(r.finished_at,''),r.metadata_json,COALESCE(s.payload_json,'{}')
		FROM workflow_node_runs r LEFT JOIN workflow_node_run_sensitive s
		ON s.execution_id=r.workflow_execution_id AND s.node_key=r.node_key AND s.scope_key=r.scope_key
		WHERE r.workflow_execution_id=? AND r.node_key=? ORDER BY r.id`, executionID, nodeKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []workflow.ExecutionCardLog{}
	for rows.Next() {
		var item workflow.ExecutionCardLog
		var publicJSON, sensitiveJSON string
		if err = rows.Scan(&item.ID, &item.NodeKey, &item.ScopeKey, &item.Status, &item.StartedAt, &item.FinishedAt, &publicJSON, &sensitiveJSON); err != nil {
			return nil, err
		}
		item.ExecutionID = executionID
		var publicMetadata map[string]any
		_ = json.Unmarshal([]byte(publicJSON), &publicMetadata)
		item.DurationMS = executionLogInt(publicMetadata["duration_ms"])
		var sensitive struct {
			Error    string         `json:"error"`
			Metadata map[string]any `json:"metadata"`
			Inputs   any            `json:"inputs"`
			Outputs  any            `json:"outputs"`
		}
		_ = json.Unmarshal([]byte(sensitiveJSON), &sensitive)
		item.Error = fmt.Sprintf("%v", workflow.ExecutionLogDiagnostic(sensitive.Error))
		item.Facts = safeExecutionLogFacts(sensitive.Metadata)
		item.Inputs = workflow.ExecutionLogDiagnostic(sensitive.Inputs)
		item.Outputs = workflow.ExecutionLogDiagnostic(sensitive.Outputs)
		if len(item.Facts) == 0 {
			item.Facts = nil
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func executionLogInt(value any) int64 {
	switch item := value.(type) {
	case float64:
		return int64(item)
	case int:
		return int64(item)
	case int64:
		return item
	default:
		return 0
	}
}

func safeExecutionLogFacts(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return nil
	}
	allowed := map[string]bool{
		"attempt_count": true, "retry_limit": true, "retry_delay_ms": true,
		"completed_iterations": true, "failed_iterations": true, "max_iterations": true,
		"concurrency": true, "model": true, "validation_error": true, "error_code": true,
		"error_policy": true, "error_action": true, "error_scope": true,
	}
	result := map[string]any{}
	for key := range allowed {
		value, ok := metadata[key]
		if !ok {
			continue
		}
		switch value.(type) {
		case string, float64, bool, int, int64:
			result[key] = value
		}
	}
	for _, key := range []string{"validation_attempts", "provider_calls"} {
		if values, ok := metadata[key].([]any); ok {
			attempts := make([]map[string]any, 0, len(values))
			for _, raw := range values {
				item, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				attempt := map[string]any{}
				if value, ok := item["attempt"].(float64); ok {
					attempt["attempt"] = int(value)
				}
				if value, ok := item["status"].(string); ok {
					attempt["status"] = value
				}
				if len(attempt) > 0 {
					attempts = append(attempts, attempt)
				}
			}
			if len(attempts) > 0 {
				result[key] = attempts
			}
		}
	}
	return result
}

// ExecutionSummaries returns a bounded, dashboard-safe view of recent runs.
// It deliberately avoids returning execution input, node-run metadata, errors,
// or integration configuration. Displayable PR coordinates are derived only
// from the typed execution input.
func (s *SQLite) ExecutionSummaries(ctx context.Context, limit int) ([]workflow.ExecutionSummary, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT e.id,e.status,e.started_at,COALESCE(e.finished_at,''),v.workflow_key,v.name,v.version,COALESCE(p.input_json,'{}')
		FROM workflow_executions e JOIN workflow_versions v ON v.id=e.workflow_version_id
		LEFT JOIN workflow_execution_sensitive p ON p.execution_id=e.id
		ORDER BY e.started_at DESC,e.id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []workflow.ExecutionSummary{}
	for rows.Next() {
		var item workflow.ExecutionSummary
		var inputJSON string
		if err = rows.Scan(&item.ID, &item.Status, &item.StartedAt, &item.FinishedAt, &item.Workflow.Key, &item.Workflow.Name, &item.Workflow.Version, &inputJSON); err != nil {
			return nil, err
		}
		item.Review = reviewContextFromInput(inputJSON)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *SQLite) Metrics(ctx context.Context) (ExecutionMetrics, error) {
	metrics := ExecutionMetrics{StatusCounts: map[string]int64{}}
	rows, err := s.db.QueryContext(ctx, `SELECT status,COUNT(*) FROM workflow_executions GROUP BY status`)
	if err != nil {
		return metrics, err
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var count int64
		if err = rows.Scan(&status, &count); err != nil {
			return metrics, err
		}
		metrics.StatusCounts[status] = count
	}
	if err = rows.Err(); err != nil {
		return metrics, err
	}
	metrics.QueueDepth = metrics.StatusCounts["queued"]
	metrics.DeadLetterCount = metrics.StatusCounts["dead_letter"]
	if err = s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(retry_count),0),COALESCE(AVG((julianday(finished_at)-julianday(started_at))*86400000),0) FROM workflow_executions WHERE finished_at IS NOT NULL`).Scan(&metrics.RetryCount, &metrics.AverageDuration); err != nil {
		return metrics, err
	}
	return metrics, nil
}

func (s *SQLite) saveNodeSensitive(ctx context.Context, executionID int64, run workflow.NodeRun, scopeKey string) error {
	payload, err := json.Marshal(map[string]any{"inputs": run.Inputs, "outputs": run.Outputs, "error": run.Error, "metadata": run.Metadata})
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO workflow_node_run_sensitive(execution_id,node_key,scope_key,payload_json,expires_at) VALUES(?,?,?,?,COALESCE((SELECT datetime(started_at,'+7 days') FROM workflow_executions WHERE id=?),datetime('now','+7 days'))) ON CONFLICT(execution_id,node_key,scope_key) DO UPDATE SET payload_json=excluded.payload_json,expires_at=excluded.expires_at`, executionID, run.NodeKey, scopeKey, string(payload), executionID)
	return err
}

func (s *SQLite) appendExecutionEvent(ctx context.Context, executionID int64, kind, status string, node *workflow.ExecutionNodeState) error {
	var nodeKey, scopeKey string
	if node != nil {
		nodeKey, scopeKey = node.NodeKey, node.ScopeKey
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO execution_events(execution_id,kind,status,node_key,scope_key) VALUES(?,?,?,?,?)`, executionID, kind, status, nodeKey, scopeKey)
	return err
}

// ExecutionEvents returns a bounded replay of persisted safe lifecycle events.
func (s *SQLite) ExecutionEvents(ctx context.Context, afterID, executionID int64, limit int) ([]workflow.ExecutionEvent, error) {
	query := `SELECT id,execution_id,kind,status,node_key,scope_key,created_at FROM execution_events WHERE id>?`
	args := []any{afterID}
	if executionID > 0 {
		query += ` AND execution_id=?`
		args = append(args, executionID)
	}
	query += ` ORDER BY id LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := []workflow.ExecutionEvent{}
	for rows.Next() {
		var item workflow.ExecutionEvent
		var nodeKey, scopeKey string
		if err = rows.Scan(&item.ID, &item.ExecutionID, &item.Kind, &item.Status, &nodeKey, &scopeKey, &item.CreatedAt); err != nil {
			return nil, err
		}
		if nodeKey != "" {
			item.Node = &workflow.ExecutionNodeState{NodeKey: nodeKey, ScopeKey: scopeKey, Status: item.Status}
		}
		events = append(events, item)
	}
	return events, rows.Err()
}

// PruneExecutionSensitiveData enforces the fixed seven-day retention period.
func (s *SQLite) PruneExecutionSensitiveData(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM workflow_node_run_sensitive WHERE expires_at <= CURRENT_TIMESTAMP`)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM workflow_execution_sensitive WHERE execution_id IN (SELECT id FROM workflow_executions WHERE started_at <= datetime('now','-7 days'))`)
	return err
}

func reviewContextFromInput(payload string) *workflow.ExecutionReviewContext {
	var input map[string]any
	if json.Unmarshal([]byte(payload), &input) != nil {
		return nil
	}
	value, ok := input["pull_request"].(map[string]any)
	if !ok {
		return nil
	}
	owner, _ := value["owner"].(string)
	repo, _ := value["repo"].(string)
	number := value["number"]
	if number == nil {
		number = value["pull_request"]
	}
	candidate, ok := configuredReviewContext(map[string]any{"owner": owner, "repo": repo, "pull_request": number})
	if !ok {
		return nil
	}
	return candidate
}

func configuredReviewContext(config map[string]any) (*workflow.ExecutionReviewContext, bool) {
	owner, ownerOK := config["owner"].(string)
	repo, repoOK := config["repo"].(string)
	if !ownerOK || !repoOK || owner == "" || repo == "" {
		return nil, false
	}
	var pullRequest int
	switch value := config["pull_request"].(type) {
	case float64:
		pullRequest = int(value)
		if value != float64(pullRequest) {
			return nil, false
		}
	case int:
		pullRequest = value
	case int64:
		pullRequest = int(value)
		if int64(pullRequest) != value {
			return nil, false
		}
	default:
		return nil, false
	}
	if pullRequest < 1 {
		return nil, false
	}
	return &workflow.ExecutionReviewContext{Owner: owner, Repo: repo, PullRequest: pullRequest}, true
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
 secret_ciphertext TEXT NOT NULL,
 status TEXT NOT NULL CHECK(status IN ('active','disabled')),
 created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS model_profiles (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  profile_key TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  integration_key TEXT NOT NULL REFERENCES integrations(integration_key),
  model TEXT NOT NULL,
  status TEXT NOT NULL CHECK(status IN ('active','disabled')),
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS integration_repositories (
 integration_key TEXT NOT NULL REFERENCES integrations(integration_key) ON DELETE CASCADE,
 owner TEXT NOT NULL,
 repo TEXT NOT NULL,
 PRIMARY KEY(integration_key,owner,repo)
);
CREATE TABLE IF NOT EXISTS workflow_executions (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 workflow_version_id INTEGER NOT NULL REFERENCES workflow_versions(id),
 status TEXT NOT NULL,
 started_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
 finished_at TEXT,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  execution_input_json TEXT NOT NULL DEFAULT '{}',
	 trigger_node_key TEXT NOT NULL DEFAULT ''
	 ,retry_count INTEGER NOT NULL DEFAULT 0
	 ,last_failure_class TEXT NOT NULL DEFAULT ''
	 ,next_retry_at TEXT
);
CREATE TABLE IF NOT EXISTS workflow_execution_sensitive (
 execution_id INTEGER PRIMARY KEY REFERENCES workflow_executions(id) ON DELETE CASCADE,
 input_json TEXT NOT NULL,
 created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
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
CREATE UNIQUE INDEX IF NOT EXISTS workflow_node_run_attempt ON workflow_node_runs(workflow_execution_id,node_key,scope_key,attempt);
CREATE TABLE IF NOT EXISTS workflow_node_run_sensitive (
 execution_id INTEGER NOT NULL REFERENCES workflow_executions(id) ON DELETE CASCADE,
 node_key TEXT NOT NULL,
 scope_key TEXT NOT NULL,
 payload_json TEXT NOT NULL,
 expires_at TEXT NOT NULL,
 PRIMARY KEY(execution_id,node_key,scope_key)
);
CREATE TABLE IF NOT EXISTS execution_events (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 execution_id INTEGER NOT NULL REFERENCES workflow_executions(id) ON DELETE CASCADE,
 kind TEXT NOT NULL CHECK(kind IN ('execution','node')),
 status TEXT NOT NULL,
 node_key TEXT NOT NULL DEFAULT '',
 scope_key TEXT NOT NULL DEFAULT '',
 created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS execution_events_execution_id ON execution_events(execution_id,id);
CREATE TABLE IF NOT EXISTS publication_attempts (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 idempotency_key TEXT NOT NULL UNIQUE,
 workflow_execution_id INTEGER NOT NULL REFERENCES workflow_executions(id),
 workflow_version_id INTEGER NOT NULL REFERENCES workflow_versions(id),
 node_key TEXT NOT NULL,
 status TEXT NOT NULL CHECK(status IN ('pending','completed','retryable','uncertain')),
 attempt INTEGER NOT NULL DEFAULT 1,
 receipt_json TEXT NOT NULL DEFAULT '{}',
 error_message TEXT NOT NULL DEFAULT '',
 created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
 completed_at TEXT
);
CREATE TABLE IF NOT EXISTS users (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 email TEXT NOT NULL UNIQUE,
 password_hash TEXT NOT NULL,
  role TEXT NOT NULL CHECK(role IN ('viewer','editor','admin')),
	 active INTEGER NOT NULL DEFAULT 1,
 created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS audit_log (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 actor_id INTEGER NOT NULL REFERENCES users(id),
 action TEXT NOT NULL,
 target TEXT NOT NULL,
 metadata_json TEXT NOT NULL DEFAULT '{}',
 created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS sessions (
 session_nonce TEXT PRIMARY KEY,
 user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 expires_at TEXT NOT NULL,
 created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS webhook_registrations (
 registration_key TEXT PRIMARY KEY,
 name TEXT NOT NULL,
 workflow_key TEXT NOT NULL,
 trigger_node_key TEXT NOT NULL,
 secret_ciphertext TEXT NOT NULL,
 active INTEGER NOT NULL DEFAULT 1,
 created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS webhook_deliveries (
 registration_key TEXT NOT NULL REFERENCES webhook_registrations(registration_key),
 delivery_id TEXT NOT NULL,
 body_sha256 TEXT NOT NULL,
 workflow_execution_id INTEGER NOT NULL REFERENCES workflow_executions(id),
 created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
 PRIMARY KEY(registration_key,delivery_id)
);
`

// migrateExecutionSensitiveData separates historical input and node payloads
// from status rows. It is idempotent: fresh databases are already created in
// the split form and legacy values are copied once before being scrubbed.
func (s *SQLite) migrateExecutionSensitiveData(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS workflow_execution_sensitive (execution_id INTEGER PRIMARY KEY REFERENCES workflow_executions(id) ON DELETE CASCADE,input_json TEXT NOT NULL,created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP)`); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS workflow_node_run_sensitive (execution_id INTEGER NOT NULL REFERENCES workflow_executions(id) ON DELETE CASCADE,node_key TEXT NOT NULL,scope_key TEXT NOT NULL,payload_json TEXT NOT NULL,expires_at TEXT NOT NULL,PRIMARY KEY(execution_id,node_key,scope_key))`); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS execution_events (id INTEGER PRIMARY KEY AUTOINCREMENT,execution_id INTEGER NOT NULL REFERENCES workflow_executions(id) ON DELETE CASCADE,kind TEXT NOT NULL,status TEXT NOT NULL,node_key TEXT NOT NULL DEFAULT '',scope_key TEXT NOT NULL DEFAULT '',created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP)`); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS execution_events_execution_id ON execution_events(execution_id,id)`); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO workflow_execution_sensitive(execution_id,input_json) SELECT id,execution_input_json FROM workflow_executions WHERE execution_input_json <> '{}'`); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE workflow_executions SET execution_input_json='{}' WHERE execution_input_json <> '{}'`); err != nil {
		return err
	}
	// Legacy node metadata can contain input/output values. Preserve it under the
	// retention boundary, then replace the public-row metadata with an empty map.
	if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO workflow_node_run_sensitive(execution_id,node_key,scope_key,payload_json,expires_at) SELECT workflow_execution_id,node_key,scope_key,metadata_json,datetime('now','+7 days') FROM workflow_node_runs WHERE metadata_json <> '{}'`); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `UPDATE workflow_node_runs SET metadata_json='{}' WHERE metadata_json <> '{}'`)
	return err
}

func (s *SQLite) ensureExecutionRetryColumns(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `PRAGMA table_info(workflow_executions)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	have := map[string]bool{}
	for rows.Next() {
		var cid, notNull, primary int
		var name, typ string
		var def any
		if err = rows.Scan(&cid, &name, &typ, &notNull, &def, &primary); err != nil {
			return err
		}
		have[name] = true
	}
	if err = rows.Err(); err != nil {
		return err
	}
	for _, column := range []struct{ name, definition string }{
		{"retry_count", "INTEGER NOT NULL DEFAULT 0"},
		{"last_failure_class", "TEXT NOT NULL DEFAULT ''"},
		{"next_retry_at", "TEXT"},
	} {
		if !have[column.name] {
			if _, err = s.db.ExecContext(ctx, `ALTER TABLE workflow_executions ADD COLUMN `+column.name+` `+column.definition); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *SQLite) ensureUserActiveColumn(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `PRAGMA table_info(users)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notNull, primary int
		var name, typ string
		var def any
		if err = rows.Scan(&cid, &name, &typ, &notNull, &def, &primary); err != nil {
			return err
		}
		if name == "active" {
			return nil
		}
	}
	if err = rows.Err(); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `ALTER TABLE users ADD COLUMN active INTEGER NOT NULL DEFAULT 1`)
	return err
}
func (s *SQLite) migratePublicationUncertain(ctx context.Context) error {
	var sqlText string
	if err := s.db.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE type='table' AND name='publication_attempts'`).Scan(&sqlText); err != nil {
		return err
	}
	if strings.Contains(sqlText, "'uncertain'") {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `CREATE TABLE publication_attempts_new (id INTEGER PRIMARY KEY AUTOINCREMENT,idempotency_key TEXT NOT NULL UNIQUE,workflow_execution_id INTEGER NOT NULL REFERENCES workflow_executions(id),workflow_version_id INTEGER NOT NULL REFERENCES workflow_versions(id),node_key TEXT NOT NULL,status TEXT NOT NULL CHECK(status IN ('pending','completed','retryable','uncertain')),attempt INTEGER NOT NULL DEFAULT 1,receipt_json TEXT NOT NULL DEFAULT '{}',error_message TEXT NOT NULL DEFAULT '',created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,completed_at TEXT)`)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO publication_attempts_new SELECT * FROM publication_attempts`); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DROP TABLE publication_attempts`); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `ALTER TABLE publication_attempts_new RENAME TO publication_attempts`); err != nil {
		return err
	}
	return tx.Commit()
}

// migrateIntegrationSecrets removes the former environment-variable reference
// column. Existing records remain as unconfigured connections because a
// reference never contained credential material and cannot be converted to an
// encrypted value. Administrators must provide a new one-time secret before
// those connections can execute.
func (s *SQLite) migrateIntegrationSecrets(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `PRAGMA table_info(integrations)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	hasLegacyReference, hasCiphertext := false, false
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull int
		var defaultValue any
		var primaryKey int
		if err = rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return err
		}
		if name == "secret_reference" {
			hasLegacyReference = true
		}
		if name == "secret_ciphertext" {
			hasCiphertext = true
		}
	}
	if err = rows.Err(); err != nil || (!hasLegacyReference && hasCiphertext) {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `CREATE TABLE integrations_secure (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 integration_key TEXT NOT NULL UNIQUE,
 name TEXT NOT NULL,
 type TEXT NOT NULL CHECK(type IN ('gitea','openai','ollama')),
 config_json TEXT NOT NULL,
 secret_ciphertext TEXT NOT NULL,
 status TEXT NOT NULL CHECK(status IN ('active','disabled')),
 created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
)`); err != nil {
		return err
	}
	ciphertext := "''"
	if hasCiphertext {
		ciphertext = "secret_ciphertext"
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO integrations_secure(id,integration_key,name,type,config_json,secret_ciphertext,status,created_at)
 SELECT id,integration_key,name,type,config_json,`+ciphertext+`,status,created_at FROM integrations`); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DROP TABLE integrations`); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `ALTER TABLE integrations_secure RENAME TO integrations`); err != nil {
		return err
	}
	return tx.Commit()
}

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

func (s *SQLite) ensureExecutionTriggerColumn(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `PRAGMA table_info(workflow_executions)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, dataType string
		var defaultValue any
		if err = rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			return err
		}
		if name == "trigger_node_key" {
			return nil
		}
	}
	if err = rows.Err(); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `ALTER TABLE workflow_executions ADD COLUMN trigger_node_key TEXT NOT NULL DEFAULT ''`)
	return err
}

// migrateFixedPullRequestCoordinates intentionally rewrites immutable version
// payloads while preserving their IDs and lifecycle statuses. Fixed coordinates
// are removed and every publish target is made explicit in the typed graph.
func (s *SQLite) migrateFixedPullRequestCoordinates(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT id,definition_json FROM workflow_versions ORDER BY id`)
	if err != nil {
		return err
	}
	type storedDefinition struct {
		id         int64
		definition workflow.Definition
	}
	definitions := []storedDefinition{}
	for rows.Next() {
		var item storedDefinition
		var payload string
		if err = rows.Scan(&item.id, &payload); err != nil {
			rows.Close()
			return err
		}
		if err = json.Unmarshal([]byte(payload), &item.definition); err != nil {
			rows.Close()
			return fmt.Errorf("migrate workflow version %d fixed PR coordinates: decode definition: %w", item.id, err)
		}
		definitions = append(definitions, item)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err = rows.Close(); err != nil {
		return err
	}

	for _, item := range definitions {
		changed, migrateErr := migrateDefinitionPullRequestCoordinates(&item.definition)
		if migrateErr != nil {
			return fmt.Errorf("migrate workflow version %d fixed PR coordinates: %w", item.id, migrateErr)
		}
		if !changed {
			continue
		}
		payload, marshalErr := json.Marshal(item.definition)
		if marshalErr != nil {
			return fmt.Errorf("migrate workflow version %d fixed PR coordinates: %w", item.id, marshalErr)
		}
		if _, err = tx.ExecContext(ctx, `UPDATE workflow_versions SET definition_json=? WHERE id=?`, string(payload), item.id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func migrateDefinitionPullRequestCoordinates(definition *workflow.Definition) (bool, error) {
	fetches := []workflow.Node{}
	usedEdgeKeys := map[string]bool{}
	for _, node := range definition.Nodes {
		if node.Type == "fetch" {
			fetches = append(fetches, node)
		}
	}
	for _, edge := range definition.Edges {
		usedEdgeKeys[edge.Key] = true
	}
	changed := false
	for _, publish := range definition.Nodes {
		if publish.Type != "publish" || hasPullRequestInputEdge(definition.Edges, publish.Key) {
			continue
		}
		fetch, err := migrationFetchForPublish(fetches, publish)
		if err != nil {
			return false, err
		}
		base := fetch.Key + "-" + publish.Key + "-pull-request"
		key := base
		for suffix := 2; usedEdgeKeys[key]; suffix++ {
			key = fmt.Sprintf("%s-%d", base, suffix)
		}
		usedEdgeKeys[key] = true
		definition.Edges = append(definition.Edges, workflow.Edge{Key: key, FromNode: fetch.Key, FromPort: "pull_request", ToNode: publish.Key, ToPort: "pull_request"})
		changed = true
	}
	for index := range definition.Nodes {
		node := &definition.Nodes[index]
		if node.Type != "fetch" && node.Type != "publish" {
			continue
		}
		for _, key := range []string{"owner", "repo", "pull_request"} {
			if _, exists := node.Config[key]; exists {
				delete(node.Config, key)
				changed = true
			}
		}
	}
	return changed, nil
}

func hasPullRequestInputEdge(edges []workflow.Edge, publishKey string) bool {
	for _, edge := range edges {
		if edge.ToNode == publishKey && edge.ToPort == "pull_request" {
			return true
		}
	}
	return false
}

func migrationFetchForPublish(fetches []workflow.Node, publish workflow.Node) (workflow.Node, error) {
	if len(fetches) == 1 {
		return fetches[0], nil
	}
	wanted, wantedOK := configuredReviewContext(publish.Config)
	matches := []workflow.Node{}
	if wantedOK {
		for _, fetch := range fetches {
			candidate, ok := configuredReviewContext(fetch.Config)
			if ok && *candidate == *wanted {
				matches = append(matches, fetch)
			}
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	return workflow.Node{}, fmt.Errorf("publish card %q has no pull_request edge and its source is ambiguous across %d fetch cards", publish.Key, len(fetches))
}
