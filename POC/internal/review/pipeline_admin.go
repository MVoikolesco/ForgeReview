package review

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// PipelineStageInput describes the editable portion of a draft stage. Stage
// types and their contracts are always resolved from the catalog, never posted
// by an administrator.
type PipelineStageInput struct {
	StageTypeKey string         `json:"stage_type_key"`
	Key          string         `json:"key"`
	Name         string         `json:"name"`
	Prompt       string         `json:"prompt_template"`
	ModelID      *int64         `json:"model_id"`
	MaxTokens    int            `json:"max_output_tokens"`
	RetryLimit   int            `json:"retry_limit"`
	Timeout      int            `json:"timeout_seconds"`
	UseLLM       bool           `json:"use_llm"`
	Required     bool           `json:"required"`
	RouteMode    string         `json:"route_mode"`
	JoinMode     string         `json:"join_mode"`
	Config       map[string]any `json:"config"`
}

type PipelineDraftInput struct {
	Name             string                     `json:"name"`
	Description      string                     `json:"description"`
	ProfileID        *int64                     `json:"profile_id"`
	Stages           []PipelineStageInput       `json:"stages"`
	Transitions      *[]PipelineTransitionInput `json:"transitions,omitempty"`
	Triggers         []PipelineTrigger          `json:"triggers"`
	SchedulerMaxRuns int                        `json:"scheduler_max_runs"`
}

// PipelineTransitionInput uses stable draft stage keys rather than database IDs.
type PipelineTransitionInput struct {
	FromStageKey  string `json:"from_stage_key"`
	ToStageKey    string `json:"to_stage_key,omitempty"`
	Type          string `json:"type"`
	ConditionKey  string `json:"condition_key"`
	Priority      int    `json:"priority"`
	Rule          *Rule  `json:"rule,omitempty"`
	MaxTraversals int    `json:"max_traversals"`
}

type PipelineCreateInput struct {
	Key       string `json:"key"`
	IsDefault bool   `json:"is_default"`
	PipelineDraftInput
}

// PipelineVersionInfo is the administrative representation of a version.
type PipelineVersionInfo struct {
	ID               int64                `json:"id"`
	Version          int                  `json:"version"`
	Status           string               `json:"status"`
	ProfileID        *int64               `json:"profile_id,omitempty"`
	CreatedAt        string               `json:"created_at"`
	PublishedAt      string               `json:"published_at,omitempty"`
	SchedulerMaxRuns int                  `json:"scheduler_max_runs"`
	Stages           []PipelineStage      `json:"stages"`
	Transitions      []PipelineTransition `json:"transitions"`
	Triggers         []PipelineTrigger    `json:"triggers"`
}

type PipelineInfo struct {
	ID          int64                 `json:"id"`
	Key         string                `json:"key"`
	Name        string                `json:"name"`
	Description string                `json:"description"`
	ProfileID   *int64                `json:"profile_id,omitempty"`
	IsDefault   bool                  `json:"is_default"`
	IsEnabled   bool                  `json:"is_enabled"`
	Versions    []PipelineVersionInfo `json:"versions"`
}

