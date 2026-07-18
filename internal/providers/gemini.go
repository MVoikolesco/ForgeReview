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

// Review sends one diff block to Gemini generateContent and parses its result.
func (p *gemini) Review(ctx context.Context, input Input) (contracts.Result, error) {
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
	}
	payload := map[string]any{
		"contents": []any{
			map[string]any{
				"parts": []any{
					map[string]string{"text": input.Prompt + "\n\nDIFF:\n" + input.Diff},
				},
			},
		},
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
		return contracts.Result{}, err
	}
	if len(output.Candidates) == 0 || len(output.Candidates[0].Content.Parts) == 0 {
		return contracts.Result{}, fmt.Errorf("provider returned no candidates")
	}

	return parseResult(output.Candidates[0].Content.Parts[0].Text)
}
