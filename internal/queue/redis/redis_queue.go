package redis

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"gitea-agents/internal/config"
	"gitea-agents/internal/queue"
)

type Queue struct {
	client *redis.Client
	stream string
	group  string
}

func New(cfg config.Config) *Queue {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})

	return &Queue{
		client: client,
		stream: cfg.RedisStream,
		group:  cfg.RedisGroup,
	}
}

func (q *Queue) Close() error {
	return q.client.Close()
}

func (q *Queue) Ping(ctx context.Context) error {
	return q.client.Ping(ctx).Err()
}

func (q *Queue) workerKey(consumer string) string { return q.stream + ":worker:" + consumer }

func (q *Queue) Heartbeat(ctx context.Context, consumer, state, currentJob string) error {
	key := q.workerKey(consumer)
	if err := q.client.HSet(ctx, key, map[string]any{"name": consumer, "state": state, "current_job": currentJob, "last_seen": time.Now().UTC().Format(time.RFC3339Nano)}).Err(); err != nil {
		return err
	}
	return q.client.Expire(ctx, key, 20*time.Second).Err()
}

func (q *Queue) RecordJob(ctx context.Context, consumer string, success bool) error {
	field := "processed"
	if !success {
		field = "failed"
	}
	return q.client.HIncrBy(ctx, q.workerKey(consumer), field, 1).Err()
}

func (q *Queue) Metrics(ctx context.Context) (queue.Metrics, error) {
	result := queue.Metrics{}
	if err := q.client.Ping(ctx).Err(); err != nil {
		return result, err
	}
	result.Connected = true
	var err error
	if result.StreamLength, err = q.client.XLen(ctx, q.stream).Result(); err != nil {
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
		keys, next, scanErr := q.client.Scan(ctx, cursor, q.stream+":worker:*", 20).Result()
		if scanErr != nil {
			return result, scanErr
		}
		for _, key := range keys {
			values, getErr := q.client.HGetAll(ctx, key).Result()
			if getErr != nil {
				return result, getErr
			}
			lastSeen, _ := time.Parse(time.RFC3339Nano, values["last_seen"])
			processed, _ := strconv.ParseInt(values["processed"], 10, 64)
			failed, _ := strconv.ParseInt(values["failed"], 10, 64)
			result.Workers = append(result.Workers, queue.WorkerMetric{Name: values["name"], State: values["state"], CurrentJob: values["current_job"], LastSeen: lastSeen, Processed: processed, Failed: failed})
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return result, nil
}

func (q *Queue) EnsureGroup(ctx context.Context) error {
	err := q.client.XGroupCreateMkStream(ctx, q.stream, q.group, "0").Err()
	if err == nil {
		return nil
	}

	if isBusyGroupError(err) {
		return nil
	}

	return err
}

func (q *Queue) Publish(ctx context.Context, job queue.ReviewJob) error {
	_, err := q.client.XAdd(ctx, &redis.XAddArgs{
		Stream: q.stream,
		Values: map[string]any{
			"owner":              job.Owner,
			"repo":               job.Repo,
			"pr_number":          job.PRNumber,
			"requested_reviewer": job.RequestedReviewer,
			"sender":             job.Sender,
			"manual":             job.Manual,
			"title":              job.Title,
			"description":        job.Description,
			"author":             job.Author,
			"base_branch":        job.BaseBranch,
			"head_branch":        job.HeadBranch,
		},
	}).Result()

	return err
}

func (q *Queue) Consume(ctx context.Context, consumer string, handle func(context.Context, queue.ReviewJob) error) error {
	if err := q.EnsureGroup(ctx); err != nil {
		return fmt.Errorf("ensure consumer group: %w", err)
	}

	for {
		streams, err := q.client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    q.group,
			Consumer: consumer,
			Streams:  []string{q.stream, ">"},
			Count:    1,
			Block:    5 * time.Second,
		}).Result()
		if err != nil {
			if errors.Is(err, redis.Nil) {
				continue
			}
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		}

		for _, stream := range streams {
			for _, message := range stream.Messages {
				job, err := decodeJob(message.Values)
				if err != nil {
					return fmt.Errorf("decode job %s: %w", message.ID, err)
				}

				if err := handle(ctx, job); err != nil {
					return err
				}

				if err := q.client.XAck(ctx, q.stream, q.group, message.ID).Err(); err != nil {
					return fmt.Errorf("ack job %s: %w", message.ID, err)
				}
			}
		}
	}
}

func decodeJob(values map[string]any) (queue.ReviewJob, error) {
	prNumber, err := strconv.Atoi(valueAsString(values["pr_number"]))
	if err != nil {
		return queue.ReviewJob{}, err
	}

	return queue.ReviewJob{
		Owner:             valueAsString(values["owner"]),
		Repo:              valueAsString(values["repo"]),
		PRNumber:          prNumber,
		RequestedReviewer: valueAsString(values["requested_reviewer"]),
		Sender:            valueAsString(values["sender"]),
		Manual:            valueAsBool(values["manual"]),
		Title:             valueAsString(values["title"]),
		Description:       valueAsString(values["description"]),
		Author:            valueAsString(values["author"]),
		BaseBranch:        valueAsString(values["base_branch"]),
		HeadBranch:        valueAsString(values["head_branch"]),
	}, nil
}

func valueAsString(value any) string {
	if value == nil {
		return ""
	}

	return fmt.Sprint(value)
}

func valueAsBool(value any) bool {
	parsed, err := strconv.ParseBool(valueAsString(value))
	return err == nil && parsed
}

func isBusyGroupError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "BUSYGROUP")
}
