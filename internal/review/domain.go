package review

import (
	"time"

	"gitea-agents/internal/contracts"
)

const (
	// StatusReceived indicates that the API persisted a review request.
	StatusReceived = "recebido"
	// StatusQueued indicates that the review job was published to Redis.
	StatusQueued = "enfileirado"
	// StatusProcessing indicates that a worker is executing the review.
	StatusProcessing = "processando"
	// StatusCompleted indicates that processing and required publication ended.
	StatusCompleted = "concluido"
	// StatusFailed indicates that a terminal processing step failed.
	StatusFailed = "falhou"
	// StatusCancelled indicates that an operator cancelled or rejected a review.
	StatusCancelled = "cancelado"
	// StatusAwaitingApproval indicates that a manual publication decision is pending.
	StatusAwaitingApproval = "aguardando_autorizacao"
)

// Comment aliases the shared inline comment contract.
type Comment = contracts.Comment

// FinalReview aliases the shared overall review contract.
type FinalReview = contracts.FinalReview

// Result aliases the shared provider-independent result contract.
type Result = contracts.Result

// Step records one persisted stage of a review execution.
type Step struct {
	ID         int64          `json:"id"`
	ReviewID   string         `json:"review_id"`
	Step       string         `json:"step"`
	Status     string         `json:"status"`
	Message    string         `json:"message"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	StartedAt  time.Time      `json:"started_at"`
	FinishedAt *time.Time     `json:"finished_at,omitempty"`
	DurationMS int64          `json:"duration_ms,omitempty"`
	Error      string         `json:"error,omitempty"`
}

// Review is the persisted review aggregate returned by API handlers.
type Review struct {
	ID          string     `json:"id"`
	Owner       string     `json:"owner"`
	Repository  string     `json:"repository"`
	PullRequest int        `json:"pull_request"`
	Status      string     `json:"status"`
	Source      string     `json:"source"`
	Result      *Result    `json:"result,omitempty"`
	Error       string     `json:"error,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
	Steps       []Step     `json:"steps,omitempty"`
}
