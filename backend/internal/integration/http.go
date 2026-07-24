package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type HTTPGiteaClient struct{ Client *http.Client }
type HTTPOpenAIClient struct{ Client *http.Client }
type HTTPOllamaClient struct{ Client *http.Client }

// FailureClass is deliberately small and safe to persist as execution metadata.
// It never contains an endpoint, request body, or provider response.
type FailureClass string

const (
	FailureTransient FailureClass = "transient"
	FailurePermanent FailureClass = "permanent"
	FailureUncertain FailureClass = "uncertain"
)

type HTTPStatusError struct{ StatusCode int }

func (e HTTPStatusError) Error() string {
	return fmt.Sprintf("provider returned status %d", e.StatusCode)
}

// ClassifyFailure distinguishes retry-safe failures from failures where a
// provider may already have accepted an externally visible request.
func ClassifyFailure(err error) FailureClass {
	if err == nil {
		return FailurePermanent
	}
	var status HTTPStatusError
	if errors.As(err, &status) {
		if status.StatusCode == http.StatusTooManyRequests || status.StatusCode >= 500 {
			return FailureTransient
		}
		return FailurePermanent
	}
	var networkErr net.Error
	if errors.As(err, &networkErr) {
		return FailureTransient
	}
	var publication PublicationError
	if errors.As(err, &publication) && publication.Uncertain {
		return FailureUncertain
	}
	return FailurePermanent
}

const outboundTimeout = 10 * time.Second

// HTTPDiscoveryAdapter performs the setup-only provider calls with a dedicated
// bounded client. It is intentionally separate from workflow execution clients.
type HTTPDiscoveryAdapter struct{ Client *http.Client }

func (a HTTPDiscoveryAdapter) Validate(ctx context.Context, item Integration, secret string) error {
	if item.Type == TypeGitea {
		var current map[string]any
		return a.getJSON(ctx, strings.TrimRight(configBaseURL(item), "/")+"/api/v1/user", "token "+secret, &current)
	}
	if item.Type == TypeOpenAI {
		var models struct {
			Data []json.RawMessage `json:"data"`
		}
		return a.getJSON(ctx, openAIEndpoint(configBaseURL(item), "models"), "Bearer "+secret, &models)
	}
	if item.Type == TypeOllama {
		var models struct {
			Models []json.RawMessage `json:"models"`
		}
		return a.getJSON(ctx, strings.TrimRight(configBaseURL(item), "/")+"/api/tags", "Bearer "+secret, &models)
	}
	return fmt.Errorf("unsupported integration type")
}

func (a HTTPDiscoveryAdapter) Organizations(ctx context.Context, item Integration, secret string) ([]string, error) {
	if item.Type != TypeGitea {
		return nil, fmt.Errorf("organizations require Gitea")
	}
	var orgs []struct {
		UserName string `json:"username"`
		Name     string `json:"name"`
	}
	if err := a.getJSON(ctx, strings.TrimRight(configBaseURL(item), "/")+"/api/v1/user/orgs", "token "+secret, &orgs); err != nil {
		return nil, err
	}
	organizations := []string{}
	seen := map[string]bool{}
	for _, org := range orgs {
		owner := org.UserName
		if owner == "" {
			owner = org.Name
		}
		if owner == "" {
			continue
		}
		if !seen[owner] {
			seen[owner] = true
			organizations = append(organizations, owner)
		}
	}
	return organizations, nil
}

// Repositories discovers a single organization's repositories. The caller must
// select the organization first; discovery never fans out across every org.
func (a HTTPDiscoveryAdapter) Repositories(ctx context.Context, item Integration, secret, organization string) ([]Repository, error) {
	if item.Type != TypeGitea || strings.TrimSpace(organization) == "" {
		return nil, fmt.Errorf("repositories require a Gitea organization")
	}
	var repos []struct {
		Name  string `json:"name"`
		Owner struct {
			UserName string `json:"username"`
		} `json:"owner"`
	}
	if err := a.getJSON(ctx, fmt.Sprintf("%s/api/v1/orgs/%s/repos", strings.TrimRight(configBaseURL(item), "/"), url.PathEscape(organization)), "token "+secret, &repos); err != nil {
		return nil, err
	}
	repositories := []Repository{}
	for _, repo := range repos {
		if repo.Name == "" {
			continue
		}
		owner := repo.Owner.UserName
		if owner == "" {
			owner = organization
		}
		repositories = append(repositories, Repository{IntegrationKey: item.Key, Owner: owner, Name: repo.Name})
	}
	return repositories, nil
}

