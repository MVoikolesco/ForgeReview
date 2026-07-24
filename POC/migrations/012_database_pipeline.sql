CREATE TABLE IF NOT EXISTS stage_contracts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    key TEXT NOT NULL,
    version INTEGER NOT NULL,
    response_instruction TEXT NOT NULL DEFAULT '',
    schema_json TEXT NOT NULL DEFAULT '{}',
    semantic_validator_key TEXT NOT NULL DEFAULT '',
    is_system INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (key, version)
);

CREATE TABLE IF NOT EXISTS stage_types (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    key TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    executor_key TEXT NOT NULL,
    input_contract_id INTEGER REFERENCES stage_contracts(id),
    output_contract_id INTEGER REFERENCES stage_contracts(id),
    is_system INTEGER NOT NULL DEFAULT 1,
    is_enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS pipeline_definitions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    profile_id INTEGER REFERENCES review_profiles(id),
    key TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    is_default INTEGER NOT NULL DEFAULT 0,
    is_enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_pipeline_default_global
    ON pipeline_definitions(is_default)
    WHERE is_default=1 AND profile_id IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS ux_pipeline_default_profile
    ON pipeline_definitions(profile_id)
    WHERE is_default=1 AND profile_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS pipeline_versions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    pipeline_definition_id INTEGER NOT NULL REFERENCES pipeline_definitions(id),
    version INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'draft',
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    published_at TEXT,
    UNIQUE (pipeline_definition_id, version)
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_pipeline_published_version
    ON pipeline_versions(pipeline_definition_id)
    WHERE status='published';

CREATE TABLE IF NOT EXISTS pipeline_stages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    pipeline_version_id INTEGER NOT NULL REFERENCES pipeline_versions(id) ON DELETE CASCADE,
    stage_type_id INTEGER NOT NULL REFERENCES stage_types(id),
    stage_key TEXT NOT NULL,
    display_name TEXT NOT NULL,
    position INTEGER NOT NULL,
    prompt_template TEXT NOT NULL DEFAULT '',
    model_id INTEGER REFERENCES ai_models(id),
    max_output_tokens INTEGER NOT NULL DEFAULT 2000,
    retry_limit INTEGER NOT NULL DEFAULT 1,
    timeout_seconds INTEGER NOT NULL DEFAULT 900,
    use_llm INTEGER NOT NULL DEFAULT 1,
    is_required INTEGER NOT NULL DEFAULT 0,
    is_enabled INTEGER NOT NULL DEFAULT 1,
    config_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (pipeline_version_id, stage_key),
    UNIQUE (pipeline_version_id, position)
);

CREATE TABLE IF NOT EXISTS pipeline_transitions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    pipeline_version_id INTEGER NOT NULL REFERENCES pipeline_versions(id) ON DELETE CASCADE,
    from_stage_id INTEGER NOT NULL REFERENCES pipeline_stages(id) ON DELETE CASCADE,
    to_stage_id INTEGER REFERENCES pipeline_stages(id) ON DELETE CASCADE,
    transition_type TEXT NOT NULL DEFAULT 'success',
    condition_key TEXT NOT NULL DEFAULT 'always',
    priority INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (from_stage_id, transition_type, condition_key, priority)
);

ALTER TABLE reviews ADD COLUMN pipeline_version_id INTEGER REFERENCES pipeline_versions(id);
ALTER TABLE reviews ADD COLUMN pipeline_profile_id INTEGER REFERENCES review_profiles(id);

CREATE TABLE IF NOT EXISTS pipeline_executions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    review_id TEXT NOT NULL REFERENCES reviews(id) ON DELETE CASCADE,
    pipeline_version_id INTEGER NOT NULL REFERENCES pipeline_versions(id),
    status TEXT NOT NULL DEFAULT 'running',
    definition_snapshot_json TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    finished_at TEXT,
    error_message TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_pipeline_executions_review
    ON pipeline_executions(review_id, id DESC);

CREATE TABLE IF NOT EXISTS stage_executions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    pipeline_execution_id INTEGER NOT NULL REFERENCES pipeline_executions(id) ON DELETE CASCADE,
    pipeline_stage_id INTEGER NOT NULL REFERENCES pipeline_stages(id),
    stage_key TEXT NOT NULL,
    attempt INTEGER NOT NULL DEFAULT 1,
    status TEXT NOT NULL DEFAULT 'running',
    artifact_type TEXT NOT NULL DEFAULT '',
    metadata_json TEXT NOT NULL DEFAULT '{}',
    started_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    finished_at TEXT,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    error_message TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_stage_executions_pipeline
    ON stage_executions(pipeline_execution_id, id);

CREATE TABLE IF NOT EXISTS stage_artifacts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    pipeline_execution_id INTEGER NOT NULL REFERENCES pipeline_executions(id) ON DELETE CASCADE,
    stage_execution_id INTEGER NOT NULL REFERENCES stage_executions(id) ON DELETE CASCADE,
    artifact_type TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_stage_artifacts_execution
    ON stage_artifacts(pipeline_execution_id, artifact_type, id);

CREATE TABLE IF NOT EXISTS review_publications (
    review_id TEXT PRIMARY KEY REFERENCES reviews(id) ON DELETE CASCADE,
    status TEXT NOT NULL,
    result_fingerprint TEXT NOT NULL,
    error_message TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    published_at TEXT
);
