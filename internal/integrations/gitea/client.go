package gitea

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"gitea-agents/internal/contracts"
)

// HTTPStatusError means Gitea answered and definitively rejected the request.
type HTTPStatusError struct {
	StatusCode int
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("gitea request failed with status %d", e.StatusCode)
}

// IsHTTPStatusError distinguishes a definitive HTTP rejection from an
// ambiguous transport failure that may have reached Gitea.
func IsHTTPStatusError(err error) bool {
	var target *HTTPStatusError
	return errors.As(err, &target)
}

// IsDefinitiveHTTPRejection reports client errors for which Gitea definitively
// rejected the request. Server/proxy errors remain ambiguous.
func IsDefinitiveHTTPRejection(err error) bool {
	var target *HTTPStatusError
	return errors.As(err, &target) && target.StatusCode >= 400 && target.StatusCode < 500
}

// Client performs authenticated requests against one Gitea API instance.
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

// Organization is the subset of a Gitea organization used by the admin UI.
type Organization struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	FullName string `json:"full_name"`
}

// Repository is the subset of a Gitea repository used by the admin UI.
type Repository struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	Owner    struct {
		Login string `json:"login"`
	} `json:"owner"`
	Private bool `json:"private"`
}

// PullRequest is the subset of an open Gitea pull request shown by the admin UI.
type PullRequest struct {
	Number  int    `json:"number"`
	Title   string `json:"title"`
	Body    string `json:"body"`
	State   string `json:"state"`
	HTMLURL string `json:"html_url"`
	User    struct {
		Login string `json:"login"`
	} `json:"user"`
}

// New returns a Gitea client configured with a normalized base URL, token, and
// a 30-second HTTP timeout.
func New(baseURL, token string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   token,
		HTTP:    &http.Client{Timeout: 30 * time.Second},
	}
}

// do sends one Gitea request and returns its body for any successful 2xx status.
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

	response, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	data, _ := io.ReadAll(response.Body)
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, &HTTPStatusError{StatusCode: response.StatusCode}
	}

	return data, nil
}

// PullRequestDiff returns the unified diff for owner/repo pull request number.
func (c *Client) PullRequestDiff(ctx context.Context, owner, repo string, number int) (string, error) {
	path := "/api/v1/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo) +
		"/pulls/" + strconv.Itoa(number) + ".diff"
	data, err := c.do(ctx, http.MethodGet, path, nil)
	return string(data), err
}

