package handlers

import (
	"database/sql"
	"net/http"

	"gitea-agents/internal/http/responses"
	"gitea-agents/internal/queue"
	"gitea-agents/internal/review"

	"github.com/gin-gonic/gin"
)

// manual validates a manual review request and enqueues it for asynchronous
// processing. The response contains the persisted review identifier.
func (h *AdminHandler) manual(c *gin.Context) {
	var body struct {
		InstanceID int64  `json:"instance_id"`
		Owner      string `json:"owner"`
		Repo       string `json:"repo"`
		PRNumber   int    `json:"pr_number"`
		URL        string `json:"url"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		responses.LegacyError(c, http.StatusBadRequest, "invalid JSON")
		return
	}
	if body.Owner == "" || body.Repo == "" || body.PRNumber <= 0 {
		responses.LegacyError(c, http.StatusBadRequest, "owner, repo e pr_number são obrigatórios")
		return
	}

	job := queue.ReviewJob{
		GiteaInstanceID:   body.InstanceID,
		Owner:             body.Owner,
		Repository:        body.Repo,
		PullRequest:       body.PRNumber,
		Sender:            "manual",
		RequestedReviewer: h.cfg.GiteaBotUsername,
		Manual:            true,
	}
	item, err := h.reviews.Enqueue(c, job, "manual")
	if err != nil {
		responses.LegacyError(c, http.StatusInternalServerError, "could not enqueue review")
		return
	}

	responses.Legacy(c, http.StatusAccepted, gin.H{
		"accepted":  true,
		"review_id": item.ID,
		"owner":     body.Owner,
		"repo":      body.Repo,
		"pr_number": body.PRNumber,
	})
}

// pending reads or changes a review awaiting manual publication. Supported
// actions are approve, reject, and rerun; an empty action returns its preview.
func (h *AdminHandler) pending(c *gin.Context) {
	id := c.Query("name")
	if id == "" {
		responses.LegacyError(c, http.StatusBadRequest, "name é obrigatório")
		return
	}

	pending, err := h.reviewRepo.Pending(c, id)
	if err == sql.ErrNoRows {
		responses.LegacyError(c, http.StatusNotFound, "não há review aguardando autorização")
		return
	}
	if err != nil {
		responses.LegacyError(c, http.StatusInternalServerError, "could not load pending review")
		return
	}

	switch c.Param("action") {
	case "":
		h.pendingPreview(c, id, pending)
	case "approve":
		h.approvePending(c, id, pending)
	case "reject":
		_ = h.reviewRepo.DeletePending(c, id)
		_ = h.reviewRepo.SetStatus(c, id, review.StatusCancelled, "review negada sem publicação")
		responses.Legacy(c, http.StatusOK, gin.H{"accepted": true, "action": "reject"})
	case "rerun":
		h.rerunPending(c, id, pending)
	default:
		responses.LegacyError(c, http.StatusNotFound, "not found")
	}
}

// pendingPreview returns the publication payload shown before manual approval.
func (h *AdminHandler) pendingPreview(c *gin.Context, id string, pending review.Pending) {
	comments := make([]gin.H, 0, len(pending.Result.Comments))
	for _, item := range pending.Result.Comments {
		comments = append(comments, gin.H{
			"path":         item.File,
			"new_position": item.Line,
			"body":         item.Comment,
		})
	}

	responses.Legacy(c, http.StatusOK, gin.H{
		"name":         id,
		"job":          pending.Job,
		"final_review": pending.Result.FinalReview,
		"publication": gin.H{
			"event":    pending.Result.FinalReview.GiteaEvent,
			"body":     pending.Result.FinalReview.Summary,
			"comments": comments,
		},
	})
}

// approvePending publishes a pending result to its resolved Gitea instance and
// marks the review as completed.
func (h *AdminHandler) approvePending(c *gin.Context, id string, pending review.Pending) {
	instanceID, err := h.resolveGiteaInstance(c, pending.Job)
	if err != nil {
		responses.LegacyError(c, http.StatusBadRequest, err.Error())
		return
	}

	client, err := h.savedGiteaByID(c, instanceID)
	if err != nil {
		responses.LegacyError(c, http.StatusBadRequest, err.Error())
		return
	}

	err = client.Publish(
		c,
		pending.Job.Owner,
		pending.Job.Repository,
		pending.Job.PullRequest,
		pending.Result,
	)
	if err != nil {
		responses.LegacyError(c, http.StatusBadGateway, "não foi possível publicar a revisão no Gitea")
		return
	}

	_ = h.reviewRepo.DeletePending(c, id)
	_ = h.reviewRepo.SetStatus(c, id, review.StatusCompleted, "")
	responses.Legacy(c, http.StatusOK, gin.H{"accepted": true, "action": "approve"})
}

// rerunPending removes the pending payload and republishes the original job.
func (h *AdminHandler) rerunPending(c *gin.Context, id string, pending review.Pending) {
	if err := h.reviewRepo.DeletePending(c, id); err != nil {
		responses.LegacyError(c, http.StatusInternalServerError, "could not reset pending review")
		return
	}

	pending.Job.ReviewID = id
	if err := h.reviews.EnqueueExisting(c, pending.Job); err != nil {
		responses.LegacyError(c, http.StatusServiceUnavailable, "não foi possível reexecutar a revisão")
		return
	}

	responses.Legacy(c, http.StatusAccepted, gin.H{"accepted": true, "action": "rerun"})
}
