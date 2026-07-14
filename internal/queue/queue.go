package queue

import "context"

type ReviewJob struct {
	Owner             string
	Repo              string
	PRNumber          int
	RequestedReviewer string
	Sender            string
	Manual            bool
}

type Publisher interface {
	Publish(ctx context.Context, job ReviewJob) error
}

type Consumer interface {
	Consume(ctx context.Context, consumer string, handle func(context.Context, ReviewJob) error) error
}
