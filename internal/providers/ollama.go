package providers

import (
	"context"
	"net/http"
	"strings"

	"gitea-agents/internal/contracts"
)

type ollama struct {
	cfg Config
}

// Name returns Ollama's provider identifier.
func (p *ollama) Name() string {
	return "ollama"
}

// Review sends one diff block to Ollama's chat endpoint and parses its result.
func (p *ollama) Review(ctx context.Context, input Input) (contracts.Result, error) {
	var output struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}

	payload := map[string]any{
		"model":  p.cfg.Model,
		"stream": false,
		"format": "json",
		"messages": []any{
			map[string]string{
				"role":    "user",
				"content": input.Prompt + "\n\nDIFF:\n" + input.Diff,
			},
		},
	}
	err := requestJSON(
		ctx,
		&http.Client{Timeout: p.cfg.Timeout},
		http.MethodPost,
		strings.TrimRight(p.cfg.BaseURL, "/")+"/api/chat",
		p.cfg.APIKey,
		payload,
		&output,
	)
	if err != nil {
		return contracts.Result{}, err
	}

	return parseResult(output.Message.Content)
}
