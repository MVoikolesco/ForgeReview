package pipeline

import (
	"encoding/json"
	"fmt"
	"strings"
)

func parseJSONStage[T any](stage string, raw string, out *T) error {
	clean := stripMarkdownFence(strings.TrimSpace(raw))
	if clean == "" {
		return fmt.Errorf("%s retornou resposta vazia", stage)
	}
	decoder := json.NewDecoder(strings.NewReader(clean))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		if repaired, ok := repairJSONObject(clean); ok {
			decoder = json.NewDecoder(strings.NewReader(repaired))
			decoder.DisallowUnknownFields()
			if retryErr := decoder.Decode(out); retryErr == nil {
				return nil
			}
		}
		return fmt.Errorf("%s retornou JSON invalido: %w", stage, err)
	}
	return nil
}

func stripMarkdownFence(value string) string {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "```") {
		return value
	}
	lines := strings.Split(value, "\n")
	if len(lines) < 2 {
		return value
	}
	first := strings.TrimSpace(lines[0])
	last := strings.TrimSpace(lines[len(lines)-1])
	if !strings.HasPrefix(first, "```") || last != "```" {
		return value
	}
	return strings.TrimSpace(strings.Join(lines[1:len(lines)-1], "\n"))
}

func repairJSONObject(value string) (string, bool) {
	start := strings.Index(value, "{")
	end := strings.LastIndex(value, "}")
	if start < 0 || end <= start {
		return "", false
	}
	return strings.TrimSpace(value[start : end+1]), true
}

func normalizeSeverity(value string) string {
	n := strings.ToLower(strings.TrimSpace(value))
	n = strings.ReplaceAll(n, "é", "e")
	n = strings.ReplaceAll(n, "í", "i")
	switch n {
	case "critical", "critico", "critica", "crítica":
		return "critica"
	case "high", "alto", "alta":
		return "alta"
	case "medium", "medio", "media", "média":
		return "media"
	case "low", "baixo", "baixa":
		return "baixa"
	default:
		return ""
	}
}

func normalizeRisk(value string) string {
	if s := normalizeSeverity(value); s != "" {
		return s
	}
	return "baixo"
}

func clampConfidence(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
