CREATE TABLE IF NOT EXISTS pipeline_version_triggers (
    pipeline_version_id INTEGER NOT NULL REFERENCES pipeline_versions(id) ON DELETE CASCADE,
    trigger_source TEXT NOT NULL,
    is_enabled INTEGER NOT NULL DEFAULT 1,
    PRIMARY KEY (pipeline_version_id, trigger_source)
);

INSERT OR IGNORE INTO pipeline_version_triggers(pipeline_version_id,trigger_source,is_enabled)
SELECT id,'webhook',1 FROM pipeline_versions;
INSERT OR IGNORE INTO pipeline_version_triggers(pipeline_version_id,trigger_source,is_enabled)
SELECT id,'api',1 FROM pipeline_versions;
INSERT OR IGNORE INTO pipeline_version_triggers(pipeline_version_id,trigger_source,is_enabled)
SELECT id,'manual',1 FROM pipeline_versions;

ALTER TABLE stage_artifacts ADD COLUMN source_stage_id INTEGER REFERENCES pipeline_stages(id);
ALTER TABLE stage_artifacts ADD COLUMN source_transition_id INTEGER REFERENCES pipeline_transitions(id);
