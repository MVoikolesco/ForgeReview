-- Generic workflow replacement foundation. These tables are intentionally
-- independent from the legacy pipeline stage/transition schema.
CREATE TABLE IF NOT EXISTS workflow_definitions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    key TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    is_enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS workflow_versions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    workflow_definition_id INTEGER NOT NULL REFERENCES workflow_definitions(id) ON DELETE CASCADE,
    version INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','published','archived')),
    max_steps INTEGER NOT NULL DEFAULT 1000 CHECK (max_steps > 0),
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    published_at TEXT,
    UNIQUE (workflow_definition_id, version)
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_workflow_versions_published
    ON workflow_versions(workflow_definition_id) WHERE status='published';

CREATE TABLE IF NOT EXISTS workflow_node_types (
    key TEXT PRIMARY KEY,
    display_name TEXT NOT NULL,
    config_schema_json TEXT NOT NULL DEFAULT '{}',
    input_ports_json TEXT NOT NULL DEFAULT '[]',
    output_ports_json TEXT NOT NULL DEFAULT '[]',
    is_enabled INTEGER NOT NULL DEFAULT 1
);

INSERT OR REPLACE INTO workflow_node_types(key,display_name,config_schema_json,input_ports_json,output_ports_json,is_enabled) VALUES
    ('source','Source','{"type":"object","properties":{"value":{}}}','[]','[{"key":"out","contract":"any"},{"key":"error","contract":"error"}]',1),
    ('passthrough','Passthrough','{"type":"object"}','[{"key":"in","contract":"any","required":true}]','[{"key":"out","contract":"any"},{"key":"error","contract":"error"}]',1),
    ('condition','Condition','{"type":"object","properties":{"field":{"type":"string"},"equals":{}}}','[{"key":"in","contract":"any","required":true}]','[{"key":"true","contract":"any"},{"key":"false","contract":"any"},{"key":"error","contract":"error"}]',1),
    ('merge','Merge','{"type":"object"}','[{"key":"left","contract":"any","required":true},{"key":"right","contract":"any","required":true}]','[{"key":"out","contract":"any"},{"key":"error","contract":"error"}]',1),
    ('sink','Sink','{"type":"object"}','[{"key":"in","contract":"any","required":true}]','[{"key":"error","contract":"error"}]',1),
    ('fail','Fail (test-safe)','{"type":"object","properties":{"message":{"type":"string"}}}','[{"key":"in","contract":"any","required":true}]','[{"key":"error","contract":"error"}]',1);

CREATE TABLE IF NOT EXISTS workflow_nodes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    workflow_version_id INTEGER NOT NULL REFERENCES workflow_versions(id) ON DELETE CASCADE,
    node_key TEXT NOT NULL,
    node_type_key TEXT NOT NULL,
    config_json TEXT NOT NULL DEFAULT '{}',
    join_mode TEXT NOT NULL DEFAULT 'all' CHECK (join_mode IN ('all','any')),
    error_policy TEXT NOT NULL DEFAULT 'fail' CHECK (error_policy IN ('fail','continue','route')),
    position_json TEXT NOT NULL DEFAULT '{}',
    UNIQUE (workflow_version_id, node_key)
);

CREATE TABLE IF NOT EXISTS workflow_node_ports (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    workflow_node_id INTEGER NOT NULL REFERENCES workflow_nodes(id) ON DELETE CASCADE,
    port_key TEXT NOT NULL,
    direction TEXT NOT NULL CHECK (direction IN ('input','output')),
    contract_key TEXT NOT NULL,
    is_required INTEGER NOT NULL DEFAULT 0,
    accepts_many INTEGER NOT NULL DEFAULT 0,
    UNIQUE (workflow_node_id, direction, port_key)
);

CREATE TABLE IF NOT EXISTS workflow_edges (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    workflow_version_id INTEGER NOT NULL REFERENCES workflow_versions(id) ON DELETE CASCADE,
    edge_key TEXT NOT NULL,
    from_node_id INTEGER NOT NULL REFERENCES workflow_nodes(id) ON DELETE CASCADE,
    from_port_key TEXT NOT NULL,
    to_node_id INTEGER NOT NULL REFERENCES workflow_nodes(id) ON DELETE CASCADE,
    to_port_key TEXT NOT NULL,
    priority INTEGER NOT NULL DEFAULT 0,
    UNIQUE (workflow_version_id, edge_key)
);

CREATE INDEX IF NOT EXISTS idx_workflow_edges_source
    ON workflow_edges(workflow_version_id, from_node_id, from_port_key, priority, id);
CREATE INDEX IF NOT EXISTS idx_workflow_edges_target
    ON workflow_edges(workflow_version_id, to_node_id, to_port_key, priority, id);

CREATE TABLE IF NOT EXISTS workflow_executions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    workflow_version_id INTEGER NOT NULL REFERENCES workflow_versions(id),
    status TEXT NOT NULL DEFAULT 'running' CHECK (status IN ('pending','running','completed','partial_failed','failed','cancelled')),
    definition_snapshot_json TEXT NOT NULL,
    started_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    finished_at TEXT,
    error_message TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS workflow_scopes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    workflow_execution_id INTEGER NOT NULL REFERENCES workflow_executions(id) ON DELETE CASCADE,
    scope_key TEXT NOT NULL,
    parent_scope_id INTEGER REFERENCES workflow_scopes(id),
    metadata_json TEXT NOT NULL DEFAULT '{}',
    UNIQUE (workflow_execution_id, scope_key)
);

CREATE TABLE IF NOT EXISTS workflow_node_executions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    workflow_execution_id INTEGER NOT NULL REFERENCES workflow_executions(id) ON DELETE CASCADE,
    workflow_node_id INTEGER NOT NULL REFERENCES workflow_nodes(id),
    workflow_scope_id INTEGER NOT NULL REFERENCES workflow_scopes(id),
    attempt INTEGER NOT NULL DEFAULT 1,
    status TEXT NOT NULL CHECK (status IN ('pending','ready','running','completed','skipped','failed','partial_failed','cancelled','waiting')),
    started_at TEXT,
    finished_at TEXT,
    error_message TEXT NOT NULL DEFAULT '',
    metadata_json TEXT NOT NULL DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_workflow_node_executions_run
    ON workflow_node_executions(workflow_execution_id, workflow_scope_id, workflow_node_id, id);

CREATE TABLE IF NOT EXISTS workflow_tokens (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    workflow_execution_id INTEGER NOT NULL REFERENCES workflow_executions(id) ON DELETE CASCADE,
    workflow_scope_id INTEGER NOT NULL REFERENCES workflow_scopes(id),
    contract_key TEXT NOT NULL,
    source_node_execution_id INTEGER REFERENCES workflow_node_executions(id),
    source_port_key TEXT NOT NULL,
    target_node_id INTEGER REFERENCES workflow_nodes(id),
    target_port_key TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'ready' CHECK (status IN ('ready','consumed','discarded')),
    payload_json TEXT NOT NULL,
    payload_hash TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_workflow_tokens_ready
    ON workflow_tokens(workflow_execution_id, workflow_scope_id, target_node_id, target_port_key, status, id);
