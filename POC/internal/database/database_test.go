package database

import (
	"context"
	"encoding/json"
	"testing"

	"gitea-agents/internal/config"
)

func TestMigrateAndSeed(t *testing.T) {
	db, err := Open(config.Config{DatabaseDriver: "sqlite", DatabaseDSN: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"ai_providers", "review_profiles", "reviews", "review_steps", "pending_reviews", "stage_contracts", "stage_types", "pipeline_definitions", "pipeline_versions", "pipeline_stages", "pipeline_transitions", "pipeline_executions", "stage_executions", "stage_artifacts", "review_publications", "workflow_processor_catalog", "workflow_entrypoint_catalog", "workflow_definitions", "workflow_versions", "workflow_node_types", "workflow_nodes", "workflow_node_ports", "workflow_edges", "workflow_executions", "workflow_scopes", "workflow_node_executions", "workflow_tokens"} {
		var name string
		if err := db.SQL.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name); err != nil {
			t.Fatalf("missing %s: %v", table, err)
		}
	}
	var providers int
	if err := db.SQL.QueryRowContext(ctx, "SELECT count(*) FROM ai_providers").Scan(&providers); err != nil {
		t.Fatal(err)
	}
	if providers < 4 {
		t.Fatalf("expected seeded providers, got %d", providers)
	}
	var stages, transitions int
	if err := db.SQL.QueryRowContext(ctx, `SELECT count(*) FROM pipeline_stages ps
		JOIN pipeline_versions pv ON pv.id=ps.pipeline_version_id
		JOIN pipeline_definitions pd ON pd.id=pv.pipeline_definition_id
		WHERE pd.key='system-default' AND pv.status='published'`).Scan(&stages); err != nil {
		t.Fatal(err)
	}
	if err := db.SQL.QueryRowContext(ctx, `SELECT count(*) FROM pipeline_transitions pt
		JOIN pipeline_versions pv ON pv.id=pt.pipeline_version_id
		JOIN pipeline_definitions pd ON pd.id=pv.pipeline_definition_id
		WHERE pd.key='system-default'`).Scan(&transitions); err != nil {
		t.Fatal(err)
	}
	if stages != 7 || transitions != 6 {
		t.Fatalf("unexpected default pipeline: stages=%d transitions=%d", stages, transitions)
	}
	if err := db.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	var definitions int
	if err := db.SQL.QueryRowContext(ctx, `SELECT count(*) FROM pipeline_definitions WHERE key='system-default'`).Scan(&definitions); err != nil {
		t.Fatal(err)
	}
	if definitions != 1 {
		t.Fatalf("pipeline seed is not idempotent: %d definitions", definitions)
	}
	if err := db.RequireSchema(ctx); err != nil {
		t.Fatal(err)
	}
	rows, err := db.SQL.QueryContext(ctx, `SELECT key,schema_json FROM stage_contracts`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var key, schema string
		if err := rows.Scan(&key, &schema); err != nil {
			t.Fatal(err)
		}
		if !json.Valid([]byte(schema)) {
			t.Fatalf("contract %s has invalid schema: %s", key, schema)
		}
	}
}

func TestSeedPreservesExistingContractPinsAndUsesV2ForNewStages(t *testing.T) {
	db, err := Open(config.Config{DatabaseDriver: "sqlite", DatabaseDSN: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if err = db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err = db.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = db.SQL.ExecContext(ctx, `
		INSERT INTO stage_contracts(key,version,response_instruction,schema_json,semantic_validator_key,is_system)
		SELECT key,1,response_instruction,schema_json,semantic_validator_key,0 FROM stage_contracts WHERE key='prepared_diff' AND version=2;
		UPDATE pipeline_stages SET input_contract_id=NULL,
			output_contract_id=(SELECT id FROM stage_contracts WHERE key='prepared_diff' AND version=1)
		WHERE id=(SELECT ps.id FROM pipeline_stages ps JOIN stage_types st ON st.id=ps.stage_type_id WHERE st.key='preparation' LIMIT 1);`); err != nil {
		t.Fatal(err)
	}
	if err = db.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	var inputVersion *int
	var outputVersion int
	if err = db.SQL.QueryRowContext(ctx, `SELECT ic.version,oc.version FROM pipeline_stages ps
		JOIN stage_types st ON st.id=ps.stage_type_id
		LEFT JOIN stage_contracts ic ON ic.id=ps.input_contract_id
		JOIN stage_contracts oc ON oc.id=ps.output_contract_id
		WHERE st.key='preparation' LIMIT 1`).Scan(&inputVersion, &outputVersion); err != nil {
		t.Fatal(err)
	}
	if inputVersion != nil || outputVersion != 1 {
		t.Fatalf("seed changed immutable pins: input=%v output=%d", inputVersion, outputVersion)
	}
	var newOutputVersion int
	if err = db.SQL.QueryRowContext(ctx, `SELECT sc.version FROM stage_types st JOIN stage_contracts sc ON sc.id=st.output_contract_id WHERE st.key='preparation'`).Scan(&newOutputVersion); err != nil {
		t.Fatal(err)
	}
	if newOutputVersion != 2 {
		t.Fatalf("new stages would not use v2 contract: %d", newOutputVersion)
	}
}
