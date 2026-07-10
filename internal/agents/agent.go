package agents

import (
	"context"

	"gitea-agents/internal/queue"
)

type Agent interface {
	Name() string
	Process(ctx context.Context, job queue.ReviewJob) error
}
