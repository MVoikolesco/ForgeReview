CREATE TABLE IF NOT EXISTS stage_artifact_inputs (
    artifact_id INTEGER NOT NULL REFERENCES stage_artifacts(id) ON DELETE CASCADE,
    input_artifact_id INTEGER NOT NULL REFERENCES stage_artifacts(id),
    ordinal INTEGER NOT NULL,
    PRIMARY KEY (artifact_id, input_artifact_id, ordinal)
);

CREATE INDEX IF NOT EXISTS idx_stage_artifact_inputs_input
    ON stage_artifact_inputs(input_artifact_id, artifact_id);

ALTER TABLE pipeline_stages ADD COLUMN input_contract_id INTEGER REFERENCES stage_contracts(id);
ALTER TABLE pipeline_stages ADD COLUMN output_contract_id INTEGER REFERENCES stage_contracts(id);

UPDATE pipeline_stages SET
    input_contract_id=(SELECT input_contract_id FROM stage_types WHERE id=pipeline_stages.stage_type_id),
    output_contract_id=(SELECT output_contract_id FROM stage_types WHERE id=pipeline_stages.stage_type_id);

UPDATE stage_contracts SET is_system=0 WHERE version<2;

CREATE TRIGGER IF NOT EXISTS stage_contracts_system_update_guard
BEFORE UPDATE ON stage_contracts WHEN OLD.is_system=1
BEGIN
    SELECT RAISE(ABORT, 'system stage contracts are immutable');
END;

CREATE TRIGGER IF NOT EXISTS stage_contracts_system_delete_guard
BEFORE DELETE ON stage_contracts WHEN OLD.is_system=1
BEGIN
    SELECT RAISE(ABORT, 'system stage contracts are immutable');
END;

UPDATE workflow_processor_catalog SET is_executable=1 WHERE key IN ('rule_filter','transform_merge');
UPDATE workflow_processor_catalog SET config_schema_json='{"type":"object","properties":{"rule":{"type":"object"},"include_extensions":{"type":"array","items":{"type":"string"}}}}' WHERE key='rule_filter';
UPDATE workflow_processor_catalog SET config_schema_json='{"type":"object","properties":{"operation":{"type":"string","enum":["identity","merge","dedupe_findings"]}}}' WHERE key='transform_merge';
