package webhook

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"gitea-agents/internal/config"
	"gitea-agents/internal/queue"
	"gitea-agents/internal/review"

	"github.com/gin-gonic/gin"
)

type webhookUser struct {
	Login    string `json:"login"`
	Username string `json:"username"`
}

type webhookEvent struct {
	Action            string      `json:"action"`
	Number            int         `json:"number"`
	RequestedReviewer webhookUser `json:"requested_reviewer"`
	PullRequest       struct {
		Number             int           `json:"number"`
		Title              string        `json:"title"`
		Body               string        `json:"body"`
		User               webhookUser   `json:"user"`
		RequestedReviewers []webhookUser `json:"requested_reviewers"`
		Base               struct {
			Ref string `json:"ref"`
		} `json:"base"`
		Head struct {
			Ref string `json:"ref"`
		} `json:"head"`
	} `json:"pull_request"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
	Sender webhookUser `json:"sender"`
}

// WebhookHandler receives public Gitea webhook and legacy manual review
// requests, validates them, and delegates queueing to the review service.
type WebhookHandler struct {
	service *review.Service
	cfg     config.Config
}

// NewWebhookHandler returns a webhook handler backed by service and cfg.
func NewWebhookHandler(service *review.Service, cfg config.Config) *WebhookHandler {
	return &WebhookHandler{service: service, cfg: cfg}
}

// Receive accepts JSON, query-string payload, or form payload webhook events.
// It ignores unrelated events and enqueues review requests addressed to the
// configured Gitea bot.
func (h *WebhookHandler) Receive(c *gin.Context) {
	var event webhookEvent
	if err := bindWebhookEvent(c, &event); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}

	reviewer := event.RequestedReviewer.Login
	if reviewer == "" {
		reviewer = event.RequestedReviewer.Username
	}
	if reviewer != h.cfg.GiteaBotUsername {
		for _, candidate := range event.PullRequest.RequestedReviewers {
			if candidate.Login == h.cfg.GiteaBotUsername || candidate.Username == h.cfg.GiteaBotUsername {
				reviewer = h.cfg.GiteaBotUsername
				break
			}
		}
	}
	if event.Action != "review_requested" || reviewer != h.cfg.GiteaBotUsername {
		c.JSON(http.StatusOK, gin.H{"status": "ignored"})
		return
	}

	owner, repository := splitFullName(event.Repository.FullName)
	pullRequest := event.Number
	if pullRequest == 0 {
		pullRequest = event.PullRequest.Number
	}

	item, err := h.service.Enqueue(c.Request.Context(), queue.ReviewJob{
		Owner:             owner,
		Repository:        repository,
		PullRequest:       pullRequest,
		RequestedReviewer: reviewer,
		Sender:            preferredUsername(event.Sender),
		Title:             event.PullRequest.Title,
		Description:       event.PullRequest.Body,
		Author:            preferredUsername(event.PullRequest.User),
		BaseBranch:        event.PullRequest.Base.Ref,
		HeadBranch:        event.PullRequest.Head.Ref,
	}, "webhook")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to publish job"})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"status": "accepted", "review_id": item.ID})
}

// Manual accepts a pull request URL, parses its repository coordinates, and
// enqueues a manual review.
func (h *WebhookHandler) Manual(c *gin.Context) {
	var body struct {
		URL string `json:"url"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}

	owner, repository, pullRequest, err := parsePullRequestURL(body.URL)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	item, err := h.service.Enqueue(c.Request.Context(), queue.ReviewJob{
		Owner:             owner,
		Repository:        repository,
		PullRequest:       pullRequest,
		RequestedReviewer: h.cfg.GiteaBotUsername,
		Sender:            "manual",
		Manual:            true,
	}, "manual")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to publish job"})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"status": "accepted", "review_id": item.ID})
}

// bindWebhookEvent decodes the legacy payload formats supported by Receive.
func bindWebhookEvent(c *gin.Context, event *webhookEvent) error {
	payload := c.Query("payload")
	if payload == "" && strings.HasPrefix(c.GetHeader("Content-Type"), "application/x-www-form-urlencoded") {
		if err := c.Request.ParseForm(); err != nil {
			return err
		}
		payload = c.PostForm("payload")
	}
	if payload != "" {
		return json.Unmarshal([]byte(payload), event)
	}
	return c.ShouldBindJSON(event)
}

// preferredUsername returns Login when present and otherwise Username.
func preferredUsername(user webhookUser) string {
	if user.Login != "" {
		return user.Login
	}
	return user.Username
}

// splitFullName separates owner/repository and preserves malformed input as the
// repository component for legacy compatibility.
func splitFullName(value string) (string, string) {
	parts := strings.SplitN(strings.Trim(value, "/"), "/", 2)
	if len(parts) != 2 {
		return "", value
	}
	return parts[0], parts[1]
}

// parsePullRequestURL validates and extracts owner, repository, and pull request
// number from an HTTP(S) Gitea URL.
func parsePullRequestURL(raw string) (string, string, int, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", "", 0, fmt.Errorf("url do pull request inválida")
	}

	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) != 4 || parts[2] != "pulls" {
		return "", "", 0, fmt.Errorf("url do pull request deve seguir o formato /owner/repo/pulls/numero")
	}

	pullRequest, err := strconv.Atoi(parts[3])
	if err != nil || pullRequest <= 0 {
		return "", "", 0, fmt.Errorf("número do pull request inválido")
	}
	return parts[0], parts[1], pullRequest, nil
}
