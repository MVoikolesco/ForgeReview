package worker

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

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
	reporter, monitored := w.consumer.(queue.WorkerReporter)
	state, currentJob := "idle", ""
	var heartbeatMu sync.RWMutex
	setHeartbeat := func(nextState, nextJob string) {
		state, currentJob = nextState, nextJob
		if monitored {
			_ = reporter.Heartbeat(ctx, w.name, nextState, nextJob)
		}
	}
	if monitored {
		setHeartbeat("idle", "")
		go func() {
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					heartbeatMu.RLock()
					s, j := state, currentJob
					heartbeatMu.RUnlock()
					_ = reporter.Heartbeat(ctx, w.name, s, j)
				}
			}
		}()
	}

	return w.consumer.Consume(ctx, w.name, func(ctx context.Context, job queue.ReviewJob) error {
		w.logger.Printf("Job recebido: %+v", job)
		w.logger.Printf("Agent selecionado: %s", w.agent.Name())
		jobName := fmt.Sprintf("%s/%s#%d", job.Owner, job.Repo, job.PRNumber)
		heartbeatMu.Lock()
		setHeartbeat("processing", jobName)
		heartbeatMu.Unlock()
		err := w.agent.Process(ctx, job)
		if monitored {
			_ = reporter.RecordJob(ctx, w.name, err == nil)
		}
		heartbeatMu.Lock()
		setHeartbeat("idle", "")
		heartbeatMu.Unlock()
		return err
	})
}
