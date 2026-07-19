package review

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gitea-agents/internal/queue"
)

// StageContract is a versioned response contract controlled by the application.
type StageContract struct {
	ID                   int64  `json:"id"`
	Key                  string `json:"key"`
	Version              int    `json:"version"`
	ResponseInstruction  string `json:"response_instruction"`
	SchemaJSON           string `json:"schema_json"`
	SemanticValidatorKey string `json:"semantic_validator_key"`
}

// PipelineStage is one configured instance of a registered executor.
type PipelineStage struct {
	ID              int64          `json:"id"`
	StageTypeID     int64          `json:"stage_type_id"`
	StageTypeKey    string         `json:"stage_type_key"`
	ExecutorKey     string         `json:"executor_key"`
	Key             string         `json:"key"`
	Name            string         `json:"name"`
	Position        int            `json:"position"`
	PromptTemplate  string         `json:"prompt_template,omitempty"`
	ModelID         *int64         `json:"model_id,omitempty"`
	MaxOutputTokens int            `json:"max_output_tokens"`
	RetryLimit      int            `json:"retry_limit"`
	TimeoutSeconds  int            `json:"timeout_seconds"`
	UseLLM          bool           `json:"use_llm"`
	Required        bool           `json:"required"`
	Config          map[string]any `json:"config,omitempty"`
	InputContract   *StageContract `json:"input_contract,omitempty"`
	OutputContract  *StageContract `json:"output_contract,omitempty"`
}

// PipelineTransition links two configured stages for a known outcome.
type PipelineTransition struct {
	FromStageID  int64  `json:"from_stage_id"`
	ToStageID    *int64 `json:"to_stage_id,omitempty"`
	Type         string `json:"type"`
	ConditionKey string `json:"condition_key"`
	Priority     int    `json:"priority"`
}

// PipelineDefinition is the immutable published version used by one execution.
type PipelineDefinition struct {
	ID          int64                `json:"id"`
	Key         string               `json:"key"`
	Name        string               `json:"name"`
	ProfileID   *int64               `json:"profile_id,omitempty"`
	VersionID   int64                `json:"version_id"`
	Version     int                  `json:"version"`
	Stages      []PipelineStage      `json:"stages"`
	Transitions []PipelineTransition `json:"transitions"`
}

// Pipeline loads and binds the immutable pipeline version selected for a review.
func (r *Repository) Pipeline(ctx context.Context, reviewID string, job queue.ReviewJob) (PipelineDefinition, error) {
	var versionID sql.NullInt64
	var selectedProfile sql.NullInt64
	err := r.db.QueryRowContext(ctx, `SELECT pipeline_version_id,pipeline_profile_id FROM reviews WHERE id=?`, reviewID).Scan(&versionID, &selectedProfile)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return PipelineDefinition{}, err
	}
	if !versionID.Valid {
		profileID, profileErr := r.profileID(ctx, job)
		if profileErr != nil && !errors.Is(profileErr, sql.ErrNoRows) {
			return PipelineDefinition{}, profileErr
		}
		if profileID != nil {
			selectedProfile = sql.NullInt64{Int64: *profileID, Valid: true}
		}
		versionID, err = r.activePipelineVersion(ctx, profileID)
		if err != nil {
			return PipelineDefinition{}, err
		}
		if reviewID != "" {
			if _, err = r.db.ExecContext(ctx, `UPDATE reviews SET pipeline_version_id=?,pipeline_profile_id=? WHERE id=? AND pipeline_version_id IS NULL`, versionID.Int64, profileID, reviewID); err != nil {
				return PipelineDefinition{}, err
			}
			var bound, boundProfile sql.NullInt64
			if err = r.db.QueryRowContext(ctx, `SELECT pipeline_version_id,pipeline_profile_id FROM reviews WHERE id=?`, reviewID).Scan(&bound, &boundProfile); err == nil && bound.Valid {
				versionID = bound
				selectedProfile = boundProfile
			}
		}
	}
	definition, err := r.loadPipelineVersion(ctx, versionID.Int64)
	if err != nil {
		return PipelineDefinition{}, err
	}
	if selectedProfile.Valid {
		definition.ProfileID = &selectedProfile.Int64
	}
	if err = validatePipelineDefinition(definition); err != nil {
		return PipelineDefinition{}, fmt.Errorf("pipeline %s v%d inválido: %w", definition.Key, definition.Version, err)
	}
	return definition, nil
}