func (r *Repository) CreatePipelineDraft(ctx context.Context, input PipelineCreateInput) (PipelineInfo, error) {
	if strings.TrimSpace(input.Key) == "" || strings.TrimSpace(input.Name) == "" {
		return PipelineInfo{}, errors.New("key and name are required")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return PipelineInfo{}, err
	}
	defer tx.Rollback()
	if input.IsDefault {
		if err = clearDefault(ctx, tx, input.ProfileID); err != nil {
			return PipelineInfo{}, err
		}
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO pipeline_definitions(profile_id,key,name,description,is_default,is_enabled) VALUES(?,?,?,?,?,1)`, input.ProfileID, input.Key, input.Name, input.Description, boolDB(input.IsDefault))
	if err != nil {
		return PipelineInfo{}, err
	}
	id, _ := result.LastInsertId()
	version, err := createVersion(ctx, tx, id, 1, input.ProfileID, input.Stages, input.Transitions, input.Triggers, input.SchedulerMaxRuns)
	if err != nil {
		return PipelineInfo{}, err
	}
	if err = tx.Commit(); err != nil {
		return PipelineInfo{}, err
	}
	return r.AdminPipeline(ctx, id, &version)
}

func (r *Repository) UpdatePipelineDraft(ctx context.Context, pipelineID, versionID int64, input PipelineDraftInput) (PipelineInfo, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return PipelineInfo{}, err
	}
	defer tx.Rollback()
	if err = ensureDraft(ctx, tx, pipelineID, versionID); err != nil {
		return PipelineInfo{}, err
	}
	if strings.TrimSpace(input.Name) == "" {
		return PipelineInfo{}, errors.New("name is required")
	}
	var published int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM pipeline_versions WHERE pipeline_definition_id=? AND status='published'`, pipelineID).Scan(&published); err != nil {
		return PipelineInfo{}, err
	}
	if published > 0 {
		if _, err = tx.ExecContext(ctx, `UPDATE pipeline_definitions SET name=?,description=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, input.Name, input.Description, pipelineID); err != nil {
			return PipelineInfo{}, err
		}
	} else if _, err = tx.ExecContext(ctx, `UPDATE pipeline_definitions SET name=?,description=?,profile_id=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, input.Name, input.Description, input.ProfileID, pipelineID); err != nil {
		return PipelineInfo{}, err
	}
	if err = replaceStages(ctx, tx, versionID, input.Stages, input.Transitions); err != nil {
		return PipelineInfo{}, err
	}
	if err = replaceTriggers(ctx, tx, versionID, input.Triggers); err != nil {
		return PipelineInfo{}, err
	}
	bound := input.SchedulerMaxRuns
	if bound < 1 {
		bound = 256
	}
	if _, err = tx.ExecContext(ctx, `UPDATE pipeline_versions SET target_profile_id=?,scheduler_max_runs=? WHERE id=?`, input.ProfileID, bound, versionID); err != nil {
		return PipelineInfo{}, err
	}
	if err = tx.Commit(); err != nil {
		return PipelineInfo{}, err
	}
	return r.AdminPipeline(ctx, pipelineID, &versionID)
}

func (r *Repository) PublishPipelineDraft(ctx context.Context, pipelineID, versionID int64) (PipelineInfo, error) {
	definition, err := r.loadPipelineVersion(ctx, versionID)
	if err != nil {
		return PipelineInfo{}, err
	}
	if definition.ID != pipelineID {
		return PipelineInfo{}, ErrNotFound
	}
	if err = validatePipelineDefinition(definition); err != nil {
		return PipelineInfo{}, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return PipelineInfo{}, err
	}
	defer tx.Rollback()
	if err = ensureDraft(ctx, tx, pipelineID, versionID); err != nil {
		return PipelineInfo{}, err
	}
	var targetProfileID sql.NullInt64
	if err = tx.QueryRowContext(ctx, `SELECT target_profile_id FROM pipeline_versions WHERE id=?`, versionID).Scan(&targetProfileID); err != nil {
		return PipelineInfo{}, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE pipeline_definitions SET profile_id=?,is_default=CASE WHEN profile_id IS ? THEN is_default ELSE 0 END,updated_at=CURRENT_TIMESTAMP WHERE id=?`, targetProfileID, targetProfileID, pipelineID); err != nil {
		return PipelineInfo{}, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE pipeline_versions SET status='archived' WHERE pipeline_definition_id=? AND status='published'`, pipelineID); err != nil {
		return PipelineInfo{}, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE pipeline_versions SET status='published',published_at=CURRENT_TIMESTAMP WHERE id=? AND status='draft'`, versionID)
	if err != nil {
		return PipelineInfo{}, err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return PipelineInfo{}, errors.New("draft was not published")
	}
	if err = tx.Commit(); err != nil {
		return PipelineInfo{}, err
	}
	return r.AdminPipeline(ctx, pipelineID, &versionID)
}

func (r *Repository) ClonePublishedPipeline(ctx context.Context, pipelineID, sourceVersionID int64) (PipelineInfo, error) {
	if sourceVersionID == 0 {
		if err := r.db.QueryRowContext(ctx, `SELECT id FROM pipeline_versions WHERE pipeline_definition_id=? AND status='published'`, pipelineID).Scan(&sourceVersionID); err != nil {
			return PipelineInfo{}, err
		}
	}
	source, err := r.loadPipelineVersion(ctx, sourceVersionID)
	if err != nil {
		return PipelineInfo{}, err
	}
	if source.ID != pipelineID {
		return PipelineInfo{}, ErrNotFound
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return PipelineInfo{}, err
	}
	defer tx.Rollback()
	var status string
	if err = tx.QueryRowContext(ctx, `SELECT status FROM pipeline_versions WHERE id=? AND pipeline_definition_id=?`, sourceVersionID, pipelineID).Scan(&status); err != nil {
		return PipelineInfo{}, err
	}
	if status != "published" && status != "archived" {
		return PipelineInfo{}, errors.New("source version must be published or archived")
	}
	var existing int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM pipeline_versions WHERE pipeline_definition_id=? AND status='draft'`, pipelineID).Scan(&existing); err != nil {
		return PipelineInfo{}, err
	}
	if existing > 0 {
		return PipelineInfo{}, errors.New("pipeline already has a draft")
	}
	var next int
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0)+1 FROM pipeline_versions WHERE pipeline_definition_id=?`, pipelineID).Scan(&next); err != nil {
		return PipelineInfo{}, err
	}
	inputs := make([]PipelineStageInput, len(source.Stages))
	for i, s := range source.Stages {
		inputs[i] = PipelineStageInput{StageTypeKey: s.StageTypeKey, Key: s.Key, Name: s.Name, Prompt: s.PromptTemplate, ModelID: s.ModelID, MaxTokens: s.MaxOutputTokens, RetryLimit: s.RetryLimit, Timeout: s.TimeoutSeconds, UseLLM: s.UseLLM, Required: s.Required, RouteMode: s.RouteMode, JoinMode: s.JoinMode, Config: s.Config}
	}
	transitions := transitionInputs(source)
	version, err := createVersion(ctx, tx, pipelineID, next, source.ProfileID, inputs, &transitions, source.Triggers, source.SchedulerMaxRuns)
	if err != nil {
		return PipelineInfo{}, err
	}
	if err = tx.Commit(); err != nil {
		return PipelineInfo{}, err
	}
	return r.AdminPipeline(ctx, pipelineID, &version)
}

func (r *Repository) ValidatePipelineDraft(ctx context.Context, pipelineID, versionID int64) error {
	d, err := r.loadPipelineVersion(ctx, versionID)
	if err != nil {
		return err
	}
	if d.ID != pipelineID {
		return ErrNotFound
	}
	var status string
	if err = r.db.QueryRowContext(ctx, `SELECT status FROM pipeline_versions WHERE id=?`, versionID).Scan(&status); err != nil {
		return err
	}
	if status != "draft" {
		return errors.New("only drafts can be validated")
	}
	return validatePipelineDefinition(d)
}

func (r *Repository) AdminPipeline(ctx context.Context, pipelineID int64, versionID *int64) (PipelineInfo, error) {
	var out PipelineInfo
	var profile sql.NullInt64
	var def, enabled int
	err := r.db.QueryRowContext(ctx, `SELECT id,key,name,description,profile_id,is_default,is_enabled FROM pipeline_definitions WHERE id=?`, pipelineID).Scan(&out.ID, &out.Key, &out.Name, &out.Description, &profile, &def, &enabled)
	if err != nil {
		return out, err
	}
	if profile.Valid {
		out.ProfileID = &profile.Int64
	}
	out.IsDefault = def != 0
	out.IsEnabled = enabled != 0
	q := `SELECT id,version,status,target_profile_id,created_at,COALESCE(published_at,''),scheduler_max_runs FROM pipeline_versions WHERE pipeline_definition_id=?`
	args := []any{pipelineID}
	if versionID != nil {
		q += ` AND id=?`
		args = append(args, *versionID)
	}
	q += ` ORDER BY version DESC`
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	versions := []PipelineVersionInfo{}
	for rows.Next() {
		var v PipelineVersionInfo
		var targetProfile sql.NullInt64
		if err = rows.Scan(&v.ID, &v.Version, &v.Status, &targetProfile, &v.CreatedAt, &v.PublishedAt, &v.SchedulerMaxRuns); err != nil {
			return out, err
		}
		if targetProfile.Valid {
			v.ProfileID = &targetProfile.Int64
		}
		versions = append(versions, v)
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	if err = rows.Close(); err != nil {
		return out, err
	}
	for _, v := range versions {
		d, e := r.loadPipelineVersion(ctx, v.ID)
		if e != nil {
			return out, e
		}
		v.Stages, v.Transitions, v.Triggers = d.Stages, d.Transitions, d.Triggers
		out.Versions = append(out.Versions, v)
	}
	return out, nil
}

func (r *Repository) AdminPipelines(ctx context.Context) ([]PipelineInfo, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id FROM pipeline_definitions ORDER BY profile_id IS NULL DESC,id`)
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
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}
	out := []PipelineInfo{}
	for _, id := range ids {
		p, e := r.AdminPipeline(ctx, id, nil)
		if e != nil {
			return nil, e
		}
		out = append(out, p)
	}
	return out, nil
}

