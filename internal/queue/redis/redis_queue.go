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
	}, nil
}

func valueAsString(value any) string {
	if value == nil {
		return ""
	}

	return fmt.Sprint(value)
}

func isBusyGroupError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "BUSYGROUP")
}
