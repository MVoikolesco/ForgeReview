CREATE UNIQUE INDEX IF NOT EXISTS ux_ai_connections_default ON ai_connections (is_default)
WHERE
    is_default = 1;

CREATE UNIQUE INDEX IF NOT EXISTS ux_ai_models_default_connection ON ai_models (connection_id)
WHERE
    is_default = 1;

CREATE UNIQUE INDEX IF NOT EXISTS ux_review_profiles_default ON review_profiles (is_default)
WHERE
    is_default = 1;

CREATE UNIQUE INDEX IF NOT EXISTS ux_gitea_instances_default ON gitea_instances (is_default)
WHERE
    is_default = 1;

CREATE INDEX IF NOT EXISTS ix_repositories_profile ON repositories (review_profile_id);

CREATE INDEX IF NOT EXISTS ix_review_prompts_profile_active ON review_prompts (
    profile_id,
    is_active,
    prompt_type,
    stack,
    version
);
