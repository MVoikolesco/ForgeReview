package app

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"gitea-agents/internal/config"
	"gitea-agents/internal/providers"
	"gitea-agents/internal/security"
)

// providerFactory returns a lazy provider constructor backed by the current
// default profile. Each invocation reads active connection/model settings and
// returns a configured LLMProvider or a configuration error.
func providerFactory(db *sql.DB, cfg config.Config) func(context.Context) (providers.LLMProvider, error) {
	return func(ctx context.Context) (providers.LLMProvider, error) {
		var provider, baseURL, model, ciphertext string
		var timeout int
		err := db.QueryRowContext(
			ctx,
			`SELECT p.name,c.base_url,m.provider_model_name,c.api_key_ciphertext,
			        COALESCE(mp.timeout_seconds,?)
			 FROM review_profiles rp
			 JOIN ai_models m ON m.id=rp.model_id
			 JOIN ai_connections c ON c.id=m.connection_id
			 JOIN ai_providers p ON p.id=c.provider_id
			 LEFT JOIN model_parameters mp ON mp.model_id=m.id
			 WHERE rp.is_default=1 AND rp.is_enabled=1
			   AND m.is_enabled=1 AND c.is_enabled=1 AND p.is_enabled=1
			 LIMIT 1`,
			cfg.ReviewRequestTimeoutSeconds,
		).Scan(&provider, &baseURL, &model, &ciphertext, &timeout)
		if err != nil {
			return nil, fmt.Errorf("review provider is not configured")
		}

		key := ""
		if ciphertext != "" {
			key, _ = security.Decrypt(ciphertext)
		}

		return providers.New(providers.Config{
			Name:    provider,
			BaseURL: baseURL,
			APIKey:  key,
			Model:   model,
			Timeout: time.Duration(timeout) * time.Second,
		}), nil
	}
}