func (r *Repository) profileID(ctx context.Context, job queue.ReviewJob) (*int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `SELECT rp.id
		FROM review_profiles rp
		LEFT JOIN repositories rep ON rep.review_profile_id=rp.id
		  AND rep.full_name=? AND rep.is_enabled=1
		WHERE rp.is_enabled=1 AND (rep.id IS NOT NULL OR rp.is_default=1)
		ORDER BY rep.id DESC, rp.is_default DESC LIMIT 1`, job.Owner+"/"+job.Repository).Scan(&id)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func (r *Repository) activePipelineVersion(ctx context.Context, profileID *int64) (sql.NullInt64, error) {
	var id sql.NullInt64
	if profileID != nil {
		err := r.db.QueryRowContext(ctx, `SELECT pv.id
			FROM pipeline_definitions pd JOIN pipeline_versions pv ON pv.pipeline_definition_id=pd.id
			WHERE pd.profile_id=? AND pd.is_default=1 AND pd.is_enabled=1 AND pv.status='published'
			LIMIT 1`, *profileID).Scan(&id)
		if err == nil {
			return id, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return id, err
		}
	}
	err := r.db.QueryRowContext(ctx, `SELECT pv.id
		FROM pipeline_definitions pd JOIN pipeline_versions pv ON pv.pipeline_definition_id=pd.id
		WHERE pd.profile_id IS NULL AND pd.is_default=1 AND pd.is_enabled=1 AND pv.status='published'
		LIMIT 1`).Scan(&id)
	return id, err
}

func (r *Repository) loadPipelineVersion(ctx context.Context, versionID int64) (PipelineDefinition, error) {
	var item PipelineDefinition
	var profileID sql.NullInt64
	err := r.db.QueryRowContext(ctx, `SELECT pd.id,pd.key,pd.name,pd.profile_id,pv.id,pv.version
		FROM pipeline_versions pv JOIN pipeline_definitions pd ON pd.id=pv.pipeline_definition_id
		WHERE pv.id=?`, versionID).Scan(
		&item.ID, &item.Key, &item.Name, &profileID, &item.VersionID, &item.Version)
	if err != nil {
		return item, err
	}
	if profileID.Valid {
		item.ProfileID = &profileID.Int64
	}

	rows, err := r.db.QueryContext(ctx, `SELECT ps.id,ps.stage_type_id,st.key,st.executor_key,
		ps.stage_key,ps.display_name,ps.position,ps.prompt_template,ps.model_id,
		ps.max_output_tokens,ps.retry_limit,ps.timeout_seconds,ps.use_llm,ps.is_required,ps.config_json,
		ic.id,ic.key,ic.version,ic.response_instruction,ic.schema_json,ic.semantic_validator_key,
		oc.id,oc.key,oc.version,oc.response_instruction,oc.schema_json,oc.semantic_validator_key
		FROM pipeline_stages ps
		JOIN stage_types st ON st.id=ps.stage_type_id AND st.is_enabled=1
		LEFT JOIN stage_contracts ic ON ic.id=st.input_contract_id
		LEFT JOIN stage_contracts oc ON oc.id=st.output_contract_id
		WHERE ps.pipeline_version_id=? AND ps.is_enabled=1 ORDER BY ps.position`, versionID)
	if err != nil {
		return item, err
	}
	defer rows.Close()
	for rows.Next() {
		var stage PipelineStage
		var modelID sql.NullInt64
		var useLLM, required int
		var config string
		var inputID sql.NullInt64
		var inputKey, inputInstruction, inputSchema, inputValidator sql.NullString
		var inputVersion sql.NullInt64
		var outputID sql.NullInt64
		var outputKey, outputInstruction, outputSchema, outputValidator sql.NullString
		var outputVersion sql.NullInt64
		if err = rows.Scan(&stage.ID, &stage.StageTypeID, &stage.StageTypeKey, &stage.ExecutorKey,
			&stage.Key, &stage.Name, &stage.Position, &stage.PromptTemplate, &modelID,
			&stage.MaxOutputTokens, &stage.RetryLimit, &stage.TimeoutSeconds, &useLLM, &required, &config,
			&inputID, &inputKey, &inputVersion, &inputInstruction, &inputSchema, &inputValidator,
			&outputID, &outputKey, &outputVersion, &outputInstruction, &outputSchema, &outputValidator); err != nil {
			return item, err
		}
		if modelID.Valid {
			stage.ModelID = &modelID.Int64
		}
		stage.UseLLM = useLLM != 0
		stage.Required = required != 0
		_ = json.Unmarshal([]byte(config), &stage.Config)
		if inputID.Valid {
			stage.InputContract = &StageContract{ID: inputID.Int64, Key: inputKey.String, Version: int(inputVersion.Int64), ResponseInstruction: inputInstruction.String, SchemaJSON: inputSchema.String, SemanticValidatorKey: inputValidator.String}
		}
		if outputID.Valid {
			stage.OutputContract = &StageContract{ID: outputID.Int64, Key: outputKey.String, Version: int(outputVersion.Int64), ResponseInstruction: outputInstruction.String, SchemaJSON: outputSchema.String, SemanticValidatorKey: outputValidator.String}
		}
		item.Stages = append(item.Stages, stage)
	}
	if err = rows.Err(); err != nil {
		return item, err
	}

	transitionRows, err := r.db.QueryContext(ctx, `SELECT from_stage_id,to_stage_id,transition_type,condition_key,priority
		FROM pipeline_transitions WHERE pipeline_version_id=? ORDER BY priority,id`, versionID)
	if err != nil {
		return item, err
	}
	defer transitionRows.Close()
	for transitionRows.Next() {
		var transition PipelineTransition
		var to sql.NullInt64
		if err = transitionRows.Scan(&transition.FromStageID, &to, &transition.Type, &transition.ConditionKey, &transition.Priority); err != nil {
			return item, err
		}
		if to.Valid {
			transition.ToStageID = &to.Int64
		}
		item.Transitions = append(item.Transitions, transition)
	}
	return item, transitionRows.Err()
}

