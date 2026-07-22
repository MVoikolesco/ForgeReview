package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"forgereview/backend/internal/integration"
	"forgereview/backend/internal/store"
	"forgereview/backend/internal/workflow"

	"github.com/gin-gonic/gin"
)

func New(catalog workflow.Catalog, workflows *store.SQLite, adapterSets ...workflow.Adapters) *gin.Engine {
	adapters := workflow.Adapters{}
	if len(adapterSets) > 0 {
		adapters = adapterSets[0]
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "http://localhost:3010")
		c.Header("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type")
		if c.Request.Method == http.MethodOptions {
			c.Status(http.StatusNoContent)
			c.Abort()
			return
		}
		c.Next()
	})
	router.Use(gin.Logger(), gin.Recovery())
	router.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	api := router.Group("/api")
	api.GET("/cards", func(c *gin.Context) { c.JSON(http.StatusOK, catalog.All()) })
	api.POST("/integrations", func(c *gin.Context) {
		var request struct {
			Key    string          `json:"key"`
			Name   string          `json:"name"`
			Type   string          `json:"type"`
			Config json.RawMessage `json:"config"`
			Secret string          `json:"secret"`
			Status string          `json:"status"`
		}
		decoder := json.NewDecoder(c.Request.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if adapters.Secrets == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "integration secret manager is not configured"})
			return
		}
		ciphertext, err := adapters.Secrets.Encrypt(request.Key, request.Secret)
		if err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "integration secret is required"})
			return
		}
		item := integration.Integration{Key: request.Key, Name: request.Name, Type: request.Type, Config: request.Config, SecretCiphertext: ciphertext, Status: request.Status}
		if err := workflows.CreateIntegration(c.Request.Context(), item); err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusCreated, item.Summary())
	})
	api.GET("/integrations", func(c *gin.Context) {
		items, err := workflows.Integrations(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not list integrations"})
			return
		}
		summaries := make([]integration.Summary, 0, len(items))
		for _, item := range items {
			summaries = append(summaries, item.Summary())
		}
		c.JSON(http.StatusOK, summaries)
	})
	api.POST("/model-profiles", func(c *gin.Context) {
		var profile integration.ModelProfile
		decoder := json.NewDecoder(c.Request.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&profile); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if err := workflows.CreateModelProfile(c.Request.Context(), profile); err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusCreated, profile)
	})
	api.GET("/model-profiles", func(c *gin.Context) {
		profiles, err := workflows.ModelProfiles(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not list model profiles"})
			return
		}
		c.JSON(http.StatusOK, profiles)
	})
	api.POST("/workflows", func(c *gin.Context) {
		var definition workflow.Definition
		if err := c.ShouldBindJSON(&definition); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if err := workflow.Validate(definition, catalog); err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
			return
		}
		id, err := workflows.Save(c.Request.Context(), definition)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not save workflow"})
			return
		}
		c.JSON(http.StatusCreated, gin.H{"version_id": id, "status": "draft"})
	})
	api.GET("/workflows", func(c *gin.Context) {
		items, err := workflows.ListDefinitions(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not list workflows"})
			return
		}
		c.JSON(http.StatusOK, items)
	})
	api.GET("/workflow-versions/:id", func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid version id"})
			return
		}
		definition, err := workflows.Load(c.Request.Context(), id)
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "workflow version not found"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load workflow"})
			return
		}
		c.JSON(http.StatusOK, definition)
	})
	api.POST("/workflow-versions/:id/publish", func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid version id"})
			return
		}
		summary, err := workflows.Publish(c.Request.Context(), id, catalog)
		switch {
		case err == nil:
			c.JSON(http.StatusOK, summary)
		case errors.Is(err, store.ErrWorkflowVersionNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "workflow draft not found"})
		case errors.Is(err, store.ErrWorkflowVersionNotDraft):
			c.JSON(http.StatusConflict, gin.H{"error": "workflow version is not a draft"})
		case errors.Is(err, store.ErrInvalidWorkflowVersion):
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not publish workflow"})
		}
	})
	api.POST("/workflow-versions/:id/executions", func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid version id"})
			return
		}
		definition, err := workflows.Load(c.Request.Context(), id)
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "workflow version not found"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load workflow"})
			return
		}
		var input map[string]any
		if err = c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		executionID, err := workflows.CreateExecution(c.Request.Context(), id, input)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not persist execution"})
			return
		}
		if adapters.Dispatcher != nil {
			if err = adapters.Dispatcher.Enqueue(c.Request.Context(), executionID); err != nil {
				_ = workflows.FailQueuedExecution(c.Request.Context(), executionID)
				c.JSON(http.StatusServiceUnavailable, gin.H{"execution_id": executionID, "error": "could not dispatch execution"})
				return
			}
			c.JSON(http.StatusAccepted, gin.H{"execution_id": executionID, "status": "queued"})
			return
		}
		if _, claimed, claimErr := workflows.ClaimExecution(c.Request.Context(), executionID); claimErr != nil || !claimed {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not start execution"})
			return
		}
		runAdapters := adapters
		runAdapters.Execution = workflow.ExecutionContext{ID: executionID, VersionID: id}
		report, runErr := workflow.RunWithAdapters(c.Request.Context(), definition, catalog, input, runAdapters)
		if err = workflows.CompleteExecution(c.Request.Context(), executionID, report); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not persist execution"})
			return
		}
		if runErr != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"execution_id": executionID, "report": report, "error": runErr.Error()})
			return
		}
		c.JSON(http.StatusCreated, gin.H{"execution_id": executionID, "report": report})
	})
	api.GET("/executions/:id", func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid execution id"})
			return
		}
		report, err := workflows.Execution(c.Request.Context(), id)
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "execution not found"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load execution"})
			return
		}
		c.JSON(http.StatusOK, report)
	})
	return router
}
