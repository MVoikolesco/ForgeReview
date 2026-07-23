package dispatch

import (
	"context"
	"errors"

	"forgereview/backend/internal/integration"
	"forgereview/backend/internal/workflow"
)

// ExecutionStore is the minimum durable lifecycle contract needed by a worker.
type ExecutionStore interface {
	ClaimExecution(context.Context, int64) (workflow.Execution, bool, error)
	Load(context.Context, int64) (workflow.Definition, error)
	SaveNodeProgress(context.Context, int64, workflow.NodeRun) error
	CompleteExecution(context.Context, int64, workflow.RunReport) error
	HandleExecutionFailure(context.Context, int64, string) (bool, error)
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
	adapters.Progress = workflow.ProgressObserverFunc(func(progressCtx context.Context, run workflow.NodeRun) error {
		return w.Store.SaveNodeProgress(progressCtx, execution.ID, run)
	})
	if cancellable, ok := w.Store.(interface {
		CancellationRequested(context.Context, int64) (bool, error)
	}); ok {
		adapters.Cancellation = func(checkCtx context.Context) (bool, error) {
			return cancellable.CancellationRequested(checkCtx, execution.ID)
		}
	}
	report, runErr := workflow.RunFromTriggerWithAdapters(ctx, definition, w.Catalog, execution.TriggerNodeKey, execution.Input, adapters)
	if runErr == nil {
		return w.Store.CompleteExecution(ctx, id, report)
	}
	if errors.Is(runErr, context.Canceled) {
		return w.Store.CompleteExecution(ctx, id, report)
	}
	retry, failureErr := w.Store.HandleExecutionFailure(ctx, id, string(integration.ClassifyFailure(runErr)))
	if failureErr != nil {
		return failureErr
	}
	if retry {
		// The next-attempt timestamp is durable. A process restart or a duplicate
		// wake-up is harmless because recovery only re-enqueues due work and Claim
		// remains atomic.
		return w.Queue.Enqueue(ctx, id)
	}
	return nil
}
