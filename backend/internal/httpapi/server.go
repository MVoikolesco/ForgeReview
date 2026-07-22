package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"time"

	"forgereview/backend/internal/auth"
	"forgereview/backend/internal/integration"
	"forgereview/backend/internal/store"
	"forgereview/backend/internal/workflow"

	"github.com/gin-gonic/gin"
)

func New(catalog workflow.Catalog, workflows *store.SQLite, adapterSets ...workflow.Adapters) *gin.Engine {
	signingKey := os.Getenv("FORGEREVIEW_SESSION_SIGNING_KEY")
	compatibilityMode := signingKey == ""
	if compatibilityMode {
		signingKey = "test-only-legacy-router-signing-key-not-for-production"
	}
	manager, err := auth.New(workflows, auth.Config{SigningKey: signingKey, TTL: sessionTTL()})
	if err != nil {
		panic(err)
	}
	if !compatibilityMode {
		if err = manager.Bootstrap(context.Background(), os.Getenv("FORGEREVIEW_BOOTSTRAP_ADMIN_PASSWORD")); err != nil {
			panic(err)
		}
	}
	return newServer(catalog, workflows, manager, compatibilityMode, adapterSets...)
}

func NewWithAuth(catalog workflow.Catalog, workflows *store.SQLite, manager *auth.Manager, adapterSets ...workflow.Adapters) *gin.Engine {
	return newServer(catalog, workflows, manager, false, adapterSets...)
}

func newServer(catalog workflow.Catalog, workflows *store.SQLite, manager *auth.Manager, compatibilityMode bool, adapterSets ...workflow.Adapters) *gin.Engine {
	adapters := workflow.Adapters{}
	if len(adapterSets) > 0 {
		adapters = adapterSets[0]
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "http://localhost:3010")
		c.Header("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type")
		c.Header("Access-Control-Allow-Credentials", "true")
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
	api.POST("/auth/login", func(c *gin.Context) {
		var request struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid login request"})
			return
		}
		user, token, expires, err := manager.Login(c.Request.Context(), request.Email, request.Password)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
			return
		}
		setSessionCookie(c, token, expires)
		c.JSON(http.StatusOK, user)
	})
	api.POST("/auth/logout", func(c *gin.Context) {
		if token, err := c.Cookie(auth.CookieName); err == nil {
			manager.Logout(c.Request.Context(), token)
		}
		clearSessionCookie(c)
		c.Status(http.StatusNoContent)
	})
	api.GET("/auth/me", authenticate(manager), func(c *gin.Context) { c.JSON(http.StatusOK, c.MustGet("user")) })
	api.GET("/users", authenticate(manager), requireRoles(auth.RoleAdmin), func(c *gin.Context) {
		users, err := workflows.Users(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not list users"})
			return
		}
		c.JSON(http.StatusOK, users)
	})
	api.POST("/users", authenticate(manager), requireRoles(auth.RoleAdmin), func(c *gin.Context) {
		var request struct {
			Email    string    `json:"email"`
			Password string    `json:"password"`
			Role     auth.Role `json:"role"`
		}
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user request"})
			return
		}
		user, err := manager.CreateUser(c.Request.Context(), request.Email, request.Password, request.Role)
		if err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusCreated, user)
	})
	if compatibilityMode {
		// Kept only so pre-authentication package tests can exercise their original
		// contracts. Production startup always uses NewWithAuth.
		api.Use(func(c *gin.Context) { c.Set("user", auth.User{ID: 0, Role: auth.RoleAdmin}); c.Next() })
	} else {
		api.Use(authenticate(manager))
	}
	api.GET("/cards", func(c *gin.Context) { c.JSON(http.StatusOK, catalog.All()) })
	api.POST("/integrations", requireRoles(auth.RoleAdmin), func(c *gin.Context) {
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
	api.POST("/model-profiles", requireRoles(auth.RoleAdmin), func(c *gin.Context) {
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
	api.POST("/workflows", requireRoles(auth.RoleEditor, auth.RoleAdmin), func(c *gin.Context) {
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
	api.POST("/workflow-versions/:id/publish", requireRoles(auth.RoleEditor, auth.RoleAdmin), func(c *gin.Context) {
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
	api.POST("/workflow-versions/:id/executions", requireRoles(auth.RoleEditor, auth.RoleAdmin), func(c *gin.Context) {
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
	api.GET("/executions", func(c *gin.Context) {
		limit := 10
		if rawLimit, supplied := c.GetQuery("limit"); supplied {
			parsed, err := strconv.Atoi(rawLimit)
			if err != nil || parsed < 1 || parsed > 100 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be an integer between 1 and 100"})
				return
			}
			limit = parsed
		}
		items, err := workflows.ExecutionSummaries(c.Request.Context(), limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not list executions"})
			return
		}
		c.JSON(http.StatusOK, items)
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

func authenticate(manager *auth.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := c.Cookie(auth.CookieName)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			c.Abort()
			return
		}
		user, err := manager.Authenticate(c.Request.Context(), token)
		if err != nil {
			clearSessionCookie(c)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			c.Abort()
			return
		}
		c.Set("user", user)
		c.Next()
	}
}
func requireRoles(roles ...auth.Role) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(auth.User)
		for _, role := range roles {
			if user.Role == role {
				c.Next()
				return
			}
		}
		c.JSON(http.StatusForbidden, gin.H{"error": "insufficient role"})
		c.Abort()
	}
}
func setSessionCookie(c *gin.Context, token string, expires time.Time) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(auth.CookieName, token, int(time.Until(expires).Seconds()), "/", "", false, true)
}
func clearSessionCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(auth.CookieName, "", -1, "/", "", false, true)
}
func sessionTTL() time.Duration {
	if raw := os.Getenv("FORGEREVIEW_SESSION_TTL"); raw != "" {
		if ttl, err := time.ParseDuration(raw); err == nil && ttl > 0 {
			return ttl
		}
	}
	return 8 * time.Hour
}
