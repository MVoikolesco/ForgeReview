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
	content, _, err := p.Chat(ctx, input.Prompt+"\n\nDIFF:\n"+input.Diff, 0)
	if err != nil {
		return contracts.Result{}, err
	}
	return parseResult(content)
}

// Chat sends a raw Ollama chat response for a pipeline stage.
func (p *ollama) Chat(ctx context.Context, prompt string, maxOutputTokens int) (string, Usage, error) {
	var output struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		PromptEvalCount int `json:"prompt_eval_count"`
		EvalCount       int `json:"eval_count"`
	}

	payload := map[string]any{
		"model":  p.cfg.Model,
		"stream": false,
		"format": "json",
		"messages": []any{
			map[string]string{
				"role":    "user",
				"content": prompt,
			},
		},
	}
	if maxOutputTokens > 0 {
		payload["options"] = map[string]int{"num_predict": maxOutputTokens}
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
		return "", Usage{}, err
	}
	return output.Message.Content, Usage{PromptTokens: output.PromptEvalCount, CompletionTokens: output.EvalCount}, nil
}
