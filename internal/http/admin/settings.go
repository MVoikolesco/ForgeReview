package admin

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"gitea-agents/internal/http/responses"

	"github.com/gin-gonic/gin"
)

type reviewSettingsResponse struct {
	Profiles     []settingsProfile   `json:"profiles"`
	Pipelines    []settingsPipeline  `json:"pipelines"`
	StageCatalog []settingsStageType `json:"stage_catalog"`
}

type settingsProfile struct {
	ID          int64           `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	IsDefault   bool            `json:"is_default"`
	IsEnabled   bool            `json:"is_enabled"`
	CreatedAt   string          `json:"created_at"`
	UpdatedAt   string          `json:"updated_at"`
	Model       *settingsModel  `json:"model,omitempty"`
	Policy      *settingsPolicy `json:"policy,omitempty"`
	Prompt      *settingsPrompt `json:"prompt,omitempty"`
}

type settingsModel struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Provider   string `json:"provider"`
	Connection string `json:"connection"`
	IsReady    bool   `json:"is_ready"`
}

type settingsPolicy struct {
	ID                      int64   `json:"id"`
	MaxBlockChars           int     `json:"max_block_chars"`
	MaxFilesPerBlock        int     `json:"max_files_per_block"`
	PublishManualReviews    bool    `json:"publish_manual_reviews"`
	AllowAutonomousReject   bool    `json:"allow_autonomous_rejection"`
	EnableDetailedStageLogs bool    `json:"enable_detailed_stage_logs"`
	ContextSafetyTokens     int     `json:"context_safety_tokens"`
	MinimumConfidence       float64 `json:"minimum_confidence"`
	MaxParallelGroups       int     `json:"max_parallel_groups"`
	MediumSeverityEvent     string  `json:"medium_severity_event"`
	PartialEvent            string  `json:"partial_event"`
}

type settingsPrompt struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Stack     string `json:"stack"`
	Content   string `json:"content"`
	Version   int    `json:"version"`
	IsActive  bool   `json:"is_active"`
	UpdatedAt string `json:"updated_at"`
}

type settingsPipeline struct {
	ID          int64           `json:"id"`
	ProfileID   *int64          `json:"profile_id,omitempty"`
	ProfileName string          `json:"profile_name,omitempty"`
	Key         string          `json:"key"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	IsDefault   bool            `json:"is_default"`
	IsEnabled   bool            `json:"is_enabled"`
	VersionID   int64           `json:"version_id"`
	Version     int             `json:"version"`
	Status      string          `json:"status"`
	PublishedAt string          `json:"published_at"`
	Stages      []settingsStage `json:"stages"`
}

type settingsStage struct {
	ID              int64  `json:"id"`
	Key             string `json:"key"`
	Name            string `json:"name"`
	Position        int    `json:"position"`
	StageTypeKey    string `json:"stage_type_key"`
	ExecutorKey     string `json:"executor_key"`
	PromptTemplate  string `json:"prompt_template"`
	ModelID         *int64 `json:"model_id,omitempty"`
	ModelName       string `json:"model_name,omitempty"`
	IsModelReady    bool   `json:"is_model_ready"`
	MaxOutputTokens int    `json:"max_output_tokens"`
	RetryLimit      int    `json:"retry_limit"`
	TimeoutSeconds  int    `json:"timeout_seconds"`
	UseLLM          bool   `json:"use_llm"`
	IsRequired      bool   `json:"is_required"`
	IsTypeEnabled   bool   `json:"is_type_enabled"`
	InputContract   string `json:"input_contract,omitempty"`
	OutputContract  string `json:"output_contract,omitempty"`
}

type settingsStageType struct {
	ID             int64             `json:"id"`
	Key            string            `json:"key"`
	Name           string            `json:"name"`
	ExecutorKey    string            `json:"executor_key"`
	IsSystem       bool              `json:"is_system"`
	IsEnabled      bool              `json:"is_enabled"`
	InputContract  *settingsContract `json:"input_contract,omitempty"`
	OutputContract *settingsContract `json:"output_contract,omitempty"`
}

