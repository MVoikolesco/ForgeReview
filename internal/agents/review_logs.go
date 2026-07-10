package agents

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gitea-agents/internal/queue"
)

type reviewRunLog struct {
	dir string
}

func newReviewRunLog(rootDir string, job queue.ReviewJob) (*reviewRunLog, error) {
	if rootDir == "" {
		rootDir = defaultDiffLogDir
	}

	dir := filepath.Join(rootDir, sanitizeLogName(fmt.Sprintf("%s_%s_pr-%d", job.Owner, job.Repo, job.PRNumber)))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("erro ao criar diretorio de logs de review: %w", err)
	}

	return &reviewRunLog{dir: dir}, nil
}

func (l *reviewRunLog) Dir() string {
	return l.dir
}

func (l *reviewRunLog) Write(name string, content string) (string, error) {
	path := filepath.Join(l.dir, sanitizeLogName(name))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("erro ao salvar log fisico: %w", err)
	}

	return path, nil
}

func (l *reviewRunLog) AppendProcess(format string, args ...any) error {
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

func sanitizeLogName(value string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == '_' {
			return r
		}

		return '_'
	}, value)
}
