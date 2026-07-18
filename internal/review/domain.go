package review

import "time"

import "gitea-agents/internal/contracts"

const (
	StatusReceived   = "recebido"
	StatusQueued     = "enfileirado"
	StatusProcessing = "processando"
	StatusCompleted  = "concluido"
	StatusFailed     = "falhou"
	StatusCancelled  = "cancelado"
)

type Comment = contracts.Comment
type FinalReview = contracts.FinalReview
type Result = contracts.Result
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
