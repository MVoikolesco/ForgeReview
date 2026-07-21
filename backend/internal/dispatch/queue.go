// Package dispatch provides durable execution-ID dispatchers and workers.
package dispatch

import (
	"context"
	"fmt"
	"strconv"

	"github.com/redis/go-redis/v9"
)

const redisQueueKey = "forgereview:execution:ids"

// Queue is deliberately limited to execution IDs, keeping event payloads in
// SQLite and out of Redis messages.
type Queue interface {
	Enqueue(context.Context, int64) error
	Dequeue(context.Context) (int64, error)
}

type RedisQueue struct{ client redis.UniversalClient }

func NewRedisQueue(rawURL string) (*RedisQueue, error) {
	options, err := redis.ParseURL(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse Redis URL: %w", err)
	}
	client := redis.NewClient(options)
	if err = client.Ping(context.Background()).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("connect Redis: %w", err)
	}
	return &RedisQueue{client: client}, nil
}

func (q *RedisQueue) Close() error { return q.client.Close() }

func (q *RedisQueue) Enqueue(ctx context.Context, executionID int64) error {
	if executionID < 1 {
		return fmt.Errorf("execution ID must be positive")
	}
	return q.client.LPush(ctx, redisQueueKey, strconv.FormatInt(executionID, 10)).Err()
}

func (q *RedisQueue) Dequeue(ctx context.Context) (int64, error) {
	items, err := q.client.BRPop(ctx, 0, redisQueueKey).Result()
	if err != nil {
		return 0, err
	}
	if len(items) != 2 {
		return 0, fmt.Errorf("Redis returned an invalid queue item")
	}
	id, err := strconv.ParseInt(items[1], 10, 64)
	if err != nil || id < 1 {
		return 0, fmt.Errorf("Redis queue contained an invalid execution ID")
	}
	return id, nil
}

// InProcessQueue is intentionally non-durable and only appropriate for tests
// or explicitly opted-in local development when Redis is unavailable.
type InProcessQueue struct{ items chan int64 }

func NewInProcessQueue() *InProcessQueue { return &InProcessQueue{items: make(chan int64, 128)} }

func (q *InProcessQueue) Enqueue(ctx context.Context, executionID int64) error {
	if executionID < 1 {
		return fmt.Errorf("execution ID must be positive")
	}
	select {
	case q.items <- executionID:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (q *InProcessQueue) Dequeue(ctx context.Context) (int64, error) {
	select {
	case id := <-q.items:
		return id, nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}
