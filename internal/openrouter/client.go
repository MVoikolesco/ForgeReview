package openrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"gitea-agents/internal/ai"
)

type Config struct {
	URL, Model, APIKey, HTTPReferer, AppTitle string
	Temperature, TopP                         float64
	TimeoutSeconds, ContextWindow, MaxTokens  int
}
type Client struct {
	url, model, key, httpReferer, appTitle string
	temperature, topP                      float64
	contextWindow, maxTokens               int
	http                                   *http.Client
}

var _ ai.Client = (*Client)(nil)

func NewClient(c Config) *Client {
	t := time.Duration(c.TimeoutSeconds) * time.Second
	if t <= 0 {
		t = 900 * time.Second
	}
	return &Client{url: strings.TrimRight(c.URL, "/"), model: c.Model, key: c.APIKey, httpReferer: c.HTTPReferer, appTitle: c.AppTitle, temperature: c.Temperature, topP: c.TopP, contextWindow: c.ContextWindow, maxTokens: c.MaxTokens, http: &http.Client{Timeout: t}}
}
func (c *Client) Model() string                        { return c.model }
func (c *Client) Unload(context.Context, string) error { return nil }

const (
	defaultReviewMaxTokens = 4096
	maximumReviewMaxTokens = 32768
	minimumOutputReserve   = 256
)

func (c *Client) effectiveMaxTokens(prompt string, requestedMaxTokens int) (int, error) {
	maxTokens := c.maxTokens
	if requestedMaxTokens > 0 {
		maxTokens = requestedMaxTokens
	}
	if maxTokens <= 0 || (c.contextWindow > 0 && maxTokens >= c.contextWindow) {
		maxTokens = defaultReviewMaxTokens
	}
	// Protect installations created before max_completion_tokens was separated
	// from the operational review limit, including rows without context metadata.
	if maxTokens > maximumReviewMaxTokens {
		maxTokens = maximumReviewMaxTokens
	}
	if c.contextWindow <= 0 {
		return maxTokens, nil
	}
	// A byte is a conservative upper bound for one tokenizer token. The extra
	// reserve covers chat-message framing and provider-side metadata.
	estimatedInputTokens := len([]byte(prompt)) + 1024
	available := c.contextWindow - estimatedInputTokens
	if available < minimumOutputReserve {
		return 0, fmt.Errorf("OpenRouter prompt does not fit model context: estimated_input_tokens=%d context_window=%d", estimatedInputTokens, c.contextWindow)
	}
	if maxTokens > available {
		maxTokens = available
	}
	return maxTokens, nil
}

func (c *Client) Chat(ctx context.Context, prompt string) (string, error) {
	result, err := c.chatWithMetadata(ctx, prompt, 0)
	return result.Content, err
}

func (c *Client) ChatWithMaxTokens(ctx context.Context, prompt string, maxOutputTokens int) (ai.ChatResult, error) {
	return c.chatWithMetadata(ctx, prompt, maxOutputTokens)
}

func (c *Client) ChatWithMetadata(ctx context.Context, prompt string) (ai.ChatResult, error) {
	return c.chatWithMetadata(ctx, prompt, 0)
}

func (c *Client) chatWithMetadata(ctx context.Context, prompt string, requestedMaxTokens int) (ai.ChatResult, error) {
	if c.key == "" {
		return ai.ChatResult{}, fmt.Errorf("OpenRouter API key is empty")
	}
	maxTokens, err := c.effectiveMaxTokens(prompt, requestedMaxTokens)
	if err != nil {
		return ai.ChatResult{}, err
	}
	payload := map[string]any{"model": c.model, "messages": []map[string]string{{"role": "user", "content": prompt}}, "temperature": c.temperature, "top_p": c.topP, "max_tokens": maxTokens}
	b, _ := json.Marshal(payload)
	r, e := http.NewRequestWithContext(ctx, http.MethodPost, c.url+"/chat/completions", bytes.NewReader(b))
	if e != nil {
		return ai.ChatResult{}, e
	}
	r.Header.Set("Authorization", "Bearer "+c.key)
	r.Header.Set("Content-Type", "application/json")
	if c.httpReferer != "" {
		r.Header.Set("HTTP-Referer", c.httpReferer)
	}
	if c.appTitle != "" {
		r.Header.Set("X-OpenRouter-Title", c.appTitle)
	}
	res, e := c.http.Do(r)
	if e != nil {
		return ai.ChatResult{}, e
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		x, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return ai.ChatResult{}, fmt.Errorf("OpenRouter status=%d: %s", res.StatusCode, strings.TrimSpace(string(x)))
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if e = json.NewDecoder(res.Body).Decode(&out); e != nil {
		return ai.ChatResult{}, e
	}
	if len(out.Choices) == 0 || out.Choices[0].Message.Content == "" {
		return ai.ChatResult{}, fmt.Errorf("OpenRouter returned an empty response")
	}
	return ai.ChatResult{Content: out.Choices[0].Message.Content}, nil
}
