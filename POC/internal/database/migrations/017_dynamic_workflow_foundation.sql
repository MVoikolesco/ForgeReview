ALTER TABLE pipeline_versions ADD COLUMN scheduler_max_runs INTEGER NOT NULL DEFAULT 256 CHECK (scheduler_max_runs > 0);

ALTER TABLE pipeline_stages ADD COLUMN route_mode TEXT NOT NULL DEFAULT 'all_matches'
    CHECK (route_mode IN ('all_matches','first_match'));
ALTER TABLE pipeline_stages ADD COLUMN join_mode TEXT NOT NULL DEFAULT 'each_arrival'
    CHECK (join_mode IN ('each_arrival','any','wait_all'));

ALTER TABLE pipeline_transitions ADD COLUMN rule_json TEXT;
ALTER TABLE pipeline_transitions ADD COLUMN max_traversals INTEGER NOT NULL DEFAULT 0 CHECK (max_traversals >= 0);

CREATE TABLE IF NOT EXISTS workflow_processor_catalog (
    key TEXT PRIMARY KEY,
    display_name TEXT NOT NULL,
    description TEXT NOT NULL,
    config_schema_json TEXT NOT NULL DEFAULT '{}',
    is_executable INTEGER NOT NULL DEFAULT 0
);

INSERT OR IGNORE INTO workflow_processor_catalog(key,display_name,description,config_schema_json,is_executable) VALUES
    ('system','System','Registered system stage executor.','{"type":"object"}',1),
    ('llm','LLM','Registered LLM-backed stage executor.','{"type":"object","properties":{"prompt_template":{"type":"string"},"model_id":{"type":["integer","null"]},"max_output_tokens":{"type":"integer","minimum":1}}}',1),
    ('rule_filter','Rule filter','Filters artifacts with the controlled Rule AST.','{"type":"object","required":["rule"],"properties":{"rule":{"type":"object"}}}',0),
    ('transform_merge','Transform / merge','Deterministic artifact transformation and merge.','{"type":"object"}',0);

CREATE TABLE IF NOT EXISTS workflow_entrypoint_catalog (
    key TEXT PRIMARY KEY,
    display_name TEXT NOT NULL,
    adapter_key TEXT NOT NULL UNIQUE,
    config_schema_json TEXT NOT NULL DEFAULT '{}',
    is_enabled INTEGER NOT NULL DEFAULT 1
);

INSERT OR IGNORE INTO workflow_entrypoint_catalog(key,display_name,adapter_key,config_schema_json,is_enabled) VALUES
    ('webhook','Webhook','webhook','{"type":"object"}',1),
    ('api','API','api','{"type":"object"}',1),
    ('manual','Manual','manual','{"type":"object"}',1);
