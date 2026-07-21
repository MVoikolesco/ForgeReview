package httpapi

import (
	"database/sql"
	"net/http"
	"strconv"

	"forgereview/backend/internal/store"
	"forgereview/backend/internal/workflow"
	"github.com/gin-gonic/gin"
)

func New(catalog workflow.Catalog, workflows *store.SQLite) *gin.Engine {
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())
	router.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	api := router.Group("/api")
	api.GET("/cards", func(c *gin.Context) { c.JSON(http.StatusOK, catalog.All()) })
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
	return router
}
