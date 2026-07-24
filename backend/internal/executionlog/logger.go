// Package executionlog writes safe, human-readable per-card execution history.
package executionlog

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"forgereview/backend/internal/workflow"
)

type Writer struct {
	directory string
	mu        sync.Mutex
}

func New(directory string) (*Writer, error) {
	if directory == "" {
		return nil, fmt.Errorf("execution log directory is required")
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return nil, fmt.Errorf("create execution log directory: %w", err)
	}
	return &Writer{directory: directory}, nil
}

func (w *Writer) WriteExecutionLog(_ context.Context, entry workflow.ExecutionLogEntry) error {
	if entry.ExecutionID < 1 || entry.NodeKey == "" || entry.Event == "" || entry.Status == "" {
		return fmt.Errorf("execution log entry is incomplete")
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	path := filepath.Join(w.directory, fmt.Sprintf("execution-%d.log", entry.ExecutionID))
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open execution log: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("inspect execution log: %w", err)
	}
	if info.Size() == 0 {
		if _, err = fmt.Fprintf(file, "ForgeReview — execução #%d (versão %d)\n%s\n", entry.ExecutionID, entry.VersionID, strings.Repeat("=", 72)); err != nil {
			return fmt.Errorf("write execution log header: %w", err)
		}
	}
	refs := []string{}
	if entry.Event != "started" {
		if ref, err := w.writeDiagnostic(entry, "entrada", entry.Inputs); err != nil {
			return err
		} else if ref != "" {
			refs = append(refs, "entrada: "+ref)
		}
		if ref, err := w.writeDiagnostic(entry, "saida", entry.Outputs); err != nil {
			return err
		} else if ref != "" {
			refs = append(refs, "saída: "+ref)
		}
	}
	line := fmt.Sprintf("[%s] %s | %s", entry.OccurredAt.Local().Format("15:04:05.000"), resultLabel(entry), entry.NodeName)
	if entry.NodeType != "" {
		line += fmt.Sprintf(" [%s]", entry.NodeType)
	}
	if entry.ScopeKey != "" && entry.ScopeKey != "root" {
		line += fmt.Sprintf(" | escopo %s", entry.ScopeKey)
	}
	if entry.DurationMS > 0 {
		line += fmt.Sprintf(" | %s", duration(entry.DurationMS))
	}
	if facts := formatFacts(entry.Facts); facts != "" {
		line += " | " + facts
	}
	if entry.Error != "" {
		line += " | motivo: " + entry.Error
	}
	if len(refs) > 0 {
		line += " | detalhes: " + strings.Join(refs, "; ")
	}
	if _, err = fmt.Fprintln(file, line); err != nil {
		return fmt.Errorf("write execution log: %w", err)
	}
	return nil
}

func (w *Writer) writeDiagnostic(entry workflow.ExecutionLogEntry, label string, value any) (string, error) {
	if value == nil {
		return "", nil
	}
	payload, err := json.MarshalIndent(value, "  ", "  ")
	if err != nil {
		return "", fmt.Errorf("serialize execution diagnostic: %w", err)
	}
	if string(payload) == "{}" || string(payload) == "[]" || string(payload) == "null" {
		return "", nil
	}
	directory := filepath.Join(w.directory, fmt.Sprintf("execution-%d", entry.ExecutionID))
	if err = os.MkdirAll(directory, 0o755); err != nil {
		return "", fmt.Errorf("create execution diagnostic directory: %w", err)
	}
	scope := entry.ScopeKey
	if scope == "" {
		scope = "root"
	}
	name := fmt.Sprintf("%s--%s--%s.json", safeFilename(entry.NodeKey), safeFilename(scope), label)
	if err = os.WriteFile(filepath.Join(directory, name), append(payload, '\n'), 0o600); err != nil {
		return "", fmt.Errorf("write execution diagnostic: %w", err)
	}
	return filepath.ToSlash(filepath.Join(fmt.Sprintf("execution-%d", entry.ExecutionID), name)), nil
}

func safeFilename(value string) string {
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, value)
}

func resultLabel(entry workflow.ExecutionLogEntry) string {
	if entry.Event == "started" {
		return "INÍCIO"
	}
	if entry.Event == "log_card" {
		return "LOG REGISTRADO"
	}
	if entry.Status == "completed" && invalidValidation(entry.Facts) {
		return "RESPOSTA INVÁLIDA"
	}
	switch entry.Status {
	case "completed":
		return "CONCLUÍDO"
	case "failed":
		return "FALHOU"
	case "partial":
		return "PARCIAL"
	case "cancelled":
		return "CANCELADO"
	default:
		return strings.ToUpper(entry.Status)
	}
}

func invalidValidation(facts map[string]any) bool {
	attempts := attemptsFrom(facts["validation_attempts"])
	return len(attempts) > 0 && attempts[len(attempts)-1]["status"] == "invalid"
}

func attemptsFrom(value any) []map[string]any {
	switch values := value.(type) {
	case []map[string]any:
		return values
	case []any:
		result := make([]map[string]any, 0, len(values))
		for _, raw := range values {
			if item, ok := raw.(map[string]any); ok {
				result = append(result, item)
			}
		}
		return result
	default:
		return nil
	}
}

func duration(milliseconds int64) string {
	if milliseconds < 1000 {
		return fmt.Sprintf("%d ms", milliseconds)
	}
	return fmt.Sprintf("%.2f s", float64(milliseconds)/1000)
}

func formatFacts(facts map[string]any) string {
	if len(facts) == 0 {
		return ""
	}
	labels := map[string]string{"attempt_count": "tentativas", "retry_limit": "limite de tentativas", "retry_delay_ms": "intervalo", "completed_iterations": "iterações concluídas", "failed_iterations": "iterações falhas", "max_iterations": "limite de iterações", "concurrency": "concorrência", "model": "modelo", "validation_error": "motivo da validação", "error_code": "código", "error_policy": "política", "error_action": "ação"}
	keys := make([]string, 0, len(facts))
	for key := range facts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		if key == "validation_attempts" || key == "provider_calls" {
			attempts := attemptsFrom(facts[key])
			attemptLabels := make([]string, 0, len(attempts))
			for _, attempt := range attempts {
				attemptLabels = append(attemptLabels, fmt.Sprintf("tentativa %v: %v", attempt["attempt"], attempt["status"]))
			}
			if len(attemptLabels) > 0 {
				parts = append(parts, strings.Join(attemptLabels, ", "))
			}
			continue
		}
		label := labels[key]
		if label == "" {
			continue
		}
		value := facts[key]
		if key == "retry_delay_ms" {
			value = fmt.Sprintf("%v ms", value)
		}
		parts = append(parts, fmt.Sprintf("%s: %v", label, value))
	}
	return strings.Join(parts, " | ")
}
