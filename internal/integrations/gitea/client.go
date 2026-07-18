package gitea

import (
	"context"
	"encoding/json"
	"fmt"
	"gitea-agents/internal/contracts"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	BaseURL, Token string
	HTTP           *http.Client
}

func New(baseURL, token string) *Client {
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), Token: token, HTTP: &http.Client{Timeout: 30 * time.Second}}
}
func (c *Client) do(ctx context.Context, method, path string, body any) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		reader = strings.NewReader(string(data))
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "token "+c.Token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("gitea request failed with status %d", resp.StatusCode)
	}
	return data, nil
}
func (c *Client) PullRequestDiff(ctx context.Context, owner, repo string, number int) (string, error) {
	data, err := c.do(ctx, http.MethodGet, "/api/v1/repos/"+url.PathEscape(owner)+"/"+url.PathEscape(repo)+"/pulls/"+strconv.Itoa(number)+".diff", nil)
	return string(data), err
}
func (c *Client) Publish(ctx context.Context, owner, repo string, number int, result contracts.Result) error {
	comments := make([]map[string]any, 0, len(result.Comments))
	for _, item := range result.Comments {
		comments = append(comments, map[string]any{"path": item.File, "body": item.Comment, "new_position": item.Line})
	}
	event := result.FinalReview.GiteaEvent
	if event == "" {
		event = "COMMENT"
	}
	_, err := c.do(ctx, http.MethodPost, "/api/v1/repos/"+url.PathEscape(owner)+"/"+url.PathEscape(repo)+"/pulls/"+strconv.Itoa(number)+"/reviews", map[string]any{"body": result.FinalReview.Summary, "event": event, "comments": comments})
	return err
}
