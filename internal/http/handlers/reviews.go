package handlers

import (
	"database/sql"
	"errors"
	"gitea-agents/internal/http/responses"
	"gitea-agents/internal/queue"
	"gitea-agents/internal/review"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type ReviewHandler struct {
	service *review.Service
	repo    *review.Repository
}

func NewReviewHandler(service *review.Service, repo *review.Repository) *ReviewHandler {
	return &ReviewHandler{service: service, repo: repo}
}
func (h *ReviewHandler) List(c *gin.Context) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	items, err := h.repo.List(c.Request.Context(), limit)
	if err != nil {
		responses.Fail(c, 500, "LIST_REVIEWS_FAILED", "could not list reviews")
		return
	}
	responses.OK(c, 200, items)
}
func (h *ReviewHandler) Get(c *gin.Context) {
	item, err := h.repo.Get(c.Request.Context(), c.Param("id"))
	if errors.Is(err, sql.ErrNoRows) {
		responses.Fail(c, 404, "NOT_FOUND", "review not found")
		return
	}
	if err != nil {
		responses.Fail(c, 500, "GET_REVIEW_FAILED", "could not load review")
		return
	}
	responses.OK(c, 200, item)
}
func (h *ReviewHandler) Steps(c *gin.Context) {
	steps, err := h.repo.Steps(c.Request.Context(), c.Param("id"))
	if errors.Is(err, sql.ErrNoRows) {
		responses.Fail(c, 404, "NOT_FOUND", "review not found")
		return
	}
	if err != nil {
		responses.Fail(c, 500, "GET_STEPS_FAILED", "could not load review steps")
		return
	}
	responses.OK(c, 200, steps)
}
func (h *ReviewHandler) Result(c *gin.Context) {
	item, err := h.repo.Get(c.Request.Context(), c.Param("id"))
	if errors.Is(err, sql.ErrNoRows) {
		responses.Fail(c, 404, "NOT_FOUND", "review not found")
		return
	}
	if err != nil {
		responses.Fail(c, 500, "GET_RESULT_FAILED", "could not load review result")
		return
	}
	if item.Result == nil {
		responses.Fail(c, 404, "RESULT_NOT_READY", "review result is not ready")
		return
	}
	responses.OK(c, 200, item.Result)
}
func (h *ReviewHandler) Create(c *gin.Context) {
	var request struct {
		URL         string `json:"url"`
		Owner       string `json:"owner"`
		Repository  string `json:"repository"`
		PullRequest int    `json:"pull_request"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		responses.Fail(c, 400, "VALIDATION_ERROR", "invalid JSON payload")
		return
	}
	owner, repo, number := request.Owner, request.Repository, request.PullRequest
	if owner == "" || repo == "" || number <= 0 {
		responses.Fail(c, 400, "VALIDATION_ERROR", "owner, repository and pull_request are required")
		return
	}
	job := queue.ReviewJob{Owner: owner, Repository: repo, PullRequest: number, Sender: "api", RequestedReviewer: "manual", Manual: true}
	item, err := h.service.Enqueue(c.Request.Context(), job, "manual")
	if err != nil {
		responses.Fail(c, 500, "ENQUEUE_FAILED", "could not enqueue review")
		return
	}
	responses.OK(c, http.StatusAccepted, item)
}
func (h *ReviewHandler) Reprocess(c *gin.Context) {
	job, err := h.repo.Job(c.Request.Context(), c.Param("id"))
	if errors.Is(err, sql.ErrNoRows) {
		responses.Fail(c, 404, "NOT_FOUND", "review not found")
		return
	}
	if err != nil {
		responses.Fail(c, 500, "LOAD_JOB_FAILED", "could not load review job")
		return
	}
	job.ReviewID = c.Param("id")
	if err := h.service.EnqueueExisting(c.Request.Context(), job); err != nil {
		responses.Fail(c, 500, "ENQUEUE_FAILED", "could not reprocess review")
		return
	}
	responses.OK(c, http.StatusAccepted, gin.H{"id": job.ReviewID, "status": review.StatusQueued})
}
func (h *ReviewHandler) Cancel(c *gin.Context) {
	if err := h.repo.SetStatus(c.Request.Context(), c.Param("id"), review.StatusCancelled, "cancelled by operator"); err != nil {
		responses.Fail(c, 404, "NOT_FOUND", "review not found")
		return
	}
	responses.OK(c, 200, gin.H{"id": c.Param("id"), "status": review.StatusCancelled})
}
