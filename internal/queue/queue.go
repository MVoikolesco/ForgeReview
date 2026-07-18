package queue

import (
	"context"
	"time"
)

type ReviewJob struct {
	ReviewID                                           string `json:"review_id"`
	GiteaInstanceID                                    int64  `json:"gitea_instance_id,omitempty"`
	Owner                                              string `json:"owner"`
	Repository                                         string `json:"repo"`
	PullRequest                                        int    `json:"pr_number"`
	RequestedReviewer                                  string `json:"requested_reviewer"`
	Sender                                             string `json:"sender"`
	Manual                                             bool   `json:"manual"`
	Title, Description, Author, BaseBranch, HeadBranch string
}

type Publisher interface {
	Publish(context.Context, ReviewJob) error
}
type Consumer interface {
	Consume(context.Context, string, func(context.Context, ReviewJob) error) error
}
type Observer interface {
	Metrics(context.Context) (Metrics, error)
}
type WorkerReporter interface {
	Heartbeat(context.Context, string, string, string) error
	RecordJob(context.Context, string, bool) error
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
