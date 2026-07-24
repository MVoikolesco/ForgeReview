package admin

import (
	"net/http"

	"gitea-agents/internal/http/responses"
	"gitea-agents/internal/review"

	"github.com/gin-gonic/gin"
)

// status returns counts for the principal administrative resources.
func (h *AdminHandler) status(c *gin.Context) {
	counts := map[string]any{}
	tables := map[string]string{
		"providers":    "ai_providers",
		"connections":  "ai_connections",
		"models":       "ai_models",
		"profiles":     "review_profiles",
		"prompts":      "review_prompts",
		"policies":     "review_policies",
		"repositories": "repositories",
	}

	for name, table := range tables {
		var count int
		_ = h.db.QueryRowContext(c, "SELECT count(*) FROM "+table).Scan(&count)
		counts[name] = count
	}

	responses.Legacy(c, http.StatusOK, counts)
}

// metrics returns queue health, worker heartbeats, and the persisted review
// count expected by the administrative dashboard.
func (h *AdminHandler) metrics(c *gin.Context) {
	value := gin.H{
		"queue": gin.H{
			"connected":     false,
			"stream_length": 0,
			"pending":       0,
			"workers":       []any{},
		},
		"reviews": gin.H{"total": 0, "bytes": 0},
	}

	if h.observer != nil {
		if queueMetrics, err := h.observer.Metrics(c); err == nil {
			value["queue"] = queueMetrics
		}
	}
	if items, err := h.reviewRepo.List(c, 100); err == nil {
		value["reviews"] = gin.H{"total": len(items), "bytes": 0}
	}

	responses.Legacy(c, http.StatusOK, value)
}

// observabilityReviews returns the review summaries consumed by the execution
// history selector in the administrative frontend.
func (h *AdminHandler) observabilityReviews(c *gin.Context) {
	items, err := h.reviewRepo.List(c, 100)
	if err != nil {
		responses.LegacyError(c, http.StatusInternalServerError, "could not read reviews")
		return
	}

	out := make([]gin.H, 0, len(items))
	for _, item := range items {
		out = append(out, gin.H{
			"name":       item.ID,
			"updated_at": item.UpdatedAt,
			"files":      0,
			"bytes":      0,
			"owner":      item.Owner,
			"repo":       item.Repository,
			"pr_number":  item.PullRequest,
		})
	}

	responses.Legacy(c, http.StatusOK, out)
}

// observabilityProgress returns one review's steps translated to the stage and
// status vocabulary used by the administrative flow visualization.
func (h *AdminHandler) observabilityProgress(c *gin.Context) {
	id := c.Query("name")
	if id == "" {
		items, _ := h.reviewRepo.List(c, 1)
		if len(items) > 0 {
			id = items[0].ID
		}
	}
	if id == "" {
		responses.Legacy(c, http.StatusOK, gin.H{"active": false})
		return
	}

	item, err := h.reviewRepo.Get(c, id)
	if err != nil {
		responses.Legacy(c, http.StatusOK, gin.H{"active": false})
		return
	}

	events := make([]gin.H, 0, len(item.Steps))
	for _, step := range item.Steps {
		stage, status, percent := progressMapping(step.Step, step.Status)
		event := gin.H{
			"stage":       stage,
			"status":      status,
			"percent":     percent,
			"message":     step.Message,
			"timestamp":   step.StartedAt,
			"metadata":    step.Metadata,
			"duration_ms": step.DurationMS,
			"error":       step.Error,
		}
		if metadata := step.Metadata; metadata != nil {
			if value := stringValue(metadata["group_id"]); value != "" {
				event["group_id"] = value
			}
			for _, key := range []string{"group_index", "total_groups", "findings", "failed_groups", "attempt", "max_attempts"} {
				if value, ok := metadata[key]; ok {
					event[key] = intValue(value, 0)
				}
			}
			if files := stringValues(metadata["files"]); len(files) > 0 {
				event["files"] = files
			}
		}
		events = append(events, event)
	}
	events = appendTerminalReviewEvent(item, events)

	stage, status, percent := progressMapping(item.Status, "")
	if len(events) > 0 {
		last := events[len(events)-1]
		stage = stringValue(last["stage"])
		status = stringValue(last["status"])
		percent = intValue(last["percent"], 0)
	}

	active := item.Status != review.StatusCompleted &&
		item.Status != review.StatusFailed &&
		item.Status != review.StatusCancelled

	responses.Legacy(c, http.StatusOK, gin.H{
		"active": active,
		"review": gin.H{
			"name":       item.ID,
			"updated_at": item.UpdatedAt,
			"percent":    percent,
			"stage":      stage,
			"status":     status,
			"message":    item.Error,
			"events":     events,
			"owner":      item.Owner,
			"repo":       item.Repository,
			"pr_number":  item.PullRequest,
		},
	})
}

