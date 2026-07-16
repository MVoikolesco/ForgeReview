package agents

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gitea-agents/internal/queue"
	"gitea-agents/internal/review/pipeline"
)

type reviewRunLog struct {
	dir     string
	enabled bool
}

func newReviewRunLog(rootDir string, job queue.ReviewJob, enabled bool) (*reviewRunLog, error) {
	if !enabled {
		return &reviewRunLog{enabled: false}, nil
	}
	if rootDir == "" {
		rootDir = defaultDiffLogDir
	}

	dir := filepath.Join(rootDir, sanitizeLogName(fmt.Sprintf("%s_%s_pr-%d", job.Owner, job.Repo, job.PRNumber)))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("erro ao criar diretorio de logs de review: %w", err)
	}

	return &reviewRunLog{dir: dir, enabled: true}, nil
}

func (l *reviewRunLog) Dir() string {
	if !l.enabled {
		return "disabled"
	}
	return l.dir
}

func (l *reviewRunLog) Write(name string, content string) (string, error) {
	if !l.enabled {
		return "", nil
	}
	path := filepath.Join(l.dir, sanitizeLogName(name))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("erro ao salvar log fisico: %w", err)
	}

	return path, nil
}

func (l *reviewRunLog) AppendProcess(format string, args ...any) error {
	if !l.enabled {
		return nil
	}
	path := filepath.Join(l.dir, "00-processo.log")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("erro ao abrir log de processo: %w", err)
	}
	defer file.Close()

	line := fmt.Sprintf(format, args...)
	if _, err := fmt.Fprintf(file, "%s %s\n", time.Now().Format(time.RFC3339), line); err != nil {
		return fmt.Errorf("erro ao escrever log de processo: %w", err)
	}

	return nil
}

func (l *reviewRunLog) AppendProgress(event pipeline.ProgressEvent) error {
	if !l.enabled {
		return nil
	}
	if event.Timestamp == "" {
		event.Timestamp = time.Now().Format(time.RFC3339)
	}
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("erro ao serializar progresso: %w", err)
	}
	path := filepath.Join(l.dir, "00-progress.jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("erro ao abrir log de progresso: %w", err)
	}
	defer file.Close()
	if _, err := file.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("erro ao escrever log de progresso: %w", err)
	}
	return nil
}

func sanitizeLogName(value string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == '_' {
			return r
		}

		return '_'
	}, value)
}
