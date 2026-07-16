package store

import (
	"context"
	"gitea-agents/internal/config"
)

func (s *Store) Seed(ctx context.Context, _ config.Config) error {
	providers := []struct{ n, d, u, a string }{{"ollama", "Ollama", "http://localhost:11434", "none"}, {"openrouter", "OpenRouter", "https://openrouter.ai/api/v1", "bearer"}, {"google_gemini", "Google Gemini", "https://generativelanguage.googleapis.com", "api_key"}, {"groq", "Groq", "https://api.groq.com/openai/v1", "bearer"}}
	for _, p := range providers {
		if _, err := s.DB.ExecContext(ctx, "INSERT INTO ai_providers(name,display_name,base_url,auth_type) VALUES(?,?,?,?) ON CONFLICT(name) DO NOTHING", p.n, p.d, p.u, p.a); err != nil {
			return err
		}
	}
	if _, err := s.DB.ExecContext(ctx, "UPDATE ai_providers SET is_default=1 WHERE name='ollama' AND NOT EXISTS (SELECT 1 FROM ai_providers WHERE is_default=1)"); err != nil {
		return err
	}
	return nil
}
