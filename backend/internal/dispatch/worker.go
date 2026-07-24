package dispatch

import (
	"context"
	"errors"
	"fmt"
	"time"

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
	if adapters.Workflows == nil {
		if resolver, ok := w.Store.(workflow.WorkflowResolver); ok {
			adapters.Workflows = resolver
		}
	}
	adapters.Execution = workflow.ExecutionContext{ID: execution.ID, VersionID: execution.VersionID}
	nodes := make(map[string]workflow.Node, len(definition.Nodes))
	for _, node := range definition.Nodes {
		nodes[node.Key] = node
	}
	adapters.Progress = workflow.ProgressObserverFunc(func(progressCtx context.Context, run workflow.NodeRun) error {
		if err := w.Store.SaveNodeProgress(progressCtx, execution.ID, run); err != nil {
			return err
		}
		if adapters.Logs == nil {
			return nil
		}
		node := nodes[run.NodeKey]
		event := "finished"
		if run.Status == "running" {
			event = "started"
		} else if node.Type == "log" {
			event = "log_card"
		}
		return adapters.Logs.WriteExecutionLog(progressCtx, workflow.ExecutionLogEntry{ExecutionID: execution.ID, VersionID: execution.VersionID, NodeKey: run.NodeKey, NodeName: node.Name, NodeType: node.Type, ScopeKey: run.ScopeKey, Event: event, Status: run.Status, DurationMS: run.DurationMS, Facts: workflow.SafeExecutionLogFacts(run.Metadata), Error: safeExecutionLogError(run.Error), Inputs: workflow.ExecutionLogDiagnostic(run.Inputs), Outputs: workflow.ExecutionLogDiagnostic(run.Outputs), OccurredAt: time.Now().UTC()})
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

func safeExecutionLogError(value string) string {
	if value == "" {
		return ""
	}
	return fmt.Sprintf("%v", workflow.ExecutionLogDiagnostic(value))
}