func validatePipelineDefinition(definition PipelineDefinition) error {
	if len(definition.Stages) == 0 {
		return errors.New("não possui etapas")
	}
	known := map[string]bool{"preparation": true, "planner": true, "reviewer": true, "consolidator": true, "verification": true, "formatting": true, "publication": true}
	counts := map[string]int{}
	positions := map[string]int{}
	expectedOutputs := map[string]string{
		"preparation": "prepared_diff", "planner": "review_plan", "reviewer": "review_findings",
		"consolidator": "consolidated_findings", "verification": "verified_findings",
		"formatting": "formatted_review", "publication": "publication_result",
	}
	lastPosition := 0
	for _, stage := range definition.Stages {
		if !known[stage.ExecutorKey] {
			return fmt.Errorf("executor desconhecido %q", stage.ExecutorKey)
		}
		if stage.Position <= lastPosition {
			return errors.New("posições de etapas inválidas")
		}
		lastPosition = stage.Position
		counts[stage.ExecutorKey]++
		positions[stage.ExecutorKey] = stage.Position
		if stage.OutputContract == nil || stage.OutputContract.Key != expectedOutputs[stage.ExecutorKey] {
			return fmt.Errorf("contrato de saída incompatível na etapa %q", stage.Key)
		}
	}
	if definition.Stages[0].ExecutorKey != "preparation" {
		return errors.New("preparação deve ser a primeira etapa")
	}
	if definition.Stages[len(definition.Stages)-1].ExecutorKey != "publication" {
		return errors.New("publicação deve ser a última etapa")
	}
	for _, required := range []string{"preparation", "verification", "formatting", "publication"} {
		if counts[required] != 1 {
			return fmt.Errorf("executor obrigatório %q deve aparecer exatamente uma vez", required)
		}
	}
	if positions["verification"] >= positions["formatting"] || positions["formatting"] >= positions["publication"] {
		return errors.New("verificação, formatação e publicação estão fora de ordem")
	}
	for _, executor := range []string{"planner", "reviewer", "consolidator"} {
		if position := positions[executor]; position > 0 && position >= positions["verification"] {
			return fmt.Errorf("executor %q deve ocorrer antes da verificação", executor)
		}
	}
	return nil
}

