package reviews

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"gitea-agents/internal/http/responses"
	"gitea-agents/internal/queue"
	"gitea-agents/internal/review"

	"github.com/gin-gonic/gin"
)

// ReviewHandler exposes authenticated versioned endpoints for review creation,
// inspection, cancellation, and reprocessing.
type ReviewHandler struct {
	service *review.Service
	repo    *review.Repository
}

// NewReviewHandler returns a review handler backed by service and repo.
func NewReviewHandler(service *review.Service, repo *review.Repository) *ReviewHandler {
	return &ReviewHandler{service: service, repo: repo}
}

// List writes the most recent reviews, honoring an optional bounded limit query.
func (h *ReviewHandler) List(c *gin.Context) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	items, err := h.repo.List(c.Request.Context(), limit)
	if err != nil {
		responses.Fail(c, http.StatusInternalServerError, "LIST_REVIEWS_FAILED", "could not list reviews")
		return
	}
	responses.OK(c, http.StatusOK, items)
}

// Get writes one persisted review, including its result and execution steps.
func (h *ReviewHandler) Get(c *gin.Context) {
	item, err := h.repo.Get(c.Request.Context(), c.Param("id"))
	if errors.Is(err, sql.ErrNoRows) {
		responses.Fail(c, http.StatusNotFound, "NOT_FOUND", "review not found")
		return
	}
	if err != nil {
		responses.Fail(c, http.StatusInternalServerError, "GET_REVIEW_FAILED", "could not load review")
		return
	}
	responses.OK(c, http.StatusOK, item)
}

// Steps writes the ordered execution steps for the requested review ID.
func (h *ReviewHandler) Steps(c *gin.Context) {
	steps, err := h.repo.Steps(c.Request.Context(), c.Param("id"))
	if errors.Is(err, sql.ErrNoRows) {
		responses.Fail(c, http.StatusNotFound, "NOT_FOUND", "review not found")
		return
	}
	if err != nil {
		responses.Fail(c, http.StatusInternalServerError, "GET_STEPS_FAILED", "could not load review steps")
		return
	}
	responses.OK(c, http.StatusOK, steps)
}

// Result writes a completed review result or reports that it is not ready.
func (h *ReviewHandler) Result(c *gin.Context) {
	item, err := h.repo.Get(c.Request.Context(), c.Param("id"))
	if errors.Is(err, sql.ErrNoRows) {
		responses.Fail(c, http.StatusNotFound, "NOT_FOUND", "review not found")
		return
	}
	if err != nil {
		responses.Fail(c, http.StatusInternalServerError, "GET_RESULT_FAILED", "could not load review result")
		return
	}
	if item.Result == nil {
		responses.Fail(c, http.StatusNotFound, "RESULT_NOT_READY", "review result is not ready")
		return
	}
	responses.OK(c, http.StatusOK, item.Result)
}

// Create validates a versioned review request, enqueues it, and writes the
// accepted persisted review.
func (h *ReviewHandler) Create(c *gin.Context) {
	var request struct {
		URL         string `json:"url"`
		Owner       string `json:"owner"`
		Repository  string `json:"repository"`
		PullRequest int    `json:"pull_request"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		responses.Fail(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON payload")
		return
	}
	owner, repo, number := request.Owner, request.Repository, request.PullRequest
	if owner == "" || repo == "" || number <= 0 {
		responses.Fail(c, http.StatusBadRequest, "VALIDATION_ERROR", "owner, repository and pull_request are required")
		return
	}
	job := queue.ReviewJob{
		Owner:             owner,
		Repository:        repo,
		PullRequest:       number,
		Sender:            "api",
		RequestedReviewer: "manual",
		Manual:            true,
	}
	item, err := h.service.Enqueue(c.Request.Context(), job, "api")
	if err != nil {
		responses.Fail(c, http.StatusInternalServerError, "ENQUEUE_FAILED", "could not enqueue review")
		return
	}
	responses.OK(c, http.StatusAccepted, item)
}

// Reprocess loads a review's original job and republishes it with the same ID.
func (h *ReviewHandler) Reprocess(c *gin.Context) {
	job, err := h.repo.Job(c.Request.Context(), c.Param("id"))
	if errors.Is(err, sql.ErrNoRows) {
		responses.Fail(c, http.StatusNotFound, "NOT_FOUND", "review not found")
		return
	}
	if err != nil {
		responses.Fail(c, http.StatusInternalServerError, "LOAD_JOB_FAILED", "could not load review job")
		return
	}
	job.ReviewID = c.Param("id")
	if err := h.service.EnqueueExisting(c.Request.Context(), job); err != nil {
		responses.Fail(c, http.StatusInternalServerError, "ENQUEUE_FAILED", "could not reprocess review")
		return
	}
	responses.OK(c, http.StatusAccepted, gin.H{"id": job.ReviewID, "status": review.StatusQueued})
}

// Cancel marks the requested review as cancelled and writes its new status.
func (h *ReviewHandler) Cancel(c *gin.Context) {
	cancelled, err := h.repo.Cancel(c.Request.Context(), c.Param("id"), "cancelled by operator")
	if err != nil {
		responses.Fail(c, http.StatusNotFound, "NOT_FOUND", "review not found")
		return
	}
	if !cancelled {
		responses.Fail(c, http.StatusConflict, "PUBLICATION_IN_PROGRESS", "review publication is already reserved")
		return
	}
	responses.OK(c, http.StatusOK, gin.H{"id": c.Param("id"), "status": review.StatusCancelled})
}
