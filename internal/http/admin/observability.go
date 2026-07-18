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
		events = append(events, gin.H{
			"stage":     stage,
			"status":    status,
			"percent":   percent,
			"message":   step.Message,
			"timestamp": step.StartedAt,
		})
	}

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
	}

	percentages := map[string]int{
		"enfileirado":                5,
		"buscando_diff":              20,
		"enviando_para_ia":           45,
		"recebendo_resposta_parcial": 60,
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
