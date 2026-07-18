package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"gitea-agents/internal/contracts"
	"net/http"
	"strings"
	"time"
)

type Input struct {
	Owner, Repository string
	PullRequest       int
	Diff, Prompt      string
}
type LLMProvider interface {
	Name() string
	Review(context.Context, Input) (contracts.Result, error)
}
type Config struct {
	Name, BaseURL, APIKey, Model string
	Timeout                      time.Duration
}

func New(cfg Config) LLMProvider {
	switch strings.ToLower(cfg.Name) {
	case "ollama":
		return &ollama{cfg: cfg}
	case "google_gemini":
		return &gemini{cfg: cfg}
	default:
		return &openAICompatible{cfg: cfg}
	}
}
func parseResult(raw string) (contracts.Result, error) {
	raw = strings.TrimSpace(raw)
	if i := strings.Index(raw, "```"); i >= 0 {
		raw = strings.TrimSpace(raw[i+3:])
		if strings.HasPrefix(raw, "json") {
			raw = strings.TrimSpace(raw[4:])
		}
		if end := strings.Index(raw, "```"); end >= 0 {
			raw = raw[:end]
		}
	}
	var value contracts.Result
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return value, fmt.Errorf("provider returned invalid review JSON")
	}
	if value.Comments == nil {
		value.Comments = []contracts.Comment{}
	}
	return value, nil
}
func requestJSON(ctx context.Context, client *http.Client, method, url, key string, body any, target any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, url, strings.NewReader(string(data)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("provider request failed with status %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(target)
}

type ollama struct{ cfg Config }

func (p *ollama) Name() string { return "ollama" }
func (p *ollama) Review(ctx context.Context, in Input) (contracts.Result, error) {
	var out struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	err := requestJSON(ctx, &http.Client{Timeout: p.cfg.Timeout}, http.MethodPost, strings.TrimRight(p.cfg.BaseURL, "/")+"/api/chat", p.cfg.APIKey, map[string]any{"model": p.cfg.Model, "stream": false, "format": "json", "messages": []any{map[string]string{"role": "user", "content": in.Prompt + "\n\nDIFF:\n" + in.Diff}}}, &out)
	if err != nil {
		return contracts.Result{}, err
	}
	return parseResult(out.Message.Content)
}

type openAICompatible struct{ cfg Config }

func (p *openAICompatible) Name() string { return p.cfg.Name }
func (p *openAICompatible) Review(ctx context.Context, in Input) (contracts.Result, error) {
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	err := requestJSON(ctx, &http.Client{Timeout: p.cfg.Timeout}, http.MethodPost, strings.TrimRight(p.cfg.BaseURL, "/")+"/chat/completions", p.cfg.APIKey, map[string]any{"model": p.cfg.Model, "temperature": 0, "messages": []any{map[string]string{"role": "user", "content": in.Prompt + "\n\nDIFF:\n" + in.Diff}}}, &out)
	if err != nil {
		return contracts.Result{}, err
	}
	if len(out.Choices) == 0 {
		return contracts.Result{}, fmt.Errorf("provider returned no choices")
	}
	return parseResult(out.Choices[0].Message.Content)
}

type gemini struct{ cfg Config }

func (p *gemini) Name() string { return "google_gemini" }
func (p *gemini) Review(ctx context.Context, in Input) (contracts.Result, error) {
	url := strings.TrimRight(p.cfg.BaseURL, "/") + "/v1beta/models/" + p.cfg.Model + ":generateContent?key=" + p.cfg.APIKey
	var out struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	err := requestJSON(ctx, &http.Client{Timeout: p.cfg.Timeout}, http.MethodPost, url, "", map[string]any{"contents": []any{map[string]any{"parts": []any{map[string]string{"text": in.Prompt + "\n\nDIFF:\n" + in.Diff}}}}}, &out)
	if err != nil {
		return contracts.Result{}, err
	}
	if len(out.Candidates) == 0 || len(out.Candidates[0].Content.Parts) == 0 {
		return contracts.Result{}, fmt.Errorf("provider returned no candidates")
	}
	return parseResult(out.Candidates[0].Content.Parts[0].Text)
}
