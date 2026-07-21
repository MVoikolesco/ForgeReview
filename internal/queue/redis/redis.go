package redis

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gitea-agents/internal/config"
	"gitea-agents/internal/queue"

	"github.com/redis/go-redis/v9"
)

const heartbeatTTL = 20 * time.Second

// Queue implements review publishing, consumption, and observability with
// Redis Streams.
type Queue struct {
	client *redis.Client
	stream string
	group  string
}

// New creates a Redis queue from the configured address, database, stream, and
// consumer group. The returned queue owns its Redis client.
func New(cfg config.Config) *Queue {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	return &Queue{client: client, stream: cfg.RedisStream, group: cfg.RedisGroup}
}

// Close releases the underlying Redis client.
func (q *Queue) Close() error {
	return q.client.Close()
}

// Client returns the underlying Redis client for low-level integrations.
func (q *Queue) Client() *redis.Client {
	return q.client
}

// Ping verifies Redis connectivity and returns the client error, if any.
func (q *Queue) Ping(ctx context.Context) error {
	return q.client.Ping(ctx).Err()
}

// EnsureGroup creates the configured stream and consumer group when absent.
func (q *Queue) EnsureGroup(ctx context.Context) error {
	err := q.client.XGroupCreateMkStream(ctx, q.stream, q.group, "0").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return err
	}
	return nil
}

// Publish appends one review job to the configured Redis Stream.
func (q *Queue) Publish(ctx context.Context, job queue.ReviewJob) error {
	values := map[string]any{
		"review_id":          job.ReviewID,
		"gitea_instance_id":  job.GiteaInstanceID,
		"owner":              job.Owner,
		"repo":               job.Repository,
		"pr_number":          job.PullRequest,
		"requested_reviewer": job.RequestedReviewer,
		"sender":             job.Sender,
		"manual":             job.Manual,
		"source":             job.Source,
		"title":              job.Title,
		"description":        job.Description,
		"author":             job.Author,
		"base_branch":        job.BaseBranch,
		"head_branch":        job.HeadBranch,
	}
	_, err := q.client.XAdd(ctx, &redis.XAddArgs{Stream: q.stream, Values: values}).Result()
	return err
}

// Consume reads one job at a time, invokes handle, and acknowledges successful
// callbacks. It blocks until context cancellation or a queue/callback error.
func (q *Queue) Consume(ctx context.Context, consumer string, handle func(context.Context, queue.ReviewJob) error) error {
	if err := q.EnsureGroup(ctx); err != nil {
		return err
	}
	for {
		items, err := q.client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    q.group,
			Consumer: consumer,
			Streams:  []string{q.stream, ">"},
			Count:    1,
			Block:    5 * time.Second,
		}).Result()
		if errors.Is(err, redis.Nil) {
			continue
		}
		if errors.Is(err, context.Canceled) {
			return nil
		}
		if err != nil {
			return err
		}
		for _, stream := range items {
			for _, message := range stream.Messages {
				job, err := decode(message.Values)
				if err != nil {
					return fmt.Errorf("decode job %s: %w", message.ID, err)
				}
				if err := handle(ctx, job); err != nil {
					return err
				}
				if err := q.client.XAck(ctx, q.stream, q.group, message.ID).Err(); err != nil {
					return err
				}
			}
		}
	}
}

// decode converts Redis Stream fields to a ReviewJob.
func decode(values map[string]any) (queue.ReviewJob, error) {
	pullRequest, err := strconv.Atoi(value(values["pr_number"]))
	if err != nil {
		return queue.ReviewJob{}, err
	}

	manual := strings.EqualFold(value(values["manual"]), "true") || value(values["manual"]) == "1"
	return queue.ReviewJob{
		ReviewID:          value(values["review_id"]),
		GiteaInstanceID:   int64Value(values["gitea_instance_id"]),
		Owner:             value(values["owner"]),
		Repository:        value(values["repo"]),
		PullRequest:       pullRequest,
		RequestedReviewer: value(values["requested_reviewer"]),
		Sender:            value(values["sender"]),
		Manual:            manual,
		Source:            value(values["source"]),
		Title:             value(values["title"]),
		Description:       value(values["description"]),
		Author:            value(values["author"]),
		BaseBranch:        value(values["base_branch"]),
		HeadBranch:        value(values["head_branch"]),
	}, nil
}

// value converts a Redis field to its string representation.
func value(input any) string {
	return fmt.Sprint(input)
}

// int64Value converts a Redis field to int64, returning zero when invalid.
func int64Value(input any) int64 {
	number, _ := strconv.ParseInt(value(input), 10, 64)
	return number
}

// Heartbeat updates a worker's state and refreshes its expiration window.
func (q *Queue) Heartbeat(ctx context.Context, consumer, state, currentJob string) error {
	key := q.stream + ":worker:" + consumer
	fields := map[string]any{
		"name":        consumer,
		"state":       state,
		"current_job": currentJob,
		"last_seen":   time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := q.client.HSet(ctx, key, fields).Err(); err != nil {
		return err
	}
	return q.client.Expire(ctx, key, heartbeatTTL).Err()
}

// RecordJob increments the successful or failed counter for one worker.
func (q *Queue) RecordJob(ctx context.Context, consumer string, success bool) error {
	field := "processed"
	if !success {
		field = "failed"
	}
	return q.client.HIncrBy(ctx, q.stream+":worker:"+consumer, field, 1).Err()
}

// Metrics returns connectivity, stream backlog, pending jobs, and current
// worker heartbeat data.
func (q *Queue) Metrics(ctx context.Context) (queue.Metrics, error) {
	result := queue.Metrics{}
	if err := q.Ping(ctx); err != nil {
		return result, err
	}
	result.Connected = true
	var err error
	result.StreamLength, err = q.client.XLen(ctx, q.stream).Result()
	if err != nil {
		return result, err
	}
	pending, err := q.client.XPending(ctx, q.stream, q.group).Result()
	if err == nil {
		result.Pending = pending.Count
	} else if !strings.Contains(err.Error(), "NOGROUP") {
		return result, err
	}
	var cursor uint64
	for {
		keys, next, e := q.client.Scan(ctx, cursor, q.stream+":worker:*", 20).Result()
		if e != nil {
			return result, e
		}
		for _, key := range keys {
			fields, e := q.client.HGetAll(ctx, key).Result()
			if e != nil {
				return result, e
			}
			lastSeen, _ := time.Parse(time.RFC3339Nano, fields["last_seen"])
			processed, _ := strconv.ParseInt(fields["processed"], 10, 64)
			failed, _ := strconv.ParseInt(fields["failed"], 10, 64)
			result.Workers = append(result.Workers, queue.WorkerMetric{
				Name:       fields["name"],
				State:      fields["state"],
				CurrentJob: fields["current_job"],
				LastSeen:   lastSeen,
				Processed:  processed,
				Failed:     failed,
			})
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return result, nil
}
