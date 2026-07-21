package dispatch

import (
	"context"

	"forgereview/backend/internal/workflow"
)

// ExecutionStore is the minimum durable lifecycle contract needed by a worker.
type ExecutionStore interface {
	ClaimExecution(context.Context, int64) (workflow.Execution, bool, error)
	Load(context.Context, int64) (workflow.Definition, error)
	CompleteExecution(context.Context, int64, workflow.RunReport) error
}

type Worker struct {
	Queue    Queue
	Store    ExecutionStore
	Catalog  workflow.Catalog
	Adapters workflow.Adapters
}

func (w Worker) Run(ctx context.Context) error {
	for {
		id, err := w.Queue.Dequeue(ctx)
		if err != nil {
			return err
		}
		if err = w.Process(ctx, id); err != nil && ctx.Err() != nil {
			return ctx.Err()
		}
	}
}

// Process is safe for at-least-once queue delivery: ClaimExecution ensures one
// worker runs a queued ID, while publication has a second durable idempotency
// boundary for the external side effect.
func (w Worker) Process(ctx context.Context, id int64) error {
	execution, claimed, err := w.Store.ClaimExecution(ctx, id)
	if err != nil || !claimed {
		return err
	}
	definition, err := w.Store.Load(ctx, execution.VersionID)
	if err != nil {
		return w.Store.CompleteExecution(ctx, id, workflow.RunReport{Status: "failed"})
	}
	adapters := w.Adapters
	adapters.Execution = workflow.ExecutionContext{ID: execution.ID, VersionID: execution.VersionID}
	report, _ := workflow.RunWithAdapters(ctx, definition, w.Catalog, execution.Input, adapters)
	return w.Store.CompleteExecution(ctx, id, report)
}
