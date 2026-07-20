package admin

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"gitea-agents/internal/http/responses"
	"gitea-agents/internal/review"
	"github.com/gin-gonic/gin"
)

func (h *AdminHandler) pipelines(c *gin.Context) {
	items, err := h.reviewRepo.AdminPipelines(c)
	if err != nil {
		pipelineError(c, err)
		return
	}
	responses.Legacy(c, http.StatusOK, items)
}
func (h *AdminHandler) pipeline(c *gin.Context) {
	id, ok := pipelineID(c)
	if !ok {
		return
	}
	item, err := h.reviewRepo.AdminPipeline(c, id, nil)
	if err != nil {
		pipelineError(c, err)
		return
	}
	responses.Legacy(c, http.StatusOK, item)
}
func (h *AdminHandler) createPipeline(c *gin.Context) {
	var input review.PipelineCreateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		responses.LegacyError(c, http.StatusBadRequest, "invalid JSON")
		return
	}
	item, err := h.reviewRepo.CreatePipelineDraft(c, input)
	if err != nil {
		pipelineError(c, err)
		return
	}
	responses.Legacy(c, http.StatusCreated, item)
}
func (h *AdminHandler) pipelineDraft(c *gin.Context) {
	id, ok := pipelineID(c)
	if !ok {
		return
	}
	version, ok := versionID(c)
	if !ok {
		return
	}
	item, err := h.reviewRepo.AdminPipeline(c, id, &version)
	if err != nil {
		pipelineError(c, err)
		return
	}
	if len(item.Versions) != 1 || item.Versions[0].Status != "draft" {
		responses.LegacyError(c, http.StatusBadRequest, "version is not a draft")
		return
	}
	responses.Legacy(c, http.StatusOK, item)
}
func (h *AdminHandler) updatePipelineDraft(c *gin.Context) {
	id, ok := pipelineID(c)
	if !ok {
		return
	}
	version, ok := versionID(c)
	if !ok {
		return
	}
	var input review.PipelineDraftInput
	if err := c.ShouldBindJSON(&input); err != nil {
		responses.LegacyError(c, http.StatusBadRequest, "invalid JSON")
		return
	}
	item, err := h.reviewRepo.UpdatePipelineDraft(c, id, version, input)
	if err != nil {
		pipelineError(c, err)
		return
	}
	responses.Legacy(c, http.StatusOK, item)
}
func (h *AdminHandler) validatePipelineDraft(c *gin.Context) {
	id, ok := pipelineID(c)
	if !ok {
		return
	}
	version, ok := versionID(c)
	if !ok {
		return
	}
	if err := h.reviewRepo.ValidatePipelineDraft(c, id, version); err != nil {
		pipelineError(c, err)
		return
	}
	responses.Legacy(c, http.StatusOK, gin.H{"valid": true})
}
func (h *AdminHandler) publishPipelineDraft(c *gin.Context) {
	id, ok := pipelineID(c)
	if !ok {
		return
	}
	version, ok := versionID(c)
	if !ok {
		return
	}
	item, err := h.reviewRepo.PublishPipelineDraft(c, id, version)
	if err != nil {
		pipelineError(c, err)
		return
	}
	responses.Legacy(c, http.StatusOK, item)
}
func (h *AdminHandler) clonePipeline(c *gin.Context) {
	id, ok := pipelineID(c)
	if !ok {
		return
	}
	source, _ := strconv.ParseInt(c.Query("version_id"), 10, 64)
	item, err := h.reviewRepo.ClonePublishedPipeline(c, id, source)
	if err != nil {
		pipelineError(c, err)
		return
	}
	responses.Legacy(c, http.StatusCreated, item)
}
func (h *AdminHandler) selectPipeline(c *gin.Context) {
	var profile *int64
	if raw := c.Query("profile_id"); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			responses.LegacyError(c, http.StatusBadRequest, "invalid profile_id")
			return
		}
		profile = &id
	}
	item, err := h.reviewRepo.SelectedPipeline(c, profile)
	if err != nil {
		pipelineError(c, err)
		return
	}
	responses.Legacy(c, http.StatusOK, item)
}
func (h *AdminHandler) selectPipelineForProfile(c *gin.Context) {
	id, ok := pipelineID(c)
	if !ok {
		return
	}
	item, err := h.reviewRepo.SelectPipeline(c, id)
	if err != nil {
		pipelineError(c, err)
		return
	}
	responses.Legacy(c, http.StatusOK, item)
}
func pipelineID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		responses.LegacyError(c, http.StatusBadRequest, "invalid pipeline id")
		return 0, false
	}
	return id, true
}
func versionID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("versionID"), 10, 64)
	if err != nil || id < 1 {
		responses.LegacyError(c, http.StatusBadRequest, "invalid version id")
		return 0, false
	}
	return id, true
}
func pipelineError(c *gin.Context, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		responses.LegacyError(c, http.StatusNotFound, "not found")
		return
	}
	responses.LegacyError(c, http.StatusBadRequest, err.Error())
}
