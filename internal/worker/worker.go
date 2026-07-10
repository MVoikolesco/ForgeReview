package worker

import (
	"context"
	"log"

	"gitea-agents/internal/agents"
	"gitea-agents/internal/queue"
)

type Worker struct {
	logger   *log.Logger
	consumer queue.Consumer
	name     string
	agent    agents.Agent
}

func New(logger *log.Logger, consumer queue.Consumer, name string, agent agents.Agent) *Worker {
	return &Worker{
		logger:   logger,
		consumer: consumer,
		name:     name,
		agent:    agent,
	}
}

func (w *Worker) Run(ctx context.Context) error {
	w.logger.Printf("worker iniciado: consumer=%s", w.name)

	return w.consumer.Consume(ctx, w.name, func(ctx context.Context, job queue.ReviewJob) error {
		w.logger.Printf("Job recebido: %+v", job)
		w.logger.Printf("Agent selecionado: %s", w.agent.Name())
		return w.agent.Process(ctx, job)
	})
}
