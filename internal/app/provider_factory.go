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

// providerFactory returns a lazy provider constructor backed by the pipeline
// profile or by an explicit model configured for a stage.
func providerFactory(db *sql.DB, cfg config.Config) func(context.Context, *int64, *int64) (providers.LLMProvider, error) {
	return func(ctx context.Context, profileID, modelID *int64) (providers.LLMProvider, error) {
		var provider, baseURL, model, ciphertext string
		var timeout int
		var selectedModel any
		if modelID != nil {
			selectedModel = *modelID
		}
		var selectedProfile any
		if profileID != nil {
			selectedProfile = *profileID
		}
		err := db.QueryRowContext(
			ctx,
			`SELECT p.name,c.base_url,m.provider_model_name,c.api_key_ciphertext,
			        COALESCE(mp.timeout_seconds,?)
			 FROM ai_models m
			 JOIN ai_connections c ON c.id=m.connection_id
			 JOIN ai_providers p ON p.id=c.provider_id
			 LEFT JOIN model_parameters mp ON mp.model_id=m.id
			 WHERE m.id=COALESCE(?,(
			   SELECT rp.model_id FROM review_profiles rp
			   WHERE rp.id=COALESCE(?,(SELECT id FROM review_profiles WHERE is_default=1 AND is_enabled=1 LIMIT 1))
			     AND rp.is_enabled=1
			 )) AND m.is_enabled=1 AND c.is_enabled=1 AND p.is_enabled=1
			 LIMIT 1`,
			cfg.ReviewRequestTimeoutSeconds,
			selectedModel,
			selectedProfile,
		).Scan(&provider, &baseURL, &model, &ciphertext, &timeout)
		if err != nil {
			return nil, fmt.Errorf("review provider is not configured: %w", err)
		}

		key := ""
		if ciphertext != "" {
			var decryptErr error
			key, decryptErr = security.Decrypt(ciphertext)
			if decryptErr != nil {
				return nil, fmt.Errorf("provider credential could not be decrypted: %w", decryptErr)
			}
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
