ALTER TABLE ai_providers ADD COLUMN is_default INTEGER NOT NULL DEFAULT 0;

UPDATE ai_providers
SET is_default = 1
WHERE id = (
    SELECT ap.id
    FROM review_profiles rp
    JOIN ai_models am ON am.id = rp.model_id
    JOIN ai_connections ac ON ac.id = am.connection_id
    JOIN ai_providers ap ON ap.id = ac.provider_id
    WHERE rp.is_default = 1
    LIMIT 1
);

UPDATE ai_providers
SET is_default = 1
WHERE NOT EXISTS (SELECT 1 FROM ai_providers WHERE is_default = 1)
  AND name = 'ollama';

DROP INDEX IF EXISTS ux_ai_connections_default;

CREATE UNIQUE INDEX IF NOT EXISTS ux_ai_providers_default ON ai_providers (is_default)
WHERE
    is_default = 1;

CREATE UNIQUE INDEX IF NOT EXISTS ux_ai_connections_default_provider ON ai_connections (provider_id)
WHERE
    is_default = 1;

UPDATE ai_connections
SET is_default = 1
WHERE id IN (
    SELECT MIN(id)
    FROM ai_connections
    WHERE is_enabled = 1
    GROUP BY provider_id
)
AND provider_id NOT IN (
    SELECT provider_id
    FROM ai_connections
    WHERE is_default = 1
);

UPDATE ai_models
SET is_default = 1
WHERE id IN (
    SELECT MIN(id)
    FROM ai_models
    WHERE is_enabled = 1
    GROUP BY connection_id
)
AND connection_id NOT IN (
    SELECT connection_id
    FROM ai_models
    WHERE is_default = 1
);
