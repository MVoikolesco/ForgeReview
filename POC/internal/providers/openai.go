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

// Model returns the configured model identifier.
func (p *openAICompatible) Model() string { return p.cfg.Model }

// Review sends one diff block to an OpenAI-compatible chat completion endpoint.
func (p *openAICompatible) Review(ctx context.Context, input Input) (contracts.Result, error) {
	content, _, err := p.Chat(ctx, input.Prompt+"\n\nDIFF:\n"+input.Diff, 0)
	if err != nil {
		return contracts.Result{}, err
	}
	return parseResult(content)
}

// Chat sends a raw OpenAI-compatible completion for a pipeline stage.
func (p *openAICompatible) Chat(ctx context.Context, prompt string, maxOutputTokens int) (string, Usage, error) {
	var output struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}

	payload := map[string]any{
		"model":       p.cfg.Model,
		"temperature": 0,
		"messages": []any{
			map[string]string{
				"role":    "user",
				"content": prompt,
			},
		},
	}
	if maxOutputTokens > 0 {
		payload["max_tokens"] = maxOutputTokens
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
		return "", Usage{}, err
	}
	if len(output.Choices) == 0 {
		return "", Usage{}, fmt.Errorf("provider returned no choices")
	}
	return output.Choices[0].Message.Content, Usage{PromptTokens: output.Usage.PromptTokens, CompletionTokens: output.Usage.CompletionTokens}, nil
}