type settingsContract struct {
	ID                   int64           `json:"id"`
	Key                  string          `json:"key"`
	Version              int             `json:"version"`
	ResponseInstruction  string          `json:"response_instruction"`
	Schema               json.RawMessage `json:"schema"`
	SemanticValidatorKey string          `json:"semantic_validator_key"`
}

func (h *AdminHandler) reviewSettings(c *gin.Context) {
	profiles, err := h.settingsProfiles(c)
	if err != nil {
		responses.LegacyError(c, http.StatusInternalServerError, "could not load review profiles")
		return
	}
	pipelines, err := h.settingsPipelines(c)
	if err != nil {
		responses.LegacyError(c, http.StatusInternalServerError, "could not load review pipelines")
		return
	}
	catalog, err := h.settingsStageCatalog(c)
	if err != nil {
		responses.LegacyError(c, http.StatusInternalServerError, "could not load stage catalog")
		return
	}
	responses.Legacy(c, http.StatusOK, reviewSettingsResponse{Profiles: profiles, Pipelines: pipelines, StageCatalog: catalog})
}

func (h *AdminHandler) settingsProfiles(c *gin.Context) ([]settingsProfile, error) {
	rows, err := h.db.QueryContext(c, `SELECT rp.id,rp.name,rp.description,rp.is_default,rp.is_enabled,rp.created_at,rp.updated_at,
		m.id,m.display_name,p.display_name,ac.name,m.is_enabled,ac.is_enabled,p.is_enabled,
		pol.id,pol.max_block_chars,pol.max_files_per_block,pol.publish_manual_reviews,
		pol.allow_autonomous_rejection,pol.enable_detailed_stage_logs,pol.review_context_safety_margin_tokens,
		pol.review_min_publish_confidence,pol.review_max_parallel_groups,
		pol.review_medium_severity_event,pol.review_partial_event
		FROM review_profiles rp
		LEFT JOIN ai_models m ON m.id=rp.model_id
		LEFT JOIN ai_connections ac ON ac.id=m.connection_id
		LEFT JOIN ai_providers p ON p.id=ac.provider_id
		LEFT JOIN review_policies pol ON pol.profile_id=rp.id
		ORDER BY rp.is_default DESC,rp.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []settingsProfile{}
	for rows.Next() {
		var item settingsProfile
		var isDefault, isEnabled int
		var modelID sql.NullInt64
		var modelName, providerName, connectionName sql.NullString
		var modelEnabled, connectionEnabled, providerEnabled sql.NullInt64
		var policyID sql.NullInt64
		var maxBlockChars, maxFiles, publish, reject, detailedLogs, safety, parallel sql.NullInt64
		var confidence sql.NullFloat64
		var mediumEvent, partialEvent sql.NullString
		if err = rows.Scan(&item.ID, &item.Name, &item.Description, &isDefault, &isEnabled, &item.CreatedAt, &item.UpdatedAt,
			&modelID, &modelName, &providerName, &connectionName, &modelEnabled, &connectionEnabled, &providerEnabled,
			&policyID, &maxBlockChars, &maxFiles, &publish, &reject, &detailedLogs, &safety, &confidence, &parallel, &mediumEvent, &partialEvent); err != nil {
			return nil, err
		}
		item.IsDefault = isDefault != 0
		item.IsEnabled = isEnabled != 0
		if modelID.Valid {
			item.Model = &settingsModel{ID: modelID.Int64, Name: modelName.String, Provider: providerName.String,
				Connection: connectionName.String, IsReady: modelEnabled.Int64 != 0 && connectionEnabled.Int64 != 0 && providerEnabled.Int64 != 0}
		}
		if policyID.Valid {
			item.Policy = &settingsPolicy{ID: policyID.Int64, MaxBlockChars: int(maxBlockChars.Int64), MaxFilesPerBlock: int(maxFiles.Int64),
				PublishManualReviews: publish.Int64 != 0, AllowAutonomousReject: reject.Int64 != 0,
				EnableDetailedStageLogs: detailedLogs.Int64 != 0,
				ContextSafetyTokens:     int(safety.Int64), MinimumConfidence: confidence.Float64,
				MaxParallelGroups: int(parallel.Int64), MediumSeverityEvent: mediumEvent.String, PartialEvent: partialEvent.String}
		}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}
	for index := range items {
		items[index].Prompt, err = h.settingsPrompt(c, items[index].ID)
		if err != nil {
			return nil, err
		}
	}
	return items, nil
}

func (h *AdminHandler) settingsPrompt(c *gin.Context, profileID int64) (*settingsPrompt, error) {
	var item settingsPrompt
	var active int
	err := h.db.QueryRowContext(c, `SELECT id,name,prompt_type,stack,content,version,is_active,updated_at
		FROM review_prompts WHERE profile_id=? AND is_active=1 ORDER BY version DESC,id DESC LIMIT 1`, profileID).Scan(
		&item.ID, &item.Name, &item.Type, &item.Stack, &item.Content, &item.Version, &active, &item.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	item.IsActive = active != 0
	return &item, nil
}

func (h *AdminHandler) settingsPipelines(c *gin.Context) ([]settingsPipeline, error) {
	rows, err := h.db.QueryContext(c, `SELECT pd.id,pd.profile_id,COALESCE(rp.name,''),pd.key,pd.name,pd.description,
		pd.is_default,pd.is_enabled,pv.id,pv.version,pv.status,COALESCE(pv.published_at,'')
		FROM pipeline_definitions pd
		JOIN pipeline_versions pv ON pv.pipeline_definition_id=pd.id AND pv.status='published'
		LEFT JOIN review_profiles rp ON rp.id=pd.profile_id
		ORDER BY pd.profile_id IS NULL DESC,pd.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []settingsPipeline{}
	byVersion := map[int64]int{}
	for rows.Next() {
		var item settingsPipeline
		var profileID sql.NullInt64
		var isDefault, isEnabled int
		if err = rows.Scan(&item.ID, &profileID, &item.ProfileName, &item.Key, &item.Name, &item.Description,
			&isDefault, &isEnabled, &item.VersionID, &item.Version, &item.Status, &item.PublishedAt); err != nil {
			return nil, err
		}
		if profileID.Valid {
			item.ProfileID = &profileID.Int64
		}
		item.IsDefault = isDefault != 0
		item.IsEnabled = isEnabled != 0
		item.Stages = []settingsStage{}
		items = append(items, item)
		byVersion[item.VersionID] = len(items) - 1
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}
	stageRows, err := h.db.QueryContext(c, `SELECT ps.pipeline_version_id,ps.id,ps.stage_key,ps.display_name,ps.position,
		st.key,st.executor_key,ps.prompt_template,ps.model_id,COALESCE(m.display_name,''),
		CASE WHEN m.id IS NULL THEN 1 WHEN m.is_enabled=1 AND ac.is_enabled=1 AND ap.is_enabled=1 THEN 1 ELSE 0 END,
		ps.max_output_tokens,ps.retry_limit,ps.timeout_seconds,ps.use_llm,ps.is_required,st.is_enabled,
		COALESCE(ic.key,''),COALESCE(oc.key,'')
		FROM pipeline_stages ps
		JOIN stage_types st ON st.id=ps.stage_type_id
		LEFT JOIN ai_models m ON m.id=ps.model_id
		LEFT JOIN ai_connections ac ON ac.id=m.connection_id
		LEFT JOIN ai_providers ap ON ap.id=ac.provider_id
		LEFT JOIN stage_contracts ic ON ic.id=st.input_contract_id
		LEFT JOIN stage_contracts oc ON oc.id=st.output_contract_id
		WHERE ps.pipeline_version_id IN (SELECT id FROM pipeline_versions WHERE status='published')
		  AND ps.is_enabled=1 ORDER BY ps.pipeline_version_id,ps.position`)
	if err != nil {
		return nil, err
	}
	defer stageRows.Close()
	for stageRows.Next() {
		var versionID int64
		var stage settingsStage
		var modelID sql.NullInt64
		var useLLM, required, typeEnabled, modelReady int
		if err = stageRows.Scan(&versionID, &stage.ID, &stage.Key, &stage.Name, &stage.Position,
			&stage.StageTypeKey, &stage.ExecutorKey, &stage.PromptTemplate, &modelID, &stage.ModelName, &modelReady,
			&stage.MaxOutputTokens, &stage.RetryLimit, &stage.TimeoutSeconds, &useLLM, &required, &typeEnabled,
			&stage.InputContract, &stage.OutputContract); err != nil {
			return nil, err
		}
		if modelID.Valid {
			stage.ModelID = &modelID.Int64
		}
		stage.UseLLM = useLLM != 0
		stage.IsRequired = required != 0
		stage.IsTypeEnabled = typeEnabled != 0
		stage.IsModelReady = modelReady != 0
		if index, ok := byVersion[versionID]; ok {
			items[index].Stages = append(items[index].Stages, stage)
		}
	}
	return items, stageRows.Err()
}

func (h *AdminHandler) settingsStageCatalog(c *gin.Context) ([]settingsStageType, error) {
	rows, err := h.db.QueryContext(c, `SELECT st.id,st.key,st.display_name,st.executor_key,st.is_system,st.is_enabled,
		ic.id,ic.key,ic.version,ic.response_instruction,ic.schema_json,ic.semantic_validator_key,
		oc.id,oc.key,oc.version,oc.response_instruction,oc.schema_json,oc.semantic_validator_key
		FROM stage_types st
		LEFT JOIN stage_contracts ic ON ic.id=st.input_contract_id
		LEFT JOIN stage_contracts oc ON oc.id=st.output_contract_id
		ORDER BY st.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []settingsStageType{}
	for rows.Next() {
		var item settingsStageType
		var isSystem, isEnabled int
		var inputID, outputID sql.NullInt64
		var inputKey, inputInstruction, inputSchema, inputValidator sql.NullString
		var inputVersion sql.NullInt64
		var outputKey, outputInstruction, outputSchema, outputValidator sql.NullString
		var outputVersion sql.NullInt64
		if err = rows.Scan(&item.ID, &item.Key, &item.Name, &item.ExecutorKey, &isSystem, &isEnabled,
			&inputID, &inputKey, &inputVersion, &inputInstruction, &inputSchema, &inputValidator,
			&outputID, &outputKey, &outputVersion, &outputInstruction, &outputSchema, &outputValidator); err != nil {
			return nil, err
		}
		item.IsSystem = isSystem != 0
		item.IsEnabled = isEnabled != 0
		if inputID.Valid {
			item.InputContract = settingsContractValue(inputID.Int64, inputKey.String, int(inputVersion.Int64), inputInstruction.String, inputSchema.String, inputValidator.String)
		}
		if outputID.Valid {
			item.OutputContract = settingsContractValue(outputID.Int64, outputKey.String, int(outputVersion.Int64), outputInstruction.String, outputSchema.String, outputValidator.String)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func settingsContractValue(id int64, key string, version int, instruction, schema, validator string) *settingsContract {
	if !json.Valid([]byte(schema)) {
		schema = "{}"
	}
	return &settingsContract{ID: id, Key: key, Version: version, ResponseInstruction: instruction,
		Schema: json.RawMessage(schema), SemanticValidatorKey: validator}
}
