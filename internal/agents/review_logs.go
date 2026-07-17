package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gitea-agents/internal/queue"
	"gitea-agents/internal/review/pipeline"
	"gitea-agents/internal/reviewlog"
)

type reviewRunLog struct {
	dir   string
	store reviewlog.Store
	ctx   context.Context
}

func newReviewRunLog(rootDir string, job queue.ReviewJob) (*reviewRunLog, error) {
	return newReviewRunLogWithStore(context.Background(), rootDir, nil, job)
}

func newReviewRunLogWithStore(ctx context.Context, rootDir string, store reviewlog.Store, job queue.ReviewJob) (*reviewRunLog, error) {
	baseName := sanitizeLogName(fmt.Sprintf("%s_%s_pr-%d", job.Owner, job.Repo, job.PRNumber))
	if store != nil {
		name, err := store.CreateRun(ctx, baseName)
		if err != nil {
			return nil, fmt.Errorf("erro ao criar registro de logs de review: %w", err)
		}
		return &reviewRunLog{dir: name, store: store, ctx: ctx}, nil
	}

	if rootDir == "" {
		rootDir = defaultDiffLogDir
	}
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		return nil, fmt.Errorf("erro ao criar diretorio de logs de review: %w", err)
	}

	for attempt := 0; ; attempt++ {
		name := baseName
		if attempt > 0 {
			name = fmt.Sprintf("%s-run-%d", baseName, time.Now().UnixNano())
		}
		dir := filepath.Join(rootDir, name)
		if err := os.Mkdir(dir, 0o755); err == nil {
			return &reviewRunLog{dir: dir, ctx: ctx}, nil
		} else if !os.IsExist(err) {
			return nil, fmt.Errorf("erro ao criar diretorio de logs de review: %w", err)
		}
	}
}

func (l *reviewRunLog) Dir() string {
	return l.dir
}

func (l *reviewRunLog) Write(name string, content string) (string, error) {
	if l.store != nil {
		return name, l.store.Write(l.ctx, filepath.Base(l.dir), sanitizeLogName(name), content)
	}
	path := filepath.Join(l.dir, sanitizeLogName(name))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("erro ao salvar log fisico: %w", err)
	}

	return path, nil
}

func (l *reviewRunLog) AppendProcess(format string, args ...any) error {
	path := filepath.Join(l.dir, "00-processo.log")
	line := fmt.Sprintf(format, args...)
	if l.store != nil {
		return l.store.Append(l.ctx, filepath.Base(l.dir), "00-processo.log", fmt.Sprintf("%s %s\n", time.Now().Format(time.RFC3339), line))
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("erro ao abrir log de processo: %w", err)
	}
	defer file.Close()

	if _, err := fmt.Fprintf(file, "%s %s\n", time.Now().Format(time.RFC3339), line); err != nil {
		return fmt.Errorf("erro ao escrever log de processo: %w", err)
	}

	return nil
}

func (l *reviewRunLog) AppendProgress(event pipeline.ProgressEvent) error {
	if event.Timestamp == "" {
		event.Timestamp = time.Now().Format(time.RFC3339)
	}
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("erro ao serializar progresso: %w", err)
	}
	if l.store != nil {
		return l.store.Append(l.ctx, filepath.Base(l.dir), "00-progress.jsonl", string(data)+"\n")
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
