package providers

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"gitea-agents/internal/contracts"
)

type gemini struct {
	cfg Config
}

// Name returns Gemini's configured provider identifier.
func (p *gemini) Name() string {
	return "google_gemini"
}

// Model returns the configured model identifier.
func (p *gemini) Model() string { return p.cfg.Model }

// Review sends one diff block to Gemini generateContent and parses its result.
func (p *gemini) Review(ctx context.Context, input Input) (contracts.Result, error) {
	content, _, err := p.Chat(ctx, input.Prompt+"\n\nDIFF:\n"+input.Diff, 0)
	if err != nil {
		return contracts.Result{}, err
	}
	return parseResult(content)
}

// Chat sends a raw Gemini response for a pipeline stage.
func (p *gemini) Chat(ctx context.Context, prompt string, maxOutputTokens int) (string, Usage, error) {
	endpoint := strings.TrimRight(p.cfg.BaseURL, "/") +
		"/v1beta/models/" + p.cfg.Model +
		":generateContent?key=" + p.cfg.APIKey

	var output struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		UsageMetadata struct {
			PromptTokenCount     int `json:"promptTokenCount"`
			CandidatesTokenCount int `json:"candidatesTokenCount"`
		} `json:"usageMetadata"`
	}
	payload := map[string]any{
		"contents": []any{
			map[string]any{
				"parts": []any{
					map[string]string{"text": prompt},
				},
			},
		},
	}
	if maxOutputTokens > 0 {
		payload["generationConfig"] = map[string]int{"maxOutputTokens": maxOutputTokens}
	}

	err := requestJSON(
		ctx,
		&http.Client{Timeout: p.cfg.Timeout},
		http.MethodPost,
		endpoint,
		"",
		payload,
		&output,
	)
	if err != nil {
		return "", Usage{}, err
	}
	if len(output.Candidates) == 0 || len(output.Candidates[0].Content.Parts) == 0 {
		return "", Usage{}, fmt.Errorf("provider returned no candidates")
	}
	return output.Candidates[0].Content.Parts[0].Text, Usage{PromptTokens: output.UsageMetadata.PromptTokenCount, CompletionTokens: output.UsageMetadata.CandidatesTokenCount}, nil
}