func (h *AdminHandler) observabilityStageLogs(c *gin.Context) {
	id := c.Query("name")
	stageKey := c.Query("stage")
	if id == "" || stageKey == "" {
		responses.LegacyError(c, http.StatusBadRequest, "name e stage são obrigatórios")
		return
	}
	entries, err := h.reviewRepo.StageExecutionLogs(c, id, stageKey)
	if err != nil {
		responses.LegacyError(c, http.StatusInternalServerError, "could not load stage logs")
		return
	}
	out := make([]gin.H, 0, len(entries))
	for _, entry := range entries {
		item := gin.H{
			"id":            entry.ID,
			"stage_key":     entry.StageKey,
			"attempt":       entry.Attempt,
			"status":        entry.Status,
			"artifact_type": entry.ArtifactType,
			"metadata":      entry.Metadata,
			"started_at":    entry.StartedAt,
			"finished_at":   entry.FinishedAt,
			"duration_ms":   entry.DurationMS,
			"error":         entry.Error,
			"artifacts":     entry.Artifacts,
		}
		if metadata := entry.Metadata; metadata != nil {
			if value := stringValue(metadata["group_id"]); value != "" {
				item["group_id"] = value
			}
			for _, key := range []string{"group_index", "total_groups", "attempt", "max_attempts", "input_chars", "response_chars", "requested_output_tokens", "actual_prompt_tokens", "actual_completion_tokens", "duration_ms"} {
				if value, ok := metadata[key]; ok {
					item[key] = intValue(value, 0)
				}
			}
			for _, key := range []string{"prompt", "response", "provider", "model"} {
				if value := stringValue(metadata[key]); value != "" {
					item[key] = value
				}
			}
			if files := stringValues(metadata["files"]); len(files) > 0 {
				item["files"] = files
			}
		}
		out = append(out, item)
	}
	responses.Legacy(c, http.StatusOK, gin.H{"name": id, "stage": stageKey, "entries": out})
}

// progressMapping converts persisted review steps to frontend stages, statuses,
// and approximate completion percentages.
func progressMapping(step, rawStatus string) (stage, status string, percent int) {
	stage = step
	switch step {
	case "enfileirado", "buscando_diff":
		stage = "preparacao"
	case "enviando_para_ia", "recebendo_resposta_parcial":
		stage = "revisao"
	case "agregando_resultado":
		stage = "consolidacao"
	case "publicando_comentario":
		stage = "publicacao"
	case review.StatusAwaitingApproval, "pre-publicacao":
		stage = "pre-publicacao"
	case review.StatusCompleted:
		stage = "publicacao"
	}

	status = rawStatus
	if status == "" {
		status = "running"
	}
	switch rawStatus {
	case "concluido":
		status = "done"
	case "falhou":
		status = "failed"
	case "aguardando":
		status = "waiting"
	case "processando":
		status = "running"
	case "retentando":
		status = "retrying"
	}

	percentages := map[string]int{
		"enfileirado":                5,
		"buscando_diff":              20,
		"preparacao":                 15,
		"enviando_para_ia":           45,
		"recebendo_resposta_parcial": 60,
		"planejamento":               30,
		"revisao":                    60,
		"consolidacao":               75,
		"verificacao":                88,
		"formatacao":                 96,
		"agregando_resultado":        80,
		"pre-publicacao":             90,
		"publicando_comentario":      95,
		"publicacao":                 100,
	}
	percent = percentages[step]

	if step == review.StatusAwaitingApproval {
		percent = 90
		status = "waiting"
	}
	if step == review.StatusCompleted {
		percent = 100
		status = "done"
	}

	return stage, status, percent
}

func appendTerminalReviewEvent(item review.Review, events []gin.H) []gin.H {
	if item.Status != review.StatusCompleted {
		return events
	}
	if len(events) > 0 {
		last := events[len(events)-1]
		if stringValue(last["stage"]) == "publicacao" &&
			stringValue(last["status"]) == "done" &&
			intValue(last["percent"], 0) == 100 {
			return events
		}
	}
	return append(events, gin.H{
		"stage":     "publicacao",
		"status":    "done",
		"percent":   100,
		"message":   "Review publicada no Gitea",
		"timestamp": item.UpdatedAt,
	})
}

func stringValues(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		if text := stringValue(item); text != "" {
			result = append(result, text)
		}
	}
	return result
}