// PipelineExecutionSnapshot records configuration that must not change during a run.
type PipelineExecutionSnapshot struct {
	Definition PipelineDefinition `json:"definition"`
	BasePrompt string             `json:"base_prompt"`
	Policy     Policy             `json:"policy"`
}

// BeginPipelineExecution stores the immutable configuration snapshot used by a run.
func (r *Repository) BeginPipelineExecution(ctx context.Context, reviewID string, snapshotValue PipelineExecutionSnapshot) (int64, error) {
	snapshot, err := json.Marshal(snapshotValue)
	if err != nil {
		return 0, err
	}
	result, err := r.db.ExecContext(ctx, `INSERT INTO pipeline_executions(review_id,pipeline_version_id,status,definition_snapshot_json)
		VALUES(?,?,'running',?)`, reviewID, snapshotValue.Definition.VersionID, snapshot)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// FinishPipelineExecution records the terminal execution state.
func (r *Repository) FinishPipelineExecution(ctx context.Context, executionID int64, status string, executionErr error) error {
	errorText := ""
	if executionErr != nil {
		errorText = executionErr.Error()
	}
	_, err := r.db.ExecContext(ctx, `UPDATE pipeline_executions SET status=?,finished_at=CURRENT_TIMESTAMP,error_message=? WHERE id=?`, status, errorText, executionID)
	return err
}

func (r *Repository) beginStageExecution(ctx context.Context, executionID int64, stage PipelineStage) (int64, time.Time, error) {
	started := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, `INSERT INTO stage_executions(
		pipeline_execution_id,pipeline_stage_id,stage_key,attempt,status,started_at)
		VALUES(?,?,?,0,'running',?)`, executionID, stage.ID, stage.Key, started)
	if err != nil {
		return 0, started, err
	}
	id, err := result.LastInsertId()
	return id, started, err
}

func (r *Repository) recordStageAttempt(ctx context.Context, executionID int64, stage PipelineStage, attempt int, started time.Time, metadata any, attemptErr error) error {
	status := "completed"
	errorText := ""
	if attemptErr != nil {
		status = "failed"
		errorText = attemptErr.Error()
	}
	metadataJSON, _ := json.Marshal(metadata)
	finished := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, `INSERT INTO stage_executions(
		pipeline_execution_id,pipeline_stage_id,stage_key,attempt,status,metadata_json,started_at,finished_at,duration_ms,error_message)
		VALUES(?,?,?,?,?,?,?,?,?,?)`, executionID, stage.ID, stage.Key, attempt, status, metadataJSON,
		started, finished, finished.Sub(started).Milliseconds(), errorText)
	return err
}

