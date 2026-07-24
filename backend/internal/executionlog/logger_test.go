package executionlog

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"forgereview/backend/internal/workflow"
)

func TestWriterCreatesSafePerExecutionJSONL(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "logs")
	writer, err := New(directory)
	if err != nil {
		t.Fatal(err)
	}
	err = writer.WriteExecutionLog(context.Background(), workflow.ExecutionLogEntry{ExecutionID: 42, VersionID: 9, NodeKey: "validate", NodeName: "Validar resposta", NodeType: "validate", ScopeKey: "root", Event: "finished", Status: "failed", DurationMS: 18, OccurredAt: time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(filepath.Join(directory, "execution-42.log"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(payload)
	for _, want := range []string{"ForgeReview — execução #42", "FALHOU | Validar resposta", "18 ms"} {
		if !strings.Contains(text, want) {
			t.Fatalf("log does not contain %s: %s", want, text)
		}
	}
}
