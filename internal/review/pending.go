package review

import "gitea-agents/internal/queue"

// PendingReview is the durable handoff between the worker and the admin approval flow.
type PendingReview struct {
	Job         queue.ReviewJob `json:"job"`
	FinalReview FinalReview     `json:"final_review"`
}
