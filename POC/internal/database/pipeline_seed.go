package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type contractSeed struct {
	key, instruction, schema, validator string
}

type stageTypeSeed struct {
	key, name, executor, input, output string
}

type pipelineSeedConfig struct {
	planner, consolidator, verifier, formatter bool
	plannerTokens, groupTokens                 int
	consolidatorTokens, verifierTokens         int
	formatterTokens                            int
	retries                                    int
}

type pipelineStageSeed struct {
	key, name, stageType, prompt string
	tokens, retries              int
	useLLM, required             bool
}

func (d *DB) seedPipelines(ctx context.Context) error {
	tx, err := d.SQL.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, item := range builtInContracts() {
		_, err = tx.ExecContext(ctx, `INSERT INTO stage_contracts(key,version,response_instruction,schema_json,semantic_validator_key,is_system)
			VALUES(?,2,?,?,?,1)
			ON CONFLICT(key,version) DO NOTHING`,
			item.key, cleanSeedText(item.instruction), cleanSeedText(item.schema), item.validator)
		if err != nil {
			return err
		}
	}

	for _, item := range builtInStageTypes() {
		_, err = tx.ExecContext(ctx, `INSERT INTO stage_types(key,display_name,executor_key,input_contract_id,output_contract_id,is_system,is_enabled)
			VALUES(?,?,?,(SELECT id FROM stage_contracts WHERE key=? AND version=2),(SELECT id FROM stage_contracts WHERE key=? AND version=2),1,1)
			ON CONFLICT(key) DO UPDATE SET display_name=excluded.display_name,executor_key=excluded.executor_key,input_contract_id=excluded.input_contract_id,output_contract_id=excluded.output_contract_id,is_enabled=1`,
			item.key, item.name, item.executor, nullableContract(item.input), nullableContract(item.output))
		if err != nil {
			return err
		}
	}
	defaults := pipelineSeedConfig{true, true, true, true, 2000, 3500, 3500, 2500, 2500, 5}
	if err = ensurePipelineSeed(ctx, tx, "system-default", "Pipeline padrão", nil, defaults); err != nil {
		return err
	}

	rows, err := tx.QueryContext(ctx, `SELECT rp.id,
		pol.review_planner_enabled,pol.review_consolidator_enabled,pol.review_verifier_enabled,pol.review_formatter_enabled,
		pol.review_planner_max_output_tokens,pol.review_group_max_output_tokens,
		pol.review_consolidator_max_output_tokens,pol.review_verifier_max_output_tokens,
		pol.review_formatter_max_output_tokens,pol.review_final_retries
		FROM review_profiles rp JOIN review_policies pol ON pol.profile_id=rp.id`)
	if err != nil {
		return err
	}
	type profilePipeline struct {
		id                                         int64
		planner, consolidator, verifier, formatter int
		plannerTokens, groupTokens                 int
		consolidatorTokens, verifierTokens         int
		formatterTokens, retries                   int
	}
	profiles := []profilePipeline{}
	for rows.Next() {
		var item profilePipeline
		if err = rows.Scan(&item.id, &item.planner, &item.consolidator, &item.verifier, &item.formatter,
			&item.plannerTokens, &item.groupTokens, &item.consolidatorTokens, &item.verifierTokens,
			&item.formatterTokens, &item.retries); err != nil {
			rows.Close()
			return err
		}
		profiles = append(profiles, item)
	}
	if err = rows.Close(); err != nil {
		return err
	}
	for _, item := range profiles {
		profileID := item.id
		cfg := pipelineSeedConfig{
			item.planner != 0, item.consolidator != 0, item.verifier != 0, item.formatter != 0,
			item.plannerTokens, item.groupTokens, item.consolidatorTokens, item.verifierTokens,
			item.formatterTokens, item.retries,
		}
		if err = ensurePipelineSeed(ctx, tx, fmt.Sprintf("profile-%d-default", item.id), "Pipeline migrado do profile", &profileID, cfg); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func ensurePipelineSeed(ctx context.Context, tx *sql.Tx, key, name string, profileID *int64, cfg pipelineSeedConfig) error {
	var definitionID int64
	err := tx.QueryRowContext(ctx, `SELECT id FROM pipeline_definitions WHERE key=?`, key).Scan(&definitionID)
	if err == nil {
		return nil
	}
	if err != sql.ErrNoRows {
		return err
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO pipeline_definitions(profile_id,key,name,description,is_default,is_enabled)
		VALUES(?,?,?,'Fluxo multiestágio migrado para o executor orientado pelo banco.',1,1)`, profileID, key, name)
	if err != nil {
		return err
	}
	definitionID, err = result.LastInsertId()
	if err != nil {
		return err
	}
	result, err = tx.ExecContext(ctx, `INSERT INTO pipeline_versions(pipeline_definition_id,version,status,published_at)
		VALUES(?,1,'published',CURRENT_TIMESTAMP)`, definitionID)
	if err != nil {
		return err
	}
	versionID, err := result.LastInsertId()
	if err != nil {
		return err
	}

	stages := []pipelineStageSeed{
		{"preparacao", "Preparação", "preparation", "", 0, 1, false, true},
		{"planejamento", "Planejamento", "planner", "Agrupe os arquivos por contexto e risco para orientar a revisão.", cfg.plannerTokens, 1, cfg.planner, false},
		{"revisao", "Revisão", "reviewer", "Revise somente o diff fornecido e reporte problemas concretos introduzidos pela mudança.", cfg.groupTokens, cfg.retries, true, false},
		{"consolidacao", "Consolidação", "consolidator", "Consolide e deduplique os achados, preservando apenas os comprovados pelo diff.", cfg.consolidatorTokens, 1, cfg.consolidator, false},
		{"verificacao", "Verificação", "verification", "Verifique cada achado contra o diff e rejeite hipóteses sem evidência suficiente.", cfg.verifierTokens, 1, cfg.verifier, true},
		{"formatacao", "Formatação", "formatting", "Formate os achados confirmados para publicação objetiva no pull request.", cfg.formatterTokens, 1, cfg.formatter, true},
		{"publicacao", "Publicação", "publication", "", 0, 1, false, true},
	}
	stageIDs := make([]int64, 0, len(stages))
	for position, stage := range stages {
		result, err = tx.ExecContext(ctx, `INSERT INTO pipeline_stages(
			pipeline_version_id,stage_type_id,stage_key,display_name,position,prompt_template,
			max_output_tokens,retry_limit,use_llm,is_required,is_enabled,input_contract_id,output_contract_id)
			VALUES(?,(SELECT id FROM stage_types WHERE key=?),?,?,?,?,?,?,?,?,1,
			(SELECT input_contract_id FROM stage_types WHERE key=?),(SELECT output_contract_id FROM stage_types WHERE key=?))`,
			versionID, stage.stageType, stage.key, stage.name, position+1, stage.prompt,
			stage.tokens, positive(stage.retries, 1), boolInt(stage.useLLM), boolInt(stage.required), stage.stageType, stage.stageType)
		if err != nil {
			return err
		}
		stageID, err := result.LastInsertId()
		if err != nil {
			return err
		}
		stageIDs = append(stageIDs, stageID)
	}
	for i := 0; i < len(stageIDs)-1; i++ {
		if _, err = tx.ExecContext(ctx, `INSERT INTO pipeline_transitions(
			pipeline_version_id,from_stage_id,to_stage_id,transition_type,condition_key,priority)
			VALUES(?,?,?,'success','always',0)`, versionID, stageIDs[i], stageIDs[i+1]); err != nil {
			return err
		}
	}
	for index, source := range []string{"webhook", "api", "manual"} {
		y := (index - 1) * 145
		if _, err = tx.ExecContext(ctx, `INSERT INTO pipeline_version_triggers(
			pipeline_version_id,trigger_source,is_enabled,target_stage_key,config_json)
			VALUES(?,?,1,'preparacao',?)`, versionID, source, fmt.Sprintf(`{"x":-360,"y":%d}`, y)); err != nil {
			return err
		}
	}
	return nil
}

func builtInContracts() []contractSeed {
	return []contractSeed{
		{"prepared_diff", "", `{"type":"object","required":["files","ignored"],"properties":{"files":{"type":"array","items":{"type":"object","required":["Path","Patch"],"properties":{"Path":{"type":"string"},"Patch":{"type":"string"}}}},"ignored":{"type":"integer"}}}`, "prepared_diff"},
		{"review_plan", `Responda somente JSON: {"pr_summary":"...","risk_level":"...","risk_areas":["..."],"groups":[{"id":"...","purpose":"...","files":["path"],"risk_level":"...","review_focus":["..."]}],"assumptions":["..."]}. Cada arquivo do diff deve aparecer exatamente uma vez.`, `{"type":"object","required":["groups"],"properties":{"pr_summary":{"type":"string"},"risk_level":{"type":"string"},"risk_areas":{"type":"array","items":{"type":"string"}},"groups":{"type":"array","items":{"type":"object","required":["id","files"],"properties":{"id":{"type":"string"},"purpose":{"type":"string"},"files":{"type":"array","items":{"type":"string"}},"risk_level":{"type":"string"},"review_focus":{"type":"array","items":{"type":"string"}}}}},"assumptions":{"type":"array","items":{"type":"string"}}}}`, "review_plan"},
		{"review_findings", `Responda somente JSON: {"group_id":"...","reviewed_files":["path"],"findings":[{"id":"...","file":"path","line":1,"severity":"alta","confidence":0.9,"decision_reason":"...","comment":"...","introduced_by_pr":true}],"review_summary":"..."}.`, `{"type":"array","items":{"type":"object","required":["group_id","reviewed_files","findings","review_summary"],"properties":{"group_id":{"type":"string"},"reviewed_files":{"type":"array","items":{"type":"string"}},"findings":{"type":"array","items":{"type":"object","required":["id","file","line","severity","confidence","decision_reason","comment","introduced_by_pr"],"properties":{"id":{"type":"string"},"file":{"type":"string"},"line":{"type":"integer"},"end_line":{"type":"integer"},"severity":{"type":"string"},"category":{"type":"string"},"confidence":{"type":"number"},"decision_reason":{"type":"string"},"comment":{"type":"string"},"title":{"type":"string"},"evidence":{"type":"string"},"failure_scenario":{"type":"string"},"suggested_fix":{"type":"string"},"source_group_id":{"type":"string"},"source_stage":{"type":"string"},"source_finding_ids":{"type":"array","items":{"type":"string"}},"introduced_by_pr":{"type":"boolean"}}}},"review_summary":{"type":"string"}}}}`, "review_findings"},
		{"consolidated_findings", `Responda somente JSON: {"findings":[],"pr_summary":"...","overall_risk":"baixo","discarded_findings":[]}.`, `{"type":"object","required":["findings"],"properties":{"findings":{"type":"array","items":{"type":"object","required":["id","file","line","severity","confidence","decision_reason","comment","introduced_by_pr"],"properties":{"id":{"type":"string"},"file":{"type":"string"},"line":{"type":"integer"},"severity":{"type":"string"},"confidence":{"type":"number"},"decision_reason":{"type":"string"},"comment":{"type":"string"},"introduced_by_pr":{"type":"boolean"}}}},"pr_summary":{"type":"string"},"overall_risk":{"type":"string"},"discarded_findings":{"type":"array","items":{"type":"object","required":["source_finding_id","reason"],"properties":{"source_finding_id":{"type":"string"},"reason":{"type":"string"}}}}}}`, "consolidated_findings"},
		{"verified_findings", `Responda somente JSON: {"results":[{"finding_id":"...","status":"confirmed","confidence":0.9,"verification_reason":"..."}]}.`, `{"type":"array","items":{"type":"object","required":["id","file","line","severity","confidence","decision_reason","comment","introduced_by_pr"],"properties":{"id":{"type":"string"},"file":{"type":"string"},"line":{"type":"integer"},"severity":{"type":"string"},"confidence":{"type":"number"},"decision_reason":{"type":"string"},"comment":{"type":"string"},"introduced_by_pr":{"type":"boolean"}}}}`, "verified_findings"},
		{"formatted_review", `Responda somente JSON: {"comments":[],"final_review":{"gitea_event":"APPROVE","status":"aprovado","summary":"...","observations":"..."}}.`, `{"type":"object","required":["comments","final_review"],"properties":{"review_id":{"type":"string"},"provider":{"type":"string"},"model":{"type":"string"},"agent":{"type":"string"},"summary":{"type":"string"},"comments":{"type":"array","items":{"type":"object","required":["file","line","severity","decision_reason","comment"],"properties":{"file":{"type":"string"},"line":{"type":"integer"},"severity":{"type":"string"},"decision_reason":{"type":"string"},"comment":{"type":"string"}}}},"final_review":{"type":"object","required":["gitea_event","status","summary","observations"],"properties":{"gitea_event":{"type":"string"},"status":{"type":"string"},"summary":{"type":"string"},"observations":{"type":"string"}}},"metadata":{"type":"object"}}}`, "formatted_review"},
		{"publication_result", "", `{"type":"object","required":["status"],"properties":{"status":{"type":"string"}}}`, "publication_result"},
		{"error_payload", "", `{"type":"object","required":["error","source_stage","transition_type","condition"],"properties":{"error":{"type":"string"},"source_stage":{"type":"string"},"transition_type":{"type":"string"},"condition":{"type":"string"}}}`, "error_payload"},
		{"error_log", "", `{"type":"object","required":["error"],"properties":{"error":{"type":"string"},"source_stage":{"type":"string"},"transition_type":{"type":"string"},"condition":{"type":"string"}}}`, "error_log"},
	}
}

func builtInStageTypes() []stageTypeSeed {
	return []stageTypeSeed{
		{"preparation", "Preparação", "preparation", "", "prepared_diff"},
		{"planner", "Planejamento", "planner", "prepared_diff", "review_plan"},
		{"reviewer", "Revisão por grupos", "reviewer", "review_plan", "review_findings"},
		{"llm_review", "Revisão configurável", "reviewer", "prepared_diff", "review_findings"},
		{"consolidator", "Consolidação", "consolidator", "review_findings", "consolidated_findings"},
		{"verification", "Verificação", "verification", "consolidated_findings", "verified_findings"},
		{"formatting", "Formatação", "formatting", "verified_findings", "formatted_review"},
		{"publication", "Publicação", "publication", "formatted_review", "publication_result"},
		{"error_log", "Registro de erro", "error_log", "error_payload", "error_log"},
		{"file_filter", "Filtro de arquivos", "rule_filter", "prepared_diff", "prepared_diff"},
		{"findings_merge", "Merge de achados", "transform_merge", "review_findings", "review_findings"},
	}
}

func nullableContract(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func positive(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func cleanSeedText(value string) string {
	return strings.ReplaceAll(value, `\"`, `"`)
}
