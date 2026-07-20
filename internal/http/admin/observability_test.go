package admin

import (
	"testing"
	"time"

	"gitea-agents/internal/review"

	"github.com/gin-gonic/gin"
)

func TestAppendTerminalReviewEventCompletesManualPublication(t *testing.T) {
	updatedAt := time.Now().UTC()
	events := []gin.H{{
		"stage":   "pre-publicacao",
		"status":  "waiting",
		"percent": 90,
	}}

	result := appendTerminalReviewEvent(review.Review{
		Status:    review.StatusCompleted,
		UpdatedAt: updatedAt,
	}, events)

	if len(result) != 2 {
		t.Fatalf("expected 2 events, got %d", len(result))
	}
	last := result[len(result)-1]
	if got := stringValue(last["stage"]); got != "publicacao" {
		t.Fatalf("expected terminal stage publicacao, got %q", got)
	}
	if got := stringValue(last["status"]); got != "done" {
		t.Fatalf("expected terminal status done, got %q", got)
	}
	if got := intValue(last["percent"], 0); got != 100 {
		t.Fatalf("expected terminal percent 100, got %d", got)
	}
}

func TestAppendTerminalReviewEventDoesNotDuplicateCompletion(t *testing.T) {
	events := []gin.H{{
		"stage":   "publicacao",
		"status":  "done",
		"percent": 100,
	}}

	result := appendTerminalReviewEvent(review.Review{Status: review.StatusCompleted}, events)

	if len(result) != 1 {
		t.Fatalf("expected existing completion event to be reused, got %d events", len(result))
	}
}
