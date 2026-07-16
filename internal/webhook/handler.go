package webhook

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"gitea-agents/internal/queue"
)

const maxPayloadBytes = 1 << 20

type Handler struct {
	logger      *log.Logger
	publisher   queue.Publisher
	botUsername string
}

func RegisterRoutes(mux *http.ServeMux, logger *log.Logger, publisher queue.Publisher, botUsername string, serviceName string, version string, startedAt time.Time) {
	handler := &Handler{
		logger:      logger,
		publisher:   publisher,
		botUsername: botUsername,
	}

	mux.Handle("/health", NewHealthHandler(serviceName, version, startedAt))
	mux.HandleFunc("/webhook", handler.Receive)
	mux.HandleFunc("/review", handler.ManualReview)
}

func (h *Handler) Receive(w http.ResponseWriter, r *http.Request) {
	body, err := readPayload(w, r)
	if err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	if len(body) == 0 {
		writeIgnored(w)
		return
	}

	var event ReviewRequestedEvent
	if err := json.Unmarshal(body, &event); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	reviewer, ok := event.RequestedReviewerName(h.botUsername)
	if !ok {
		writeIgnored(w)
		return
	}

	owner, repo := splitRepository(event.Repository.FullName)
	prNumber := event.PRNumber()
	sender := event.Sender.LoginName()

	job := queue.ReviewJob{
		Owner:             owner,
		Repo:              repo,
		PRNumber:          prNumber,
		RequestedReviewer: reviewer,
		Sender:            sender,
		Title:             event.PullRequest.Title,
		Description:       event.PullRequest.Body,
		Author:            event.PullRequest.Author.LoginName(),
		BaseBranch:        event.PullRequest.Base.BranchName(),
		HeadBranch:        event.PullRequest.Head.BranchName(),
	}

	h.logger.Printf(
		"Webhook válido recebido\nReviewer: %s\nSolicitado por: %s\nRepositório: %s/%s\nPR: #%d\nAção: %s",
		reviewer,
		sender,
		owner,
		repo,
		prNumber,
		event.Action,
	)
	h.logger.Printf("Job criado: %+v", job)

	if err := h.publisher.Publish(r.Context(), job); err != nil {
		h.logger.Printf("erro ao publicar job no redis: %v", err)
		http.Error(w, "failed to publish job", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]string{
		"status": "accepted",
	})
}

type manualReviewRequest struct {
	URL string `json:"url"`
}

// ManualReview enqueues a review from a pull request URL without requiring a webhook event.
func (h *Handler) ManualReview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxPayloadBytes))
	if err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var request manualReviewRequest
	if err := json.Unmarshal(body, &request); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	owner, repo, prNumber, err := parsePullRequestURL(request.URL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	job := queue.ReviewJob{
		Owner:             owner,
		Repo:              repo,
		PRNumber:          prNumber,
		RequestedReviewer: h.botUsername,
		Sender:            "manual",
		Manual:            true,
	}

	h.logger.Printf("Review manual solicitado: %s/%s PR #%d", owner, repo, prNumber)
	h.logger.Printf("Job criado: %+v", job)

	if err := h.publisher.Publish(r.Context(), job); err != nil {
		h.logger.Printf("erro ao publicar job manual no redis: %v", err)
		http.Error(w, "failed to publish job", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]string{
		"status": "accepted",
	})
}

func parsePullRequestURL(rawURL string) (string, string, int, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", "", 0, fmt.Errorf("url do pull request inválida")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", "", 0, fmt.Errorf("url do pull request deve usar http ou https")
	}

	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) != 4 || !strings.EqualFold(parts[2], "pulls") {
		return "", "", 0, fmt.Errorf("url do pull request deve seguir o formato /owner/repo/pulls/numero")
	}

	owner, err := url.PathUnescape(parts[0])
	if err != nil {
		return "", "", 0, fmt.Errorf("owner inválido")
	}
	repo, err := url.PathUnescape(parts[1])
	if err != nil {
		return "", "", 0, fmt.Errorf("repositório inválido")
	}
	prNumber, err := strconv.Atoi(parts[3])
	if err != nil || prNumber <= 0 {
		return "", "", 0, fmt.Errorf("número do pull request inválido")
	}
	if owner == "" || repo == "" {
		return "", "", 0, fmt.Errorf("owner e repositório são obrigatórios")
	}

	return owner, repo, prNumber, nil
}

func readPayload(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	if payload := r.URL.Query().Get("payload"); payload != "" {
		return []byte(payload), nil
	}

	if r.Method != http.MethodPost {
		return nil, nil
	}

	if err := r.ParseForm(); err == nil {
		if payload := r.FormValue("payload"); payload != "" {
			return []byte(payload), nil
		}
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxPayloadBytes))
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()

	return body, nil
}

type ReviewRequestedEvent struct {
	Action            string `json:"action"`
	Number            int    `json:"number"`
	RequestedReviewer User   `json:"requested_reviewer"`
	PullRequest       struct {
		Number             int    `json:"number"`
		Title              string `json:"title"`
		Body               string `json:"body"`
		Author             User   `json:"user"`
		RequestedReviewers []User `json:"requested_reviewers"`
		Base               Branch `json:"base"`
		Head               Branch `json:"head"`
	} `json:"pull_request"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
	Sender User `json:"sender"`
}

func (e ReviewRequestedEvent) RequestedReviewerName(expectedLogin string) (string, bool) {
	if e.Action != "review_requested" {
		return "", false
	}

	if reviewer := e.RequestedReviewer.LoginName(); reviewer != "" {
		return reviewer, reviewer == expectedLogin
	}

	for _, reviewer := range e.PullRequest.RequestedReviewers {
		if name := reviewer.LoginName(); name != "" {
			if name == expectedLogin {
				return name, true
			}
		}
	}

	return "", false
}

func (e ReviewRequestedEvent) PRNumber() int {
	if e.Number != 0 {
		return e.Number
	}

	return e.PullRequest.Number
}

type User struct {
	Login    string `json:"login"`
	Username string `json:"username"`
}

type Branch struct {
	Ref    string `json:"ref"`
	Name   string `json:"name"`
	Branch string `json:"branch"`
}

func (b Branch) BranchName() string {
	if b.Ref != "" {
		return b.Ref
	}
	if b.Name != "" {
		return b.Name
	}
	return b.Branch
}

func (u User) LoginName() string {
	if u.Login != "" {
		return u.Login
	}

	return u.Username
}

func writeIgnored(w http.ResponseWriter) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ignored",
	})
}

func splitRepository(fullName string) (string, string) {
	owner, repo, ok := strings.Cut(fullName, "/")
	if !ok {
		return "", fullName
	}

	return owner, repo
}