func (a HTTPDiscoveryAdapter) Models(ctx context.Context, item Integration, secret string) ([]string, error) {
	var models []string
	if item.Type == TypeOpenAI {
		var response struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := a.getJSON(ctx, openAIEndpoint(configBaseURL(item), "models"), "Bearer "+secret, &response); err != nil {
			return nil, err
		}
		for _, model := range response.Data {
			if model.ID != "" {
				models = append(models, model.ID)
			}
		}
		return models, nil
	}
	if item.Type == TypeOllama {
		var response struct {
			Models []struct {
				Name string `json:"name"`
			} `json:"models"`
		}
		if err := a.getJSON(ctx, strings.TrimRight(configBaseURL(item), "/")+"/api/tags", "Bearer "+secret, &response); err != nil {
			return nil, err
		}
		for _, model := range response.Models {
			if model.Name != "" {
				models = append(models, model.Name)
			}
		}
		return models, nil
	}
	return nil, fmt.Errorf("models require an LLM connection")
}

func (a HTTPDiscoveryAdapter) getJSON(ctx context.Context, endpoint, authorization string, target any) error {
	ctx, cancel := context.WithTimeout(ctx, outboundTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	client := a.Client
	if client == nil {
		client = &http.Client{Timeout: outboundTimeout}
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return HTTPStatusError{StatusCode: response.StatusCode}
	}
	return json.NewDecoder(response.Body).Decode(target)
}

func configBaseURL(item Integration) string {
	config, _ := item.ConfigValues()
	return config["base_url"]
}

// openAIEndpoint accepts either a provider root or an already versioned base
// URL (OpenRouter's https://openrouter.ai/api/v1), preventing a duplicate /v1.
func openAIEndpoint(baseURL, resource string) string {
	baseURL = strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(baseURL, "/v1") {
		return baseURL + "/" + resource
	}
	return baseURL + "/v1/" + resource
}

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
	if len(files) > 0 && attachUnifiedDiffPatches(files, diff) == 0 {
		return PullRequest{}, fmt.Errorf("read Gitea pull request diff: %d changed file(s) have no reviewable patch content", len(files))
	}
	return PullRequest{Metadata: metadata, Files: files, Diff: diff}, nil
}

