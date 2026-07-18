package providers

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"gitea-agents/internal/contracts"
)

type openAICompatible struct {
	cfg Config
}

// Name returns the configured OpenAI-compatible provider identifier.
func (p *openAICompatible) Name() string {
	return p.cfg.Name
}

// Review sends one diff block to an OpenAI-compatible chat completion endpoint.
func (p *openAICompatible) Review(ctx context.Context, input Input) (contracts.Result, error) {
	var output struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	payload := map[string]any{
		"model":       p.cfg.Model,
		"temperature": 0,
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
		strings.TrimRight(p.cfg.BaseURL, "/")+"/chat/completions",
		p.cfg.APIKey,
		payload,
		&output,
	)
	if err != nil {
		return contracts.Result{}, err
	}
	if len(output.Choices) == 0 {
		return contracts.Result{}, fmt.Errorf("provider returned no choices")
	}

	return parseResult(output.Choices[0].Message.Content)
}