// SelectedPipeline returns the published profile pipeline, falling back to the global default.
func (r *Repository) SelectedPipeline(ctx context.Context, profileID *int64) (PipelineInfo, error) {
	var id int64
	var err error
	if profileID != nil {
		err = r.db.QueryRowContext(ctx, `SELECT pd.id FROM pipeline_definitions pd JOIN pipeline_versions pv ON pv.pipeline_definition_id=pd.id WHERE pd.profile_id=? AND pd.is_default=1 AND pd.is_enabled=1 AND pv.status='published'`, *profileID).Scan(&id)
	}
	if profileID == nil || errors.Is(err, sql.ErrNoRows) {
		err = r.db.QueryRowContext(ctx, `SELECT pd.id FROM pipeline_definitions pd JOIN pipeline_versions pv ON pv.pipeline_definition_id=pd.id WHERE pd.profile_id IS NULL AND pd.is_default=1 AND pd.is_enabled=1 AND pv.status='published'`).Scan(&id)
	}
	if err != nil {
		return PipelineInfo{}, err
	}
	return r.AdminPipeline(ctx, id, nil)
}

// SelectPipeline makes a published pipeline the selection for its profile scope.
func (r *Repository) SelectPipeline(ctx context.Context, pipelineID int64) (PipelineInfo, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return PipelineInfo{}, err
	}
	defer tx.Rollback()
	var profileID sql.NullInt64
	var published int
	if err = tx.QueryRowContext(ctx, `SELECT profile_id,EXISTS(SELECT 1 FROM pipeline_versions WHERE pipeline_definition_id=pipeline_definitions.id AND status='published') FROM pipeline_definitions WHERE id=?`, pipelineID).Scan(&profileID, &published); err != nil {
		return PipelineInfo{}, err
	}
	if published == 0 {
		return PipelineInfo{}, errors.New("pipeline has no published version")
	}
	var profile *int64
	if profileID.Valid {
		profile = &profileID.Int64
	}
	if err = clearDefault(ctx, tx, profile); err != nil {
		return PipelineInfo{}, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE pipeline_definitions SET is_default=1,updated_at=CURRENT_TIMESTAMP WHERE id=?`, pipelineID); err != nil {
		return PipelineInfo{}, err
	}
	if err = tx.Commit(); err != nil {
		return PipelineInfo{}, err
	}
	return r.AdminPipeline(ctx, pipelineID, nil)
}

func createVersion(ctx context.Context, tx *sql.Tx, pipelineID int64, number int, profileID *int64, stages []PipelineStageInput, transitions *[]PipelineTransitionInput, triggers []PipelineTrigger, schedulerMaxRuns ...int) (int64, error) {
	bound := 256
	if len(schedulerMaxRuns) > 0 && schedulerMaxRuns[0] > 0 {
		bound = schedulerMaxRuns[0]
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO pipeline_versions(pipeline_definition_id,version,status,target_profile_id,scheduler_max_runs) VALUES(?,?,'draft',?,?)`, pipelineID, number, profileID, bound)
	if err != nil {
		return 0, err
	}
	id, _ := result.LastInsertId()
	if err = replaceStages(ctx, tx, id, stages, transitions); err != nil {
		return 0, err
	}
	if err = replaceTriggers(ctx, tx, id, triggers); err != nil {
		return 0, err
	}
	return id, nil
}
func replaceStages(ctx context.Context, tx *sql.Tx, versionID int64, stages []PipelineStageInput, transitions *[]PipelineTransitionInput) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM pipeline_stages WHERE pipeline_version_id=?`, versionID); err != nil {
		return err
	}
	ids := make([]int64, 0, len(stages))
	idsByKey := map[string]int64{}
	seen := map[string]bool{}
	for i, s := range stages {
		if strings.TrimSpace(s.Key) == "" || strings.TrimSpace(s.Name) == "" || seen[s.Key] {
			return errors.New("stage keys must be unique and non-empty")
		}
		seen[s.Key] = true
		if s.MaxTokens < 0 || s.RetryLimit < 0 || s.Timeout < 0 {
			return errors.New("stage limits cannot be negative")
		}
		if s.RouteMode == "" {
			s.RouteMode = "all_matches"
		}
		if s.JoinMode == "" {
			s.JoinMode = "each_arrival"
		}
		var typeID int64
		var inputContractID, outputContractID sql.NullInt64
		if err := tx.QueryRowContext(ctx, `SELECT id,input_contract_id,output_contract_id FROM stage_types WHERE key=? AND is_enabled=1`, s.StageTypeKey).Scan(&typeID, &inputContractID, &outputContractID); err != nil {
			return fmt.Errorf("unknown or disabled stage type %q: %w", s.StageTypeKey, err)
		}
		config, err := json.Marshal(s.Config)
		if err != nil {
			return errors.New("config must be JSON")
		}
		result, err := tx.ExecContext(ctx, `INSERT INTO pipeline_stages(pipeline_version_id,stage_type_id,stage_key,display_name,position,prompt_template,model_id,max_output_tokens,retry_limit,timeout_seconds,use_llm,is_required,is_enabled,config_json,route_mode,join_mode,input_contract_id,output_contract_id) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,1,?,?,?,?,?)`, versionID, typeID, s.Key, s.Name, i+1, s.Prompt, s.ModelID, s.MaxTokens, s.RetryLimit, s.Timeout, boolDB(s.UseLLM), boolDB(s.Required), string(config), s.RouteMode, s.JoinMode, inputContractID, outputContractID)
		if err != nil {
			return err
		}
		id, _ := result.LastInsertId()
		ids = append(ids, id)
		idsByKey[s.Key] = id
	}
	if transitions == nil {
		for i := 0; i+1 < len(ids); i++ {
			if _, err := tx.ExecContext(ctx, `INSERT INTO pipeline_transitions(pipeline_version_id,from_stage_id,to_stage_id,transition_type,condition_key,priority) VALUES(?,?,?,'success','always',0)`, versionID, ids[i], ids[i+1]); err != nil {
				return err
			}
		}
		return nil
	}
	for _, edge := range *transitions {
		from, ok := idsByKey[edge.FromStageKey]
		if !ok {
			return fmt.Errorf("unknown transition source %q", edge.FromStageKey)
		}
		var to any
		if edge.ToStageKey != "" {
			id, ok := idsByKey[edge.ToStageKey]
			if !ok {
				return fmt.Errorf("unknown transition destination %q", edge.ToStageKey)
			}
			to = id
		}
		var ruleJSON any
		if edge.Rule != nil {
			body, err := json.Marshal(edge.Rule)
			if err != nil {
				return err
			}
			ruleJSON = string(body)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO pipeline_transitions(pipeline_version_id,from_stage_id,to_stage_id,transition_type,condition_key,priority,rule_json,max_traversals) VALUES(?,?,?,?,?,?,?,?)`, versionID, from, to, edge.Type, edge.ConditionKey, edge.Priority, ruleJSON, edge.MaxTraversals); err != nil {
			return err
		}
	}
	return nil
}

