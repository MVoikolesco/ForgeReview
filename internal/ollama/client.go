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
}

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
	}
}

func (c *Client) Model() string {
	return c.model
}

func (c *Client) Chat(ctx context.Context, prompt string) (string, error) {
	payload := chatRequest{
		Model:     c.model,
		Stream:    false,
		KeepAlive: c.keepAlive,
		Messages: []chatMessage{
			{Role: "user", Content: prompt},
		},
		Options: c.options,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("erro ao serializar payload ollama: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("erro ao criar request ollama: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("erro ao chamar ollama: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
		errorBody, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return "", fmt.Errorf("ollama retornou status=%d body=%s", res.StatusCode, strings.TrimSpace(string(errorBody)))
	}

	var chatResponse chatResponse
	if err := json.NewDecoder(res.Body).Decode(&chatResponse); err != nil {
		return "", fmt.Errorf("erro ao ler resposta ollama: %w", err)
	}

	if chatResponse.Message.Content == "" {
		return "", fmt.Errorf("ollama retornou resposta sem message.content")
	}

	return chatResponse.Message.Content, nil
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
	Message chatMessage `json:"message"`
}
