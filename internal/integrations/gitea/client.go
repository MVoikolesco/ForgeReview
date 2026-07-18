package gitea

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"gitea-agents/internal/contracts"
	"gitea-agents/internal/security"
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

func (c *Client) jsonGET(ctx context.Context, path string, target any) error {
	data, err := c.do(ctx, http.MethodGet, "/api/v1"+path, nil)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func (c *Client) TestConnection(ctx context.Context) error {
	var user map[string]any
	return c.jsonGET(ctx, "/user", &user)
}

func (c *Client) ListOrganizations(ctx context.Context) ([]Organization, error) {
	var result []Organization
	err := c.jsonGET(ctx, "/user/orgs?limit=50", &result)
	return result, err
}

func (c *Client) ListRepositories(ctx context.Context, organization string) ([]Repository, error) {
	path := "/user/repos?limit=50"
	if organization != "" {
		path = "/orgs/" + url.PathEscape(organization) + "/repos?limit=50"
	}
	var result []Repository
	err := c.jsonGET(ctx, path, &result)
	return result, err
}

func (c *Client) ListPullRequests(ctx context.Context, owner, repo string) ([]PullRequest, error) {
	var result []PullRequest
	path := "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo) + "/pulls?state=open&limit=50"
	err := c.jsonGET(ctx, path, &result)
	return result, err
}

type Resolver struct {
	db       *sql.DB
	fallback *Client
}

func NewResolver(db *sql.DB, fallback *Client) *Resolver {
	return &Resolver{db: db, fallback: fallback}
}

func (r *Resolver) Resolve(ctx context.Context, instanceID int64, owner, repo string) (*Client, error) {
	if r.db == nil {
		return r.fallback, nil
	}
	if instanceID == 0 {
		err := r.db.QueryRowContext(ctx, `SELECT gi.id FROM gitea_instances gi JOIN repositories rep ON rep.gitea_instance_id=gi.id WHERE rep.owner=? AND rep.name=? AND gi.is_enabled=1 ORDER BY rep.id LIMIT 1`, owner, repo).Scan(&instanceID)
		if err == sql.ErrNoRows {
			err = r.db.QueryRowContext(ctx, `SELECT id FROM gitea_instances WHERE is_enabled=1 ORDER BY is_default DESC, id LIMIT 1`).Scan(&instanceID)
		}
		if err == sql.ErrNoRows {
			return r.fallback, nil
		}
		if err != nil {
			return nil, err
		}
	}
	var baseURL, ciphertext string
	if err := r.db.QueryRowContext(ctx, `SELECT base_url,token_ciphertext FROM gitea_instances WHERE id=? AND is_enabled=1`, instanceID).Scan(&baseURL, &ciphertext); err != nil {
		return nil, fmt.Errorf("Gitea instance %d not found: %w", instanceID, err)
	}
	token, err := security.Decrypt(ciphertext)
	if err != nil || token == "" {
		return nil, fmt.Errorf("Gitea instance %d has no usable token", instanceID)
	}
	return New(baseURL, token), nil
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