func (c HTTPGiteaClient) PublishReview(ctx context.Context, integration Integration, secret string, request GiteaReviewRequest) (PublicationReceipt, error) {
	if integration.Type != TypeGitea {
		return PublicationReceipt{}, fmt.Errorf("integration %q is not a Gitea integration", integration.Key)
	}
	if request.Owner == "" || request.Repo == "" || request.Number < 1 || request.Body == "" || request.Event == "" || request.IdempotencyKey == "" {
		return PublicationReceipt{}, fmt.Errorf("Gitea review request is incomplete")
	}
	config, err := integration.ConfigValues()
	if err != nil {
		return PublicationReceipt{}, err
	}
	endpoint := fmt.Sprintf("%s/api/v1/repos/%s/%s/pulls/%d/reviews", strings.TrimRight(config["base_url"], "/"), url.PathEscape(request.Owner), url.PathEscape(request.Repo), request.Number)
	payload := struct {
		Body     string               `json:"body"`
		Event    string               `json:"event"`
		Comments []GiteaReviewComment `json:"comments"`
	}{Body: request.Body + "\n\n<!-- forgereview:idempotency=" + request.IdempotencyKey + " -->", Event: request.Event, Comments: request.Comments}
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
		return PublicationReceipt{}, PublicationError{Uncertain: true}
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return PublicationReceipt{}, PublicationError{Uncertain: response.StatusCode >= 500}
	}
	var published struct {
		ID      int64  `json:"id"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(response.Body).Decode(&published); err != nil {
		return PublicationReceipt{}, PublicationError{Uncertain: true}
	}
	return PublicationReceipt{Status: "completed", CommentID: published.ID, URL: published.HTMLURL, IdempotencyKey: request.IdempotencyKey}, nil
}

func (c HTTPGiteaClient) FindReviewByMarker(ctx context.Context, item Integration, secret string, target PullRequestRequest, key string) (PublicationReceipt, bool, error) {
	config, err := item.ConfigValues()
	if err != nil {
		return PublicationReceipt{}, false, err
	}
	endpoint := fmt.Sprintf("%s/api/v1/repos/%s/%s/pulls/%d/reviews", strings.TrimRight(config["base_url"], "/"), url.PathEscape(target.Owner), url.PathEscape(target.Repo), target.Number)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return PublicationReceipt{}, false, err
	}
	req.Header.Set("Authorization", "token "+secret)
	req.Header.Set("Accept", "application/json")
	response, err := c.client().Do(req)
	if err != nil {
		return PublicationReceipt{}, false, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return PublicationReceipt{}, false, fmt.Errorf("unexpected status %d", response.StatusCode)
	}
	var reviews []struct {
		ID      int64  `json:"id"`
		HTMLURL string `json:"html_url"`
		Body    string `json:"body"`
	}
	if err = json.NewDecoder(response.Body).Decode(&reviews); err != nil {
		return PublicationReceipt{}, false, err
	}
	marker := "<!-- forgereview:idempotency=" + key + " -->"
	for _, review := range reviews {
		if strings.Contains(review.Body, marker) {
			return PublicationReceipt{Status: "completed", CommentID: review.ID, URL: review.HTMLURL, IdempotencyKey: key}, true, nil
		}
	}
	return PublicationReceipt{}, false, nil
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
		return HTTPStatusError{StatusCode: response.StatusCode}
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
		return "", HTTPStatusError{StatusCode: response.StatusCode}
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

func (c HTTPOpenAIClient) Chat(ctx context.Context, integration Integration, secret, prompt string) (ChatResult, error) {
	if integration.Type != TypeOpenAI {
		return ChatResult{}, fmt.Errorf("integration %q is not an OpenAI-compatible integration", integration.Key)
	}
	config, err := integration.ConfigValues()
	if err != nil {
		return ChatResult{}, err
	}
	payload := map[string]any{"model": config["model"], "messages": []map[string]string{{"role": "user", "content": prompt}}}
	if maxTokens, parseErr := strconv.Atoi(config["max_tokens"]); parseErr == nil && maxTokens > 0 {
		payload["max_tokens"] = maxTokens
	}
	if temperature, parseErr := strconv.ParseFloat(config["temperature"], 64); parseErr == nil {
		payload["temperature"] = temperature
	}
	if topP, parseErr := strconv.ParseFloat(config["top_p"], 64); parseErr == nil {
		payload["top_p"] = topP
	}
	var response struct {
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err = postJSON(ctx, c.Client, openAIEndpoint(config["base_url"], "chat/completions"), payload, "Bearer "+secret, &response); err != nil {
		return ChatResult{}, fmt.Errorf("OpenAI-compatible chat: %w", err)
	}
	if len(response.Choices) == 0 || response.Choices[0].Message.Content == "" {
		return ChatResult{}, fmt.Errorf("OpenAI-compatible chat returned no message")
	}
	return ChatResult{Content: response.Choices[0].Message.Content, Model: response.Model, Usage: TokenUsage{Prompt: response.Usage.PromptTokens, Completion: response.Usage.CompletionTokens, Total: response.Usage.TotalTokens}}, nil
}

func (c HTTPOllamaClient) Chat(ctx context.Context, integration Integration, secret, prompt string) (ChatResult, error) {
	if integration.Type != TypeOllama {
		return ChatResult{}, fmt.Errorf("integration %q is not an Ollama integration", integration.Key)
	}
	config, err := integration.ConfigValues()
	if err != nil {
		return ChatResult{}, err
	}
	payload := map[string]any{"model": config["model"], "messages": []map[string]string{{"role": "user", "content": prompt}}, "stream": false}
	options := map[string]any{}
	if maxTokens, parseErr := strconv.Atoi(config["max_tokens"]); parseErr == nil && maxTokens > 0 {
		options["num_predict"] = maxTokens
	}
	if temperature, parseErr := strconv.ParseFloat(config["temperature"], 64); parseErr == nil {
		options["temperature"] = temperature
	}
	if topP, parseErr := strconv.ParseFloat(config["top_p"], 64); parseErr == nil {
		options["top_p"] = topP
	}
	if len(options) > 0 {
		payload["options"] = options
	}
	if keepAlive := strings.TrimSpace(config["keep_alive"]); keepAlive != "" {
		payload["keep_alive"] = keepAlive
	}
	var response struct {
		Model   string `json:"model"`
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		PromptEvalCount int `json:"prompt_eval_count"`
		EvalCount       int `json:"eval_count"`
	}
	if err = postJSON(ctx, c.Client, strings.TrimRight(config["base_url"], "/")+"/api/chat", payload, "Bearer "+secret, &response); err != nil {
		return ChatResult{}, fmt.Errorf("Ollama chat: %w", err)
	}
	if response.Message.Content == "" {
		return ChatResult{}, fmt.Errorf("Ollama chat returned no message")
	}
	usage := TokenUsage{Prompt: response.PromptEvalCount, Completion: response.EvalCount}
	if usage.Prompt > 0 || usage.Completion > 0 {
		usage.Total = usage.Prompt + usage.Completion
	}
	return ChatResult{Content: response.Message.Content, Model: response.Model, Usage: usage}, nil
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
		return HTTPStatusError{StatusCode: response.StatusCode}
	}
	return json.NewDecoder(response.Body).Decode(target)
}
