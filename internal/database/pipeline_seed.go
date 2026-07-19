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
			VALUES(?,1,?,?,?,1)
			ON CONFLICT(key,version) DO NOTHING`,
			item.key, cleanSeedText(item.instruction), cleanSeedText(item.schema), item.validator)
		if err != nil {
			return err
		}
	}

	for _, item := range builtInStageTypes() {
		_, err = tx.ExecContext(ctx, `INSERT INTO stage_types(key,display_name,executor_key,input_contract_id,output_contract_id,is_system,is_enabled)
			VALUES(?,?,?,(SELECT id FROM stage_contracts WHERE key=? AND version=1),(SELECT id FROM stage_contracts WHERE key=? AND version=1),1,1)
			ON CONFLICT(key) DO NOTHING`,
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
			max_output_tokens,retry_limit,use_llm,is_required,is_enabled)
			VALUES(?,(SELECT id FROM stage_types WHERE key=?),?,?,?,?,?,?,?,?,1)`,
			versionID, stage.stageType, stage.key, stage.name, position+1, stage.prompt,
			stage.tokens, positive(stage.retries, 1), boolInt(stage.useLLM), boolInt(stage.required))
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
	return nil
}

func builtInContracts() []contractSeed {
	return []contractSeed{
		{"prepared_diff", "", `{"type":"object","required":["files"]}`, "prepared_diff"},
		{"review_plan", `Responda somente JSON: {"pr_summary":"...","risk_level":"...","risk_areas":["..."],"groups":[{"id":"...","purpose":"...","files":["path"],"risk_level":"...","review_focus":["..."]}],"assumptions":["..."]}. Cada arquivo do diff deve aparecer exatamente uma vez.`, `{"type":"object","required":["groups"]}`, "review_plan"},
		{"review_findings", `Responda somente JSON: {"group_id":"...","reviewed_files":["path"],"findings":[{"id":"...","file":"path","line":1,"severity":"critica|alta|media|baixa","category":"...","confidence":0.0,"decision_reason":"...","comment":"...","title":"...","evidence":"...","failure_scenario":"...","suggested_fix":"...","source_group_id":"...","introduced_by_pr":true}],"review_summary":"..."}. Reporte somente linhas adicionadas no diff.`, `{"type":"object","required":["findings"]}`, "review_findings"},
		{"consolidated_findings", `Responda somente JSON: {"findings":[...],"pr_summary":"...","overall_risk":"...","discarded_findings":[{"source_finding_id":"...","reason":"..."}]}. Preserve apenas achados comprovados no diff.`, `{"type":"object","required":["findings"]}`, "consolidated_findings"},
		{"verified_findings", `Responda somente JSON: {"results":[{"finding_id":"...","status":"confirmed|adjusted|rejected","confidence":0.0,"verification_reason":"...","adjusted_finding":null}]}. Confirme somente problemas com evidência suficiente.`, `{"type":"object","required":["results"]}`, "verified_findings"},
		{"formatted_review", `Responda somente JSON: {"comments":[{"file":"path","line":1,"severity":"critica|alta|media|baixa","type":"...","decision_reason":"...","comment":"..."}],"final_review":{"gitea_event":"APPROVE|COMMENT|REQUEST_CHANGES","status":"...","summary":"...","observations":"..."}}.`, `{"type":"object","required":["comments","final_review"]}`, "formatted_review"},
		{"publication_result", "", `{"type":"object","required":["status"]}`, "publication_result"},
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
