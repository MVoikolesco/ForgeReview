package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type HTTPGiteaClient struct{ Client *http.Client }
type HTTPOpenAIClient struct{ Client *http.Client }
type HTTPOllamaClient struct{ Client *http.Client }

func (c HTTPGiteaClient) ReadPullRequest(ctx context.Context, integration Integration, secret string, request PullRequestRequest) (PullRequest, error) {
	if integration.Type != TypeGitea {
		return PullRequest{}, fmt.Errorf("integration %q is not a Gitea integration", integration.Key)
	}
	config, err := integration.ConfigValues()
	if err != nil {
		return PullRequest{}, err
	}
	base := strings.TrimRight(config["base_url"], "/")
	path := fmt.Sprintf("%s/api/v1/repos/%s/%s/pulls/%d", base, url.PathEscape(request.Owner), url.PathEscape(request.Repo), request.Number)
	metadata := map[string]any{}
	if err := c.getJSON(ctx, path, secret, &metadata); err != nil {
		return PullRequest{}, fmt.Errorf("read Gitea pull request metadata: %w", err)
	}
	var files []map[string]any
	if err := c.getJSON(ctx, path+"/files", secret, &files); err != nil {
		return PullRequest{}, fmt.Errorf("read Gitea pull request files: %w", err)
	}
	diff, err := c.getText(ctx, path+".diff", secret)
	if err != nil {
		return PullRequest{}, fmt.Errorf("read Gitea pull request diff: %w", err)
	}
	return PullRequest{Metadata: metadata, Files: files, Diff: diff}, nil
}

func (c HTTPGiteaClient) PublishReview(ctx context.Context, integration Integration, secret string, request GiteaReviewRequest) (PublicationReceipt, error) {
	if integration.Type != TypeGitea {
		return PublicationReceipt{}, fmt.Errorf("integration %q is not a Gitea integration", integration.Key)
	}
	if request.Owner == "" || request.Repo == "" || request.Number < 1 || request.Body == "" || request.IdempotencyKey == "" {
		return PublicationReceipt{}, fmt.Errorf("Gitea review request is incomplete")
	}
	config, err := integration.ConfigValues()
	if err != nil {
		return PublicationReceipt{}, err
	}
	endpoint := fmt.Sprintf("%s/api/v1/repos/%s/%s/issues/%d/comments", strings.TrimRight(config["base_url"], "/"), url.PathEscape(request.Owner), url.PathEscape(request.Repo), request.Number)
	payload := map[string]string{"body": request.Body + "\n\n<!-- forgereview:idempotency=" + request.IdempotencyKey + " -->"}
	body, err := json.Marshal(payload)
	if err != nil {
		return PublicationReceipt{}, err
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return PublicationReceipt{}, err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json")
	httpRequest.Header.Set("Authorization", "token "+secret)
	httpRequest.Header.Set("X-ForgeReview-Idempotency-Key", request.IdempotencyKey)
	response, err := c.client().Do(httpRequest)
	if err != nil {
		return PublicationReceipt{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return PublicationReceipt{}, fmt.Errorf("unexpected status %d", response.StatusCode)
	}
	var published struct {
		ID      int64  `json:"id"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(response.Body).Decode(&published); err != nil {
		return PublicationReceipt{}, err
	}
	return PublicationReceipt{Status: "completed", CommentID: published.ID, URL: published.HTMLURL, IdempotencyKey: request.IdempotencyKey}, nil
}

func (c HTTPGiteaClient) getJSON(ctx context.Context, endpoint, secret string, target any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "token "+secret)
	response, err := c.client().Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("unexpected status %d", response.StatusCode)
	}
	return json.NewDecoder(response.Body).Decode(target)
}

func (c HTTPGiteaClient) getText(ctx context.Context, endpoint, secret string) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("Accept", "text/plain")
	request.Header.Set("Authorization", "token "+secret)
	response, err := c.client().Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("unexpected status %d", response.StatusCode)
	}
	body, err := io.ReadAll(response.Body)
	return string(body), err
}

func (c HTTPGiteaClient) client() *http.Client {
	if c.Client != nil {
		return c.Client
	}
	return http.DefaultClient
}

func (c HTTPOpenAIClient) Chat(ctx context.Context, integration Integration, secret, prompt string) (string, error) {
	if integration.Type != TypeOpenAI {
		return "", fmt.Errorf("integration %q is not an OpenAI-compatible integration", integration.Key)
	}
	config, err := integration.ConfigValues()
	if err != nil {
		return "", err
	}
	payload := map[string]any{"model": config["model"], "messages": []map[string]string{{"role": "user", "content": prompt}}}
	var response struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err = postJSON(ctx, c.Client, strings.TrimRight(config["base_url"], "/")+"/v1/chat/completions", payload, "Bearer "+secret, &response); err != nil {
		return "", fmt.Errorf("OpenAI-compatible chat: %w", err)
	}
	if len(response.Choices) == 0 || response.Choices[0].Message.Content == "" {
		return "", fmt.Errorf("OpenAI-compatible chat returned no message")
	}
	return response.Choices[0].Message.Content, nil
}

func (c HTTPOllamaClient) Chat(ctx context.Context, integration Integration, secret, prompt string) (string, error) {
	if integration.Type != TypeOllama {
		return "", fmt.Errorf("integration %q is not an Ollama integration", integration.Key)
	}
	config, err := integration.ConfigValues()
	if err != nil {
		return "", err
	}
	payload := map[string]any{"model": config["model"], "messages": []map[string]string{{"role": "user", "content": prompt}}, "stream": false}
	var response struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err = postJSON(ctx, c.Client, strings.TrimRight(config["base_url"], "/")+"/api/chat", payload, "Bearer "+secret, &response); err != nil {
		return "", fmt.Errorf("Ollama chat: %w", err)
	}
	if response.Message.Content == "" {
		return "", fmt.Errorf("Ollama chat returned no message")
	}
	return response.Message.Content, nil
}

func postJSON(ctx context.Context, client *http.Client, endpoint string, payload any, authorization string, target any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("unexpected status %d", response.StatusCode)
	}
	return json.NewDecoder(response.Body).Decode(target)
}