// jsonGET performs a Gitea API GET and decodes its JSON response into target.
func (c *Client) jsonGET(ctx context.Context, path string, target any) error {
	data, err := c.do(ctx, http.MethodGet, "/api/v1"+path, nil)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

// TestConnection verifies credentials by requesting the authenticated user.
func (c *Client) TestConnection(ctx context.Context) error {
	var user map[string]any
	return c.jsonGET(ctx, "/user", &user)
}

// ListOrganizations returns up to 50 organizations visible to the client token.
func (c *Client) ListOrganizations(ctx context.Context) ([]Organization, error) {
	var result []Organization
	err := c.jsonGET(ctx, "/user/orgs?limit=50", &result)
	return result, err
}

// ListRepositories returns up to 50 user repositories or organization
// repositories when organization is non-empty.
func (c *Client) ListRepositories(ctx context.Context, organization string) ([]Repository, error) {
	path := "/user/repos?limit=50"
	if organization != "" {
		path = "/orgs/" + url.PathEscape(organization) + "/repos?limit=50"
	}

	var result []Repository
	err := c.jsonGET(ctx, path, &result)
	return result, err
}

// ListPullRequests returns up to 50 open pull requests for owner/repo.
func (c *Client) ListPullRequests(ctx context.Context, owner, repo string) ([]PullRequest, error) {
	path := "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo) +
		"/pulls?state=open&limit=50"
	var result []PullRequest
	err := c.jsonGET(ctx, path, &result)
	return result, err
}

// Publish creates a Gitea pull request review from the provider-independent
// result contract.
func (c *Client) Publish(
	ctx context.Context,
	owner string,
	repo string,
	number int,
	result contracts.Result,
) error {
	comments := make([]map[string]any, 0, len(result.Comments))
	for _, item := range result.Comments {
		comments = append(comments, map[string]any{
			"path":         item.File,
			"body":         item.Comment,
			"new_position": item.Line,
		})
	}

	event := result.FinalReview.GiteaEvent
	if event == "" {
		event = "COMMENT"
	}
	payload := map[string]any{
		"body":     publicationBody(result) + publicationMarker(result.ReviewID),
		"event":    event,
		"comments": comments,
	}
	path := "/api/v1/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo) +
		"/pulls/" + strconv.Itoa(number) + "/reviews"
	_, err := c.do(ctx, http.MethodPost, path, payload)
	return err
}

// publicationBody preserves the operator-facing envelope used by the former
// review pipeline while keeping the new structured result as its source.
func publicationBody(result contracts.Result) string {
	var b strings.Builder
	if status := strings.TrimSpace(result.FinalReview.Status); status != "" {
		fmt.Fprintf(&b, "> status: %s\n", status)
	}
	if elapsed := metadataDuration(result.Metadata); elapsed != "" || result.Model != "" {
		promptTokens := metadataTokenSum(result.Metadata, "actual_prompt_tokens")
		completionTokens := metadataTokenSum(result.Metadata, "actual_completion_tokens")
		fmt.Fprintf(&b, "> elapsed time: %s\n> model: %s\n> tokens: %d (prompt: %d, completion: %d)\n\n", elapsed, result.Model, promptTokens+completionTokens, promptTokens, completionTokens)
	}
	if summary := strings.TrimSpace(result.FinalReview.Summary); summary != "" {
		b.WriteString(summary)
	}
	if observations := strings.TrimSpace(result.FinalReview.Observations); observations != "" {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(observations)
	}
	if b.Len() == 0 {
		b.WriteString("Review automatizado concluído.")
	}
	return b.String()
}

func metadataDuration(metadata map[string]any) string {
	ms := metadataInt(metadata["total_duration_ms"])
	if ms <= 0 {
		return ""
	}
	return fmt.Sprintf("%.3fs", float64(ms)/1000)
}

func metadataTokenSum(metadata map[string]any, key string) int {
	total := 0
	switch metrics := metadata["stage_metrics"].(type) {
	case []map[string]any:
		for _, metric := range metrics {
			total += metadataInt(metric[key])
		}
	case []any:
		for _, item := range metrics {
			if metric, ok := item.(map[string]any); ok {
				total += metadataInt(metric[key])
			}
		}
	}
	return total
}

func metadataInt(value any) int {
	switch number := value.(type) {
	case int:
		return number
	case int64:
		return int(number)
	case float64:
		return int(number)
	case string:
		parsed, _ := strconv.Atoi(number)
		return parsed
	default:
		return 0
	}
}

// HasPublishedReview reconciles an uncertain local publication with reviews
// already visible in Gitea.
func (c *Client) HasPublishedReview(ctx context.Context, owner, repo string, number int, reviewID string) (bool, error) {
	marker := publicationMarker(reviewID)
	basePath := "/api/v1/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo) +
		"/pulls/" + strconv.Itoa(number) + "/reviews"
	for page := 1; ; page++ {
		data, err := c.do(ctx, http.MethodGet, basePath+"?limit=50&page="+strconv.Itoa(page), nil)
		if err != nil {
			return false, err
		}
		var reviews []struct {
			Body string `json:"body"`
		}
		if err := json.Unmarshal(data, &reviews); err != nil {
			return false, err
		}
		for _, review := range reviews {
			if strings.Contains(review.Body, marker) {
				return true, nil
			}
		}
		if len(reviews) < 50 {
			return false, nil
		}
	}
}

func publicationMarker(reviewID string) string {
	if reviewID == "" {
		sum := sha256.Sum256(nil)
		reviewID = fmt.Sprintf("anonymous-%x", sum[:8])
	}
	return "\n\n<!-- forgereview:" + reviewID + " -->"
}
