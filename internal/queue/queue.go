package queue

import (
	"context"
	"time"
)

// ReviewJob contains the immutable input required to process one pull request
// review asynchronously.
type ReviewJob struct {
	ReviewID          string `json:"review_id"`
	GiteaInstanceID   int64  `json:"gitea_instance_id,omitempty"`
	Owner             string `json:"owner"`
	Repository        string `json:"repo"`
	PullRequest       int    `json:"pr_number"`
	RequestedReviewer string `json:"requested_reviewer"`
	Sender            string `json:"sender"`
	Manual            bool   `json:"manual"`
	Source            string `json:"source"`
	Title             string
	Description       string
	Author            string
	BaseBranch        string
	HeadBranch        string
}

// Publisher appends review jobs to an asynchronous queue.
type Publisher interface {
	// Publish enqueues job or returns an error when the queue rejects it.
	Publish(ctx context.Context, job ReviewJob) error
}

// Consumer reads review jobs and delegates each one to a callback.
type Consumer interface {
	// Consume blocks until ctx is cancelled or queue processing fails.
	Consume(ctx context.Context, consumer string, handle func(context.Context, ReviewJob) error) error
}

// Observer exposes queue and worker metrics to administrative handlers.
type Observer interface {
	// Metrics returns a point-in-time queue snapshot.
	Metrics(ctx context.Context) (Metrics, error)
}

// WorkerReporter records worker heartbeats and job outcomes.
type WorkerReporter interface {
	// Heartbeat reports a consumer's state and current job.
	Heartbeat(ctx context.Context, consumer, state, currentJob string) error
	// RecordJob increments the successful or failed job counter.
	RecordJob(ctx context.Context, consumer string, success bool) error
}

// WorkerMetric is one worker heartbeat exposed through observability APIs.
type WorkerMetric struct {
	Name       string    `json:"name"`
	State      string    `json:"state"`
	CurrentJob string    `json:"current_job"`
	LastSeen   time.Time `json:"last_seen"`
	Processed  int64     `json:"processed"`
	Failed     int64     `json:"failed"`
}

// Metrics describes Redis connectivity, stream backlog, and active workers.
type Metrics struct {
	Connected    bool           `json:"connected"`
	StreamLength int64          `json:"stream_length"`
	Pending      int64          `json:"pending"`
	Workers      []WorkerMetric `json:"workers"`
}
