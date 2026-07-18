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

type Queue struct {
	client        *redis.Client
	stream, group string
}

func New(cfg config.Config) *Queue {
	return &Queue{client: redis.NewClient(&redis.Options{Addr: cfg.RedisAddr, Password: cfg.RedisPassword, DB: cfg.RedisDB}), stream: cfg.RedisStream, group: cfg.RedisGroup}
}
func (q *Queue) Close() error                   { return q.client.Close() }
func (q *Queue) Client() *redis.Client          { return q.client }
func (q *Queue) Ping(ctx context.Context) error { return q.client.Ping(ctx).Err() }
func (q *Queue) EnsureGroup(ctx context.Context) error {
	err := q.client.XGroupCreateMkStream(ctx, q.stream, q.group, "0").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return err
	}
	return nil
}
func (q *Queue) Publish(ctx context.Context, job queue.ReviewJob) error {
	_, err := q.client.XAdd(ctx, &redis.XAddArgs{Stream: q.stream, Values: map[string]any{"review_id": job.ReviewID, "gitea_instance_id": job.GiteaInstanceID, "owner": job.Owner, "repo": job.Repository, "pr_number": job.PullRequest, "requested_reviewer": job.RequestedReviewer, "sender": job.Sender, "manual": job.Manual, "title": job.Title, "description": job.Description, "author": job.Author, "base_branch": job.BaseBranch, "head_branch": job.HeadBranch}}).Result()
	return err
}
func (q *Queue) Consume(ctx context.Context, consumer string, handle func(context.Context, queue.ReviewJob) error) error {
	if err := q.EnsureGroup(ctx); err != nil {
		return err
	}
	for {
		items, err := q.client.XReadGroup(ctx, &redis.XReadGroupArgs{Group: q.group, Consumer: consumer, Streams: []string{q.stream, ">"}, Count: 1, Block: 5 * time.Second}).Result()
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
func decode(values map[string]any) (queue.ReviewJob, error) {
	n, err := strconv.Atoi(value(values["pr_number"]))
	if err != nil {
		return queue.ReviewJob{}, err
	}
	return queue.ReviewJob{ReviewID: value(values["review_id"]), GiteaInstanceID: int64Value(values["gitea_instance_id"]), Owner: value(values["owner"]), Repository: value(values["repo"]), PullRequest: n, RequestedReviewer: value(values["requested_reviewer"]), Sender: value(values["sender"]), Manual: strings.EqualFold(value(values["manual"]), "true") || value(values["manual"]) == "1", Title: value(values["title"]), Description: value(values["description"]), Author: value(values["author"]), BaseBranch: value(values["base_branch"]), HeadBranch: value(values["head_branch"])}, nil
}
func value(v any) string     { return fmt.Sprint(v) }
func int64Value(v any) int64 { n, _ := strconv.ParseInt(value(v), 10, 64); return n }
func (q *Queue) Heartbeat(ctx context.Context, consumer, state, currentJob string) error {
	key := q.stream + ":worker:" + consumer
	if err := q.client.HSet(ctx, key, map[string]any{"name": consumer, "state": state, "current_job": currentJob, "last_seen": time.Now().UTC().Format(time.RFC3339Nano)}).Err(); err != nil {
		return err
	}
	return q.client.Expire(ctx, key, heartbeatTTL).Err()
}
func (q *Queue) RecordJob(ctx context.Context, consumer string, success bool) error {
	field := "processed"
	if !success {
		field = "failed"
	}
	return q.client.HIncrBy(ctx, q.stream+":worker:"+consumer, field, 1).Err()
}
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
			v, e := q.client.HGetAll(ctx, key).Result()
			if e != nil {
				return result, e
			}
			last, _ := time.Parse(time.RFC3339Nano, v["last_seen"])
			processed, _ := strconv.ParseInt(v["processed"], 10, 64)
			failed, _ := strconv.ParseInt(v["failed"], 10, 64)
			result.Workers = append(result.Workers, queue.WorkerMetric{Name: v["name"], State: v["state"], CurrentJob: v["current_job"], LastSeen: last, Processed: processed, Failed: failed})
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return result, nil
}
