package database

import "context"

func (d *DB) Seed(ctx context.Context) error {
	providers := [][4]string{{"ollama", "Ollama", "http://localhost:11434", "none"}, {"openrouter", "OpenRouter", "https://openrouter.ai/api/v1", "bearer"}, {"google_gemini", "Google Gemini", "https://generativelanguage.googleapis.com", "api_key"}, {"groq", "Groq", "https://api.groq.com/openai/v1", "bearer"}}
	for _, p := range providers {
		if _, err := d.SQL.ExecContext(ctx, "INSERT INTO ai_providers(name,display_name,base_url,auth_type) VALUES(?,?,?,?) ON CONFLICT(name) DO NOTHING", p[0], p[1], p[2], p[3]); err != nil {
			return err
		}
	}
	_, err := d.SQL.ExecContext(ctx, "UPDATE ai_providers SET is_default=1 WHERE name='ollama' AND NOT EXISTS (SELECT 1 FROM ai_providers WHERE is_default=1)")
	return err
}