func transitionInputs(definition PipelineDefinition) []PipelineTransitionInput {
	keys := map[int64]string{}
	for _, stage := range definition.Stages {
		keys[stage.ID] = stage.Key
	}
	out := make([]PipelineTransitionInput, 0, len(definition.Transitions))
	for _, edge := range definition.Transitions {
		item := PipelineTransitionInput{FromStageKey: keys[edge.FromStageID], Type: edge.Type, ConditionKey: edge.ConditionKey, Priority: edge.Priority, Rule: edge.Rule, MaxTraversals: edge.MaxTraversals}
		if edge.ToStageID != nil {
			item.ToStageKey = keys[*edge.ToStageID]
		}
		out = append(out, item)
	}
	return out
}
func replaceTriggers(ctx context.Context, tx *sql.Tx, versionID int64, triggers []PipelineTrigger) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM pipeline_version_triggers WHERE pipeline_version_id=?`, versionID); err != nil {
		return err
	}
	if len(triggers) == 0 {
		triggers = []PipelineTrigger{{Source: "webhook", Enabled: true}, {Source: "api", Enabled: true}, {Source: "manual", Enabled: true}}
	}
	var preparationKey string
	_ = tx.QueryRowContext(ctx, `SELECT ps.stage_key FROM pipeline_stages ps JOIN stage_types st ON st.id=ps.stage_type_id WHERE ps.pipeline_version_id=? AND st.executor_key='preparation' ORDER BY ps.position LIMIT 1`, versionID).Scan(&preparationKey)
	seen := map[string]bool{}
	for _, trigger := range triggers {
		if (trigger.Source != "webhook" && trigger.Source != "api" && trigger.Source != "manual") || seen[trigger.Source] {
			return errors.New("invalid pipeline trigger")
		}
		seen[trigger.Source] = true
		if trigger.TargetStageKey == "" && trigger.Enabled {
			trigger.TargetStageKey = preparationKey
		}
		if trigger.TargetStageKey != "" {
			var targetCount int
			if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM pipeline_stages WHERE pipeline_version_id=? AND stage_key=?`, versionID, trigger.TargetStageKey).Scan(&targetCount); err != nil || targetCount != 1 {
				return errors.New("invalid pipeline trigger target")
			}
		}
		config, err := json.Marshal(trigger.Config)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO pipeline_version_triggers(pipeline_version_id,trigger_source,is_enabled,target_stage_key,config_json) VALUES(?,?,?,?,?)`, versionID, trigger.Source, boolDB(trigger.Enabled), trigger.TargetStageKey, string(config)); err != nil {
			return err
		}
	}
	return nil
}
func ensureDraft(ctx context.Context, tx *sql.Tx, pipelineID, versionID int64) error {
	var status string
	err := tx.QueryRowContext(ctx, `SELECT status FROM pipeline_versions WHERE id=? AND pipeline_definition_id=?`, versionID, pipelineID).Scan(&status)
	if err != nil {
		return err
	}
	if status != "draft" {
		return errors.New("only draft versions can be changed")
	}
	return nil
}
func clearDefault(ctx context.Context, tx *sql.Tx, profileID *int64) error {
	if profileID == nil {
		_, err := tx.ExecContext(ctx, `UPDATE pipeline_definitions SET is_default=0 WHERE profile_id IS NULL`)
		return err
	}
	_, err := tx.ExecContext(ctx, `UPDATE pipeline_definitions SET is_default=0 WHERE profile_id=?`, *profileID)
	return err
}
func boolDB(value bool) int {
	if value {
		return 1
	}
	return 0
}
