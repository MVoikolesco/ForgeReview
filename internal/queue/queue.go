package queue

import (
	"context"
	"time"
)

type ReviewJob struct {
	Owner             string
	Repo              string
	PRNumber          int
	RequestedReviewer string
	Sender            string
	Manual            bool
	Title             string
	Description       string
	Author            string
	BaseBranch        string
	HeadBranch        string
}

type Publisher interface {
	Publish(ctx context.Context, job ReviewJob) error
}

type Consumer interface {
	Consume(ctx context.Context, consumer string, handle func(context.Context, ReviewJob) error) error
}

type WorkerMetric struct {
	Name       string    `json:"name"`
	State      string    `json:"state"`
	CurrentJob string    `json:"current_job"`
	LastSeen   time.Time `json:"last_seen"`
	Processed  int64     `json:"processed"`
	Failed     int64     `json:"failed"`
}

type Metrics struct {
	Connected    bool           `json:"connected"`
	StreamLength int64          `json:"stream_length"`
	Pending      int64          `json:"pending"`
	Workers      []WorkerMetric `json:"workers"`
}

type Observer interface {
	Metrics(context.Context) (Metrics, error)
}

type WorkerReporter interface {
	Heartbeat(context.Context, string, string, string) error
	RecordJob(context.Context, string, bool) error
}
