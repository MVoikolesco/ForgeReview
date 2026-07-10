package worker

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"testing"

	"gitea-agents/internal/queue"
)

func TestWorkerProcessesJobWithAgent(t *testing.T) {
	var logs bytes.Buffer
	consumer := &fakeConsumer{
		job: queue.ReviewJob{
			Owner:             "Qualyagro",
			Repo:              "wiki",
			PRNumber:          12,
			RequestedReviewer: "ia-reviewer",
			Sender:            "marcio",
		},
	}
	agent := &fakeAgent{name: "reviewer"}
	worker := New(log.New(&logs, "", 0), consumer, "worker-1", agent)

	if err := worker.Run(context.Background()); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !consumer.handled {
		t.Fatal("expected job to be handled")
	}

	if agent.job != consumer.job {
		t.Fatalf("unexpected agent job\nwant: %+v\n got: %+v", consumer.job, agent.job)
	}

	if !strings.Contains(logs.String(), "Agent selecionado: reviewer") {
		t.Fatalf("expected agent log, got %q", logs.String())
	}
}

func TestWorkerReturnsErrorWhenAgentFails(t *testing.T) {
	consumer := &fakeConsumer{
		job: queue.ReviewJob{
			Owner:    "Qualyagro",
			Repo:     "wiki",
			PRNumber: 12,
		},
	}
	agent := &fakeAgent{name: "reviewer", err: errors.New("agent failed")}
	worker := New(log.New(&bytes.Buffer{}, "", 0), consumer, "worker-1", agent)

	err := worker.Run(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

type fakeConsumer struct {
	job     queue.ReviewJob
	handled bool
}

func (c *fakeConsumer) Consume(ctx context.Context, consumer string, handle func(context.Context, queue.ReviewJob) error) error {
	err := handle(ctx, c.job)
	if err != nil {
		return err
	}

	c.handled = true
	return nil
}

type fakeAgent struct {
	name string
	err  error
	job  queue.ReviewJob
}

func (a *fakeAgent) Name() string {
	return a.name
}

func (a *fakeAgent) Process(ctx context.Context, job queue.ReviewJob) error {
	a.job = job

	if a.err != nil {
		return a.err
	}

	return nil
}
