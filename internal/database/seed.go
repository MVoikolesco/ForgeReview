package database

import "context"

type providerSeed struct {
	name        string
	displayName string
	baseURL     string
	authType    string
}

// Seed inserts the built-in AI provider catalog and ensures that Ollama is the
// initial default. Existing provider rows are preserved.
func (d *DB) Seed(ctx context.Context) error {
	providers := []providerSeed{
		{name: "ollama", displayName: "Ollama", baseURL: "http://localhost:11434", authType: "none"},
		{name: "openrouter", displayName: "OpenRouter", baseURL: "https://openrouter.ai/api/v1", authType: "bearer"},
		{name: "google_gemini", displayName: "Google Gemini", baseURL: "https://generativelanguage.googleapis.com", authType: "api_key"},
		{name: "groq", displayName: "Groq", baseURL: "https://api.groq.com/openai/v1", authType: "bearer"},
	}

	for _, provider := range providers {
		_, err := d.SQL.ExecContext(
			ctx,
			"INSERT INTO ai_providers(name,display_name,base_url,auth_type) VALUES(?,?,?,?) ON CONFLICT(name) DO NOTHING",
			provider.name,
			provider.displayName,
			provider.baseURL,
			provider.authType,
		)
		if err != nil {
			return err
		}
	}

	_, err := d.SQL.ExecContext(ctx, "UPDATE ai_providers SET is_default=1 WHERE name='ollama' AND NOT EXISTS (SELECT 1 FROM ai_providers WHERE is_default=1)")
	if err != nil {
		return err
	}
	return d.seedPipelines(ctx)
}
