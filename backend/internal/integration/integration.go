// Package integration defines controlled external provider contracts.
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

const (
	TypeGitea  = "gitea"
	TypeOpenAI = "openai"
	TypeOllama = "ollama"

	StatusActive   = "active"
	StatusDisabled = "disabled"
)

var environmentName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Integration contains only a reference to the credential, never its value.
type Integration struct {
	Key             string          `json:"key"`
	Name            string          `json:"name"`
	Type            string          `json:"type"`
	Config          json.RawMessage `json:"config"`
	SecretReference string          `json:"secret_reference"`
	Status          string          `json:"status"`
}

// Summary is safe to expose through the HTTP API.
type Summary struct {
	Key              string          `json:"key"`
	Name             string          `json:"name"`
	Type             string          `json:"type"`
	Config           json.RawMessage `json:"config"`
	SecretConfigured bool            `json:"secret_configured"`
	Status           string          `json:"status"`
}

func (i Integration) Summary() Summary {
	return Summary{Key: i.Key, Name: i.Name, Type: i.Type, Config: i.Config, SecretConfigured: i.SecretReference != "", Status: i.Status}
}

// Validate limits configuration to transport settings so credentials cannot be
// smuggled into the persisted JSON configuration.
func (i Integration) Validate() error {
	if strings.TrimSpace(i.Key) == "" || strings.TrimSpace(i.Name) == "" {
		return fmt.Errorf("integration key and name are required")
	}
	if i.Type != TypeGitea && i.Type != TypeOpenAI && i.Type != TypeOllama {
		return fmt.Errorf("unsupported integration type %q", i.Type)
	}
	if i.Status != StatusActive && i.Status != StatusDisabled {
		return fmt.Errorf("integration status must be %q or %q", StatusActive, StatusDisabled)
	}
	if !environmentName.MatchString(i.SecretReference) {
		return fmt.Errorf("integration secret_reference must name an environment variable")
	}
	var config map[string]string
	if err := json.Unmarshal(i.Config, &config); err != nil {
		return fmt.Errorf("integration config must be a JSON object: %w", err)
	}
	if config == nil {
		return fmt.Errorf("integration config must be a JSON object")
	}
	allowed := map[string]bool{"base_url": true}
	if i.Type == TypeOpenAI || i.Type == TypeOllama {
		allowed["model"] = true
	}
	for key := range config {
		if !allowed[key] {
			return fmt.Errorf("integration config field %q is not allowed", key)
		}
	}
	baseURL := config["base_url"]
	parsed, err := url.ParseRequestURI(baseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("integration config.base_url must be an absolute URL without credentials")
	}
	if (i.Type == TypeOpenAI || i.Type == TypeOllama) && strings.TrimSpace(config["model"]) == "" {
		return fmt.Errorf("integration config.model is required")
	}
	return nil
}

func (i Integration) ConfigValues() (map[string]string, error) {
	var config map[string]string
	if err := json.Unmarshal(i.Config, &config); err != nil {
		return nil, fmt.Errorf("decode integration config: %w", err)
	}
	return config, nil
}

type Lookup interface {
	Integration(context.Context, string) (Integration, error)
}

type PullRequestRequest struct {
	Owner  string
	Repo   string
	Number int
}

type PullRequest struct {
	Metadata map[string]any   `json:"metadata"`
	Diff     string           `json:"diff"`
	Files    []map[string]any `json:"files"`
}

type GiteaPullRequestReader interface {
	ReadPullRequest(context.Context, Integration, string, PullRequestRequest) (PullRequest, error)
}

// GiteaReviewWriter is the controlled boundary for creating a pull-request
// review comment. Callers supply only a registered Gitea integration and the
// resolved credential; adapters must not persist or return the credential.
type GiteaReviewWriter interface {
	PublishReview(context.Context, Integration, string, GiteaReviewRequest) (PublicationReceipt, error)
}

type GiteaReviewRequest struct {
	Owner          string
	Repo           string
	Number         int
	Body           string
	IdempotencyKey string
}

// PublicationReceipt contains safe provider response identifiers only.
type PublicationReceipt struct {
	Status         string `json:"status"`
	CommentID      int64  `json:"comment_id,omitempty"`
	URL            string `json:"url,omitempty"`
	IdempotencyKey string `json:"idempotency_key"`
}

type ChatClient interface {
	Chat(context.Context, Integration, string, string) (string, error)
}