func (r *Repository) finishStageExecution(ctx context.Context, executionID, stageExecutionID int64, started time.Time, status, artifactType string, artifact, metadata any, stageErr error) error {
	metadataJSON, _ := json.Marshal(metadata)
	errorText := ""
	if stageErr != nil {
		errorText = stageErr.Error()
	}
	finished := time.Now().UTC()
	if _, err := r.db.ExecContext(ctx, `UPDATE stage_executions SET status=?,artifact_type=?,metadata_json=?,
		finished_at=?,duration_ms=?,error_message=? WHERE id=?`, status, artifactType, metadataJSON,
		finished, finished.Sub(started).Milliseconds(), errorText, stageExecutionID); err != nil {
		return err
	}
	if artifactType == "" || artifact == nil {
		return nil
	}
	payload, err := json.Marshal(artifact)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO stage_artifacts(
		pipeline_execution_id,stage_execution_id,artifact_type,payload_json) VALUES(?,?,?,?)`,
		executionID, stageExecutionID, artifactType, payload)
	return err
}

// BeginPublication atomically reserves publication and blocks duplicate sends.
func (r *Repository) BeginPublication(ctx context.Context, reviewID string, result Result) (bool, error) {
	payload, err := json.Marshal(result)
	if err != nil {
		return false, err
	}
	sum := sha256.Sum256(payload)
	fingerprint := hex.EncodeToString(sum[:])
	insert, err := r.db.ExecContext(ctx, `INSERT INTO review_publications(review_id,status,result_fingerprint)
		SELECT ?,'publishing',? WHERE EXISTS(SELECT 1 FROM reviews WHERE id=? AND status!='cancelado')
		ON CONFLICT(review_id) DO NOTHING`, reviewID, fingerprint, reviewID)
	if err != nil {
		return false, err
	}
	if affected, _ := insert.RowsAffected(); affected == 1 {
		return true, nil
	}
	var reviewStatus string
	if err = r.db.QueryRowContext(ctx, `SELECT status FROM reviews WHERE id=?`, reviewID).Scan(&reviewStatus); err != nil {
		return false, err
	}
	if reviewStatus == StatusCancelled {
		return false, nil
	}
	var status string
	if err = r.db.QueryRowContext(ctx, `SELECT status FROM review_publications WHERE review_id=?`, reviewID).Scan(&status); err != nil {
		return false, err
	}
	if status != "failed" {
		return false, nil
	}
	update, err := r.db.ExecContext(ctx, `UPDATE review_publications SET status='publishing',result_fingerprint=?,error_message='',updated_at=CURRENT_TIMESTAMP
		WHERE review_id=? AND status='failed'`, fingerprint, reviewID)
	if err != nil {
		return false, err
	}
	affected, _ := update.RowsAffected()
	return affected == 1, nil
}

// FinishPublication records whether the reserved external publication succeeded.
func (r *Repository) FinishPublication(ctx context.Context, reviewID string, publishErr error) error {
	if publishErr != nil {
		_, err := r.db.ExecContext(ctx, `UPDATE review_publications SET status='failed',error_message=?,updated_at=CURRENT_TIMESTAMP WHERE review_id=?`, publishErr.Error(), reviewID)
		return err
	}
	_, err := r.db.ExecContext(ctx, `UPDATE review_publications SET status='published',published_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE review_id=?`, reviewID)
	return err
}

// MarkPublicationUncertain preserves the reservation after a transport error;
// retrying automatically could duplicate a review already accepted by Gitea.
func (r *Repository) MarkPublicationUncertain(ctx context.Context, reviewID string, publishErr error) error {
	errorText := ""
	if publishErr != nil {
		errorText = publishErr.Error()
	}
	_, err := r.db.ExecContext(ctx, `UPDATE review_publications SET status='uncertain',error_message=?,updated_at=CURRENT_TIMESTAMP WHERE review_id=?`, errorText, reviewID)
	return err
}

// ResetPublicationForRetry reopens a reservation only after Gitea
// reconciliation has confirmed that the marked review is absent.
func (r *Repository) ResetPublicationForRetry(ctx context.Context, reviewID string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE review_publications SET status='failed',error_message='',updated_at=CURRENT_TIMESTAMP
		WHERE review_id=? AND status IN ('publishing','uncertain')`, reviewID)
	return err
}

// PublicationStatus returns the current idempotency reservation state.
func (r *Repository) PublicationStatus(ctx context.Context, reviewID string) (string, error) {
	var status string
	err := r.db.QueryRowContext(ctx, `SELECT status FROM review_publications WHERE review_id=?`, reviewID).Scan(&status)
	return status, err
}

// PublicationReconciliationAllowed prevents a second worker from reopening a
// publication that may still be actively sending. Both abandoned and
// uncertain reservations wait 15 minutes before remote reconciliation.
func (r *Repository) PublicationReconciliationAllowed(ctx context.Context, reviewID string) (bool, error) {
	var allowed int
	err := r.db.QueryRowContext(ctx, `SELECT CASE
		WHEN status IN ('publishing','uncertain') AND updated_at<=datetime('now','-15 minutes') THEN 1
		ELSE 0 END FROM review_publications WHERE review_id=?`, reviewID).Scan(&allowed)
	return allowed != 0, err
}
