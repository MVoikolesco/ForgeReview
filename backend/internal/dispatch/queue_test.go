package dispatch

import (
	"context"
	"testing"
)

func TestInProcessQueueDispatchesExecutionIDsInOrder(t *testing.T) {
	queue := NewInProcessQueue()
	if err := queue.Enqueue(context.Background(), 12); err != nil {
		t.Fatal(err)
	}
	if err := queue.Enqueue(context.Background(), 13); err != nil {
		t.Fatal(err)
	}
	for _, want := range []int64{12, 13} {
		got, err := queue.Dequeue(context.Background())
		if err != nil || got != want {
			t.Fatalf("dequeue = %d, %v; want %d", got, err, want)
		}
	}
}
