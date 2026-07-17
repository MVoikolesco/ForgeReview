package ollama

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

type Options struct {
	Temperature   float64 `json:"temperature"`
	TopP          float64 `json:"top_p"`
	RepeatPenalty float64 `json:"repeat_penalty"`
	NumCtx        int     `json:"num_ctx"`
	NumThread     int     `json:"num_thread,omitempty"`
	NumPredict    int     `json:"num_predict"`
}

type Config struct {
	URL            string
	Model          string
	APIKey         string
	Options        Options
	KeepAlive      string
	TimeoutSeconds int
}

type Client struct {
	baseURL    string
	httpClient *http.Client
	model      string
	options    Options
	keepAlive  string
	apiKey     string
}

var _ ai.Client = (*Client)(nil)

func NewClient(cfg Config) *Client {
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 900 * time.Second
	}

	return &Client{
		baseURL: strings.TrimRight(cfg.URL, "/"),
		httpClient: &http.Client{
			Timeout: timeout,
		},
		model:     cfg.Model,
		options:   cfg.Options,
		keepAlive: cfg.KeepAlive,
		apiKey:    cfg.APIKey,
	}
}

func (c *Client) Model() string {
	return c.model
}
func (c *Client) Unload(ctx context.Context, model string) error {
	if c.apiKey != "" {
		return nil
	}
	payload, err := json.Marshal(map[string]any{"model": model, "keep_alive": 0})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/generate", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	c.authorize(req)
	res, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("erro ao descarregar modelo ollama: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("ollama unload retornou status=%d", res.StatusCode)
	}
	return nil
}

func (c *Client) Chat(ctx context.Context, prompt string) (string, error) {
	result, err := c.ChatWithMetadata(ctx, prompt)
	if err != nil {
		return "", err
	}
	return result.Content, nil
}

func (c *Client) ChatWithMaxTokens(ctx context.Context, prompt string, maxOutputTokens int) (ChatResult, error) {
	options := c.options
	if maxOutputTokens > 0 {
		options.NumPredict = maxOutputTokens
	}
	return c.chatWithOptions(ctx, prompt, options)
}

type ChatResult = ai.ChatResult

func (c *Client) ChatWithMetadata(ctx context.Context, prompt string) (ChatResult, error) {
	return c.chatWithOptions(ctx, prompt, c.options)
}

func (c *Client) chatWithOptions(ctx context.Context, prompt string, options Options) (ChatResult, error) {
	payload := chatRequest{
		Model:     c.model,
		Stream:    false,
		KeepAlive: c.keepAlive,
		Messages: []chatMessage{
			{Role: "user", Content: prompt},
		},
		Options: options,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return ChatResult{}, fmt.Errorf("erro ao serializar payload ollama: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return ChatResult{}, fmt.Errorf("erro ao criar request ollama: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	c.authorize(req)

	res, err := c.httpClient.Do(req)
	if err != nil {
		return ChatResult{}, fmt.Errorf("erro ao chamar ollama: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
		errorBody, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return ChatResult{}, fmt.Errorf("ollama retornou status=%d body=%s", res.StatusCode, strings.TrimSpace(string(errorBody)))
	}

	var chatResponse chatResponse
	if err := json.NewDecoder(res.Body).Decode(&chatResponse); err != nil {
		return ChatResult{}, fmt.Errorf("erro ao ler resposta ollama: %w", err)
	}

	if chatResponse.Message.Content == "" {
		return ChatResult{}, fmt.Errorf("ollama retornou resposta sem message.content")
	}

	return ChatResult{
		Content:          chatResponse.Message.Content,
		PromptTokens:     chatResponse.PromptEvalCount,
		CompletionTokens: chatResponse.EvalCount,
	}, nil
}

func (c *Client) authorize(req *http.Request) {
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
}

type chatRequest struct {
	Model     string        `json:"model"`
	Stream    bool          `json:"stream"`
	KeepAlive string        `json:"keep_alive,omitempty"`
	Messages  []chatMessage `json:"messages"`
	Options   Options       `json:"options"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Message         chatMessage `json:"message"`
	PromptEvalCount int         `json:"prompt_eval_count"`
	EvalCount       int         `json:"eval_count"`
}
