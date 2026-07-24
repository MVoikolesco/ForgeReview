-- Older setup versions copied the provider capability
-- top_provider.max_completion_tokens into the per-request output limit.
-- Repair only OpenRouter rows whose value is invalid or clearly unsafe.
UPDATE ai_models
SET
    max_output_tokens = 4096,
    updated_at = CURRENT_TIMESTAMP
WHERE
    id IN (
        SELECT am.id
        FROM
            ai_models am
            JOIN ai_connections ac ON ac.id = am.connection_id
            JOIN ai_providers ap ON ap.id = ac.provider_id
        WHERE
            ap.name = 'openrouter'
            AND (
                am.max_output_tokens <= 0
                OR am.max_output_tokens > 32768
                OR (
                    am.context_window > 0
                    AND am.max_output_tokens >= am.context_window
                )
            )
    );
