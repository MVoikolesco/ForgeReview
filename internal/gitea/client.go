package gitea

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

type Organization struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	FullName string `json:"full_name"`
}
type Repository struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	Owner    struct {
		Login string `json:"login"`
	} `json:"owner"`
	Private bool `json:"private"`
}

type PullReview struct {
	ID    int64  `json:"id"`
	State string `json:"state"`
}

type CreatePullReviewOptions struct {
	Body     string                    `json:"body,omitempty"`
	Comments []CreatePullReviewComment `json:"comments,omitempty"`
	CommitID string                    `json:"commit_id,omitempty"`
	Event    string                    `json:"event"`
}

type CreatePullReviewComment struct {
	Body        string `json:"body"`
	NewPosition int    `json:"new_position,omitempty"`
	Path        string `json:"path"`
}

func NewClient(baseURL string, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) doJSON(ctx context.Context, path string, result any) error {
	if c.baseURL == "" {
		return fmt.Errorf("gitea url is empty")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1"+path, nil)
	if err != nil {
		return err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "token "+c.token)
	}
	req.Header.Set("Accept", "application/json")
	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("gitea request failed: status=%d", res.StatusCode)
	}
	return json.NewDecoder(res.Body).Decode(result)
}

func (c *Client) TestConnection(ctx context.Context) error {
	var user map[string]any
	return c.doJSON(ctx, "/user", &user)
}
func (c *Client) ListOrganizations(ctx context.Context) ([]Organization, error) {
	var result []Organization
	return result, c.doJSON(ctx, "/user/orgs?limit=50", &result)
}
func (c *Client) ListRepositories(ctx context.Context, organization string) ([]Repository, error) {
	path := "/user/repos?limit=50"
	if organization != "" {
		path = "/orgs/" + url.PathEscape(organization) + "/repos?limit=50"
	}
	var result []Repository
	return result, c.doJSON(ctx, path, &result)
}

func (c *Client) GetPullRequestDiff(ctx context.Context, owner string, repo string, prNumber int) (string, error) {
	if c.baseURL == "" {
		return "", fmt.Errorf("gitea url is empty")
	}

	endpoint := fmt.Sprintf(
		"%s/api/v1/repos/%s/%s/pulls/%d.diff",
		c.baseURL,
		url.PathEscape(owner),
		url.PathEscape(repo),
		prNumber,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}

	if c.token != "" {
		req.Header.Set("Authorization", "token "+c.token)
	}
	req.Header.Set("Accept", "text/plain")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}

	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("gitea diff request failed: status=%d", res.StatusCode)
	}

	return string(body), nil
}

func (c *Client) CreatePullRequestReview(ctx context.Context, owner string, repo string, prNumber int, options CreatePullReviewOptions) (PullReview, error) {
	if c.baseURL == "" {
		return PullReview{}, fmt.Errorf("gitea url is empty")
	}

	endpoint := fmt.Sprintf(
		"%s/api/v1/repos/%s/%s/pulls/%d/reviews",
		c.baseURL,
		url.PathEscape(owner),
		url.PathEscape(repo),
		prNumber,
	)

	payload, err := json.Marshal(options)
	if err != nil {
		return PullReview{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return PullReview{}, err
	}

	if c.token != "" {
		req.Header.Set("Authorization", "token "+c.token)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return PullReview{}, err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return PullReview{}, err
	}

	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
		return PullReview{}, fmt.Errorf("gitea create pull review request failed: status=%d body=%s", res.StatusCode, strings.TrimSpace(string(body)))
	}

	var review PullReview
	if len(body) > 0 {
		if err := json.Unmarshal(body, &review); err != nil {
			return PullReview{}, err
		}
	}

	return review, nil
}
