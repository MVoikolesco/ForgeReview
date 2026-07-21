package app

import (
	"context"
	"log"
	"sync"
	"time"

	"gitea-agents/internal/config"
	"gitea-agents/internal/queue"
	redisqueue "gitea-agents/internal/queue/redis"
	"gitea-agents/internal/review"
)

// runWorker consumes review jobs, maintains worker heartbeats, records outcomes,
// and deliberately keeps consuming after an individual review fails.
func runWorker(
	ctx context.Context,
	cfg config.Config,
	queueClient *redisqueue.Queue,
	service *review.Service,
	logger *log.Logger,
) error {
	if err := queueClient.EnsureGroup(ctx); err != nil {
		return err
	}

	logger.Printf(
		"worker listening on stream=%s group=%s consumer=%s",
		cfg.RedisStream,
		cfg.RedisGroup,
		cfg.RedisConsumer,
	)

	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var currentJob string
	var stateMu sync.RWMutex
	reportHeartbeat := func() {
		stateMu.RLock()
		job := currentJob
		stateMu.RUnlock()

		state := "idle"
		if job != "" {
			state = "processing"
		}
		_ = queueClient.Heartbeat(workerCtx, cfg.RedisConsumer, state, job)
	}

	go maintainHeartbeat(workerCtx, reportHeartbeat)
	reportHeartbeat()

	return queueClient.Consume(workerCtx, cfg.RedisConsumer, func(jobCtx context.Context, job queue.ReviewJob) error {
		stateMu.Lock()
		currentJob = job.ReviewID
		stateMu.Unlock()
		reportHeartbeat()

		err := service.Process(jobCtx, job)

		stateMu.Lock()
		currentJob = ""
		stateMu.Unlock()
		reportHeartbeat()

		_ = queueClient.RecordJob(jobCtx, cfg.RedisConsumer, err == nil)
		if err != nil {
			logger.Printf("review job failed without stopping worker: err=%v", err)
		}

		return nil
	})
}

// maintainHeartbeat invokes report every five seconds until ctx is cancelled.
func maintainHeartbeat(ctx context.Context, report func()) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			report()
		}
	}
}
