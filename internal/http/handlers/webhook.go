package handlers

import (
	"encoding/json"
	"fmt"
	"gitea-agents/internal/config"
	"gitea-agents/internal/queue"
	"gitea-agents/internal/review"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

type WebhookHandler struct {
	service *review.Service
	cfg     config.Config
}

func NewWebhookHandler(service *review.Service, cfg config.Config) *WebhookHandler {
	return &WebhookHandler{service: service, cfg: cfg}
}
func (h *WebhookHandler) Receive(c *gin.Context) {
	var event struct {
		Action            string `json:"action"`
		Number            int    `json:"number"`
		RequestedReviewer struct {
			Login    string `json:"login"`
			Username string `json:"username"`
		} `json:"requested_reviewer"`
		PullRequest struct {
			Number int    `json:"number"`
			Title  string `json:"title"`
			Body   string `json:"body"`
			User   struct {
				Login    string `json:"login"`
				Username string `json:"username"`
			} `json:"user"`
			Base struct {
				Ref string `json:"ref"`
			} `json:"base"`
			Head struct {
				Ref string `json:"ref"`
			} `json:"head"`
			RequestedReviewers []struct {
				Login    string `json:"login"`
				Username string `json:"username"`
			} `json:"requested_reviewers"`
		} `json:"pull_request"`
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
		Sender struct {
			Login    string `json:"login"`
			Username string `json:"username"`
		} `json:"sender"`
	}
	var err error
	if payload := c.Query("payload"); payload != "" {
		err = json.Unmarshal([]byte(payload), &event)
	} else {
		err = c.ShouldBindJSON(&event)
	}
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid json"})
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
		c.JSON(200, gin.H{"status": "ignored"})
		return
	}
	owner, repo := splitFullName(event.Repository.FullName)
	number := event.Number
	if number == 0 {
		number = event.PullRequest.Number
	}
	sender := event.Sender.Login
	if sender == "" {
		sender = event.Sender.Username
	}
	author := event.PullRequest.User.Login
	if author == "" {
		author = event.PullRequest.User.Username
	}
	item, err := h.service.Enqueue(c, queue.ReviewJob{Owner: owner, Repository: repo, PullRequest: number, RequestedReviewer: reviewer, Sender: sender, Title: event.PullRequest.Title, Description: event.PullRequest.Body, Author: author, BaseBranch: event.PullRequest.Base.Ref, HeadBranch: event.PullRequest.Head.Ref}, "webhook")
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to publish job"})
		return
	}
	c.JSON(202, gin.H{"status": "accepted", "review_id": item.ID})
}
func (h *WebhookHandler) Manual(c *gin.Context) {
	var body struct {
		URL string `json:"url"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": "invalid json"})
		return
	}
	owner, repo, number, err := parseURL(body.URL)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	item, err := h.service.Enqueue(c, queue.ReviewJob{Owner: owner, Repository: repo, PullRequest: number, RequestedReviewer: h.cfg.GiteaBotUsername, Sender: "manual", Manual: true}, "manual")
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to publish job"})
		return
	}
	c.JSON(202, gin.H{"status": "accepted", "review_id": item.ID})
}
func splitFullName(value string) (string, string) {
	parts := strings.SplitN(strings.Trim(value, "/"), "/", 2)
	if len(parts) != 2 {
		return "", value
	}
	return parts[0], parts[1]
}
func parseURL(raw string) (string, string, int, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", "", 0, fmt.Errorf("url do pull request inválida")
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) != 4 || parts[2] != "pulls" {
		return "", "", 0, fmt.Errorf("url do pull request deve seguir o formato /owner/repo/pulls/numero")
	}
	n, err := strconv.Atoi(parts[3])
	if err != nil || n <= 0 {
		return "", "", 0, fmt.Errorf("número do pull request inválido")
	}
	return parts[0], parts[1], n, nil
}
