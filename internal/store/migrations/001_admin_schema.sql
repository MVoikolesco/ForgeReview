CREATE TABLE IF NOT EXISTS ai_providers (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    base_url TEXT NOT NULL DEFAULT '',
    auth_type TEXT NOT NULL DEFAULT 'none',
    is_enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS ai_connections (
    id INTEGER PRIMARY KEY,
    provider_id INTEGER NOT NULL REFERENCES ai_providers (id),
    name TEXT NOT NULL,
    base_url TEXT NOT NULL DEFAULT '',
    api_key_env_name TEXT NOT NULL DEFAULT '',
    organization_id TEXT NOT NULL DEFAULT '',
    project_id TEXT NOT NULL DEFAULT '',
    is_default INTEGER NOT NULL DEFAULT 0,
    is_enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (provider_id, name)
);

CREATE TABLE IF NOT EXISTS ai_models (
    id INTEGER PRIMARY KEY,
    connection_id INTEGER NOT NULL REFERENCES ai_connections (id),
    provider_model_name TEXT NOT NULL,
    display_name TEXT NOT NULL,
    context_window INTEGER NOT NULL DEFAULT 0,
    max_output_tokens INTEGER NOT NULL DEFAULT 0,
    supports_json INTEGER NOT NULL DEFAULT 0,
    supports_tools INTEGER NOT NULL DEFAULT 0,
    supports_streaming INTEGER NOT NULL DEFAULT 0,
    is_default INTEGER NOT NULL DEFAULT 0,
    is_enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (
        connection_id,
        provider_model_name
    )
);

CREATE TABLE IF NOT EXISTS model_parameters (
    id INTEGER PRIMARY KEY,
    model_id INTEGER NOT NULL UNIQUE REFERENCES ai_models (id),
    temperature REAL,
    top_p REAL,
    repeat_penalty REAL,
    num_ctx INTEGER,
    num_threads INTEGER,
    num_predict INTEGER,
    keep_alive TEXT NOT NULL DEFAULT '',
    timeout_seconds INTEGER NOT NULL DEFAULT 900,
    unload_model_after_review INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS review_profiles (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    model_id INTEGER REFERENCES ai_models (id),
    is_default INTEGER NOT NULL DEFAULT 0,
    is_enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS review_prompts (
    id INTEGER PRIMARY KEY,
    profile_id INTEGER NOT NULL REFERENCES review_profiles (id),
    name TEXT NOT NULL,
    prompt_type TEXT NOT NULL,
    stack TEXT NOT NULL DEFAULT '',
    content TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    is_active INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (
        profile_id,
        name,
        stack,
        version
    )
);

CREATE TABLE IF NOT EXISTS review_policies (
    id INTEGER PRIMARY KEY,
    profile_id INTEGER NOT NULL UNIQUE REFERENCES review_profiles (id),
    max_block_chars INTEGER NOT NULL,
    max_files_per_block INTEGER NOT NULL,
    review_concurrency INTEGER NOT NULL DEFAULT 1,
    review_wip_pull_requests INTEGER NOT NULL DEFAULT 0,
    review_own_pull_requests INTEGER NOT NULL DEFAULT 0,
    log_sensitive_data INTEGER NOT NULL DEFAULT 0,
    unload_model_after_review INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS gitea_instances (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    base_url TEXT NOT NULL,
    bot_username TEXT NOT NULL DEFAULT '',
    token_env_name TEXT NOT NULL DEFAULT '',
    is_default INTEGER NOT NULL DEFAULT 0,
    is_enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS repositories (
    id INTEGER PRIMARY KEY,
    gitea_instance_id INTEGER REFERENCES gitea_instances (id),
    owner TEXT NOT NULL,
    name TEXT NOT NULL,
    full_name TEXT NOT NULL UNIQUE,
    review_profile_id INTEGER REFERENCES review_profiles (id),
    is_enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
