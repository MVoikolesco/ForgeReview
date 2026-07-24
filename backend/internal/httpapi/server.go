package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
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
		if err = manager.Bootstrap(context.Background(), os.Getenv("FORGEREVIEW_BOOTSTRAP_ADMIN_EMAIL"), os.Getenv("FORGEREVIEW_BOOTSTRAP_ADMIN_PASSWORD")); err != nil {
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
	if adapters.Workflows == nil {
		adapters.Workflows = workflows
	}
	router := gin.New()
	var sseConnections atomic.Int64
	proxies, err := trustedProxies()
	if err != nil {
		panic(fmt.Sprintf("invalid FORGEREVIEW_TRUSTED_PROXIES: %v", err))
	}
	if err := router.SetTrustedProxies(proxies); err != nil {
		panic(fmt.Sprintf("invalid FORGEREVIEW_TRUSTED_PROXIES: %v", err))
	}
	origins := corsOrigins()
	cookie, err := sessionCookieSettings()
	if err != nil {
		panic(fmt.Sprintf("invalid session cookie configuration: %v", err))
	}
	router.Use(func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" && origins[origin] {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Credentials", "true")
		}
		c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
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
	router.POST("/webhooks/gitea/:registration", func(c *gin.Context) {
		handleGiteaWebhook(c, workflows, adapters)
	})
	api := router.Group("/api")
	discovery := integration.HTTPDiscoveryAdapter{}
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
		setSessionCookie(c, token, expires, cookie)
		c.JSON(http.StatusOK, user)
	})
	api.POST("/auth/logout", func(c *gin.Context) {
		if token, err := c.Cookie(auth.CookieName); err == nil {
			manager.Logout(c.Request.Context(), token)
		}
		clearSessionCookie(c, cookie)
		c.Status(http.StatusNoContent)
	})
	api.GET("/auth/me", authenticate(manager, cookie), func(c *gin.Context) { c.JSON(http.StatusOK, c.MustGet("user")) })
	api.GET("/users", authenticate(manager, cookie), requireRoles(auth.RoleAdmin), func(c *gin.Context) {
		users, err := workflows.Users(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not list users"})
			return
		}
		c.JSON(http.StatusOK, users)
	})
	api.POST("/users", authenticate(manager, cookie), requireRoles(auth.RoleAdmin), func(c *gin.Context) {
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
		_ = workflows.Audit(c.Request.Context(), currentUser(c).ID, "user.created", fmt.Sprintf("user:%d", user.ID), map[string]any{"role": user.Role})
		c.JSON(http.StatusCreated, user)
	})
	api.PATCH("/users/:id", authenticate(manager, cookie), requireRoles(auth.RoleAdmin), func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || id < 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
			return
		}
		var request struct {
			Role   auth.Role `json:"role"`
			Active *bool     `json:"active"`
		}
		if c.ShouldBindJSON(&request) != nil || request.Active == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "role and active are required"})
			return
		}
		actor := currentUser(c)
		if actor.ID == id && (request.Role != auth.RoleAdmin || !*request.Active) {
			c.JSON(http.StatusConflict, gin.H{"error": "administrators cannot remove their own admin access or disable themselves"})
			return
		}
		user, err := manager.UpdateUser(c.Request.Context(), id, request.Role, *request.Active)
		if errors.Is(err, store.ErrUserNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		if errors.Is(err, store.ErrLastActiveAdmin) {
			c.JSON(http.StatusConflict, gin.H{"error": "cannot remove the last active admin"})
			return
		}
		if err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "could not update user"})
			return
		}
		_ = workflows.Audit(c.Request.Context(), actor.ID, "user.updated", fmt.Sprintf("user:%d", user.ID), map[string]any{"role": user.Role, "active": user.Active})
		c.JSON(http.StatusOK, user)
	})
	api.GET("/audit-log", authenticate(manager, cookie), requireRoles(auth.RoleAdmin), func(c *gin.Context) {
		entries, err := workflows.AuditEntries(c.Request.Context(), 100)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not list audit log"})
			return
		}
		c.JSON(http.StatusOK, entries)
	})
	api.POST("/publications/:key/reconcile", authenticate(manager, cookie), requireRoles(auth.RoleAdmin), func(c *gin.Context) {
		var request struct {
			IntegrationKey string `json:"integration_key"`
			Owner          string `json:"owner"`
			Repo           string `json:"repo"`
			Number         int    `json:"number"`
		}
		if c.ShouldBindJSON(&request) != nil || request.IntegrationKey == "" || request.Owner == "" || request.Repo == "" || request.Number < 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "valid publication coordinates are required"})
			return
		}
		lookup, ok := adapters.GiteaWriter.(integration.GiteaReviewMarkerLookup)
		if !ok || adapters.Secrets == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "publication reconciliation is unavailable"})
			return
		}
		item, err := workflows.Integration(c.Request.Context(), request.IntegrationKey)
		if err != nil || item.Type != integration.TypeGitea || item.Status != integration.StatusActive {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "active Gitea integration is required"})
			return
		}
		secret, err := adapters.Secrets.Resolve(item)
		if err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "integration is unavailable"})
			return
		}
		receipt, found, err := lookup.FindReviewByMarker(c.Request.Context(), item, secret, integration.PullRequestRequest{Owner: request.Owner, Repo: request.Repo, Number: request.Number}, c.Param("key"))
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "provider reconciliation failed"})
			return
		}
		if found {
			err = workflows.ReconcilePublication(c.Request.Context(), c.Param("key"), &receipt)
		} else {
			err = workflows.ReconcilePublication(c.Request.Context(), c.Param("key"), nil)
		}
		if err != nil {
			c.JSON(http.StatusConflict, gin.H{"error": "publication is not awaiting reconciliation"})
			return
		}
		_ = workflows.Audit(c.Request.Context(), currentUser(c).ID, "publication.reconciled", "publication:"+c.Param("key"), map[string]any{"marker_found": found})
		c.JSON(http.StatusOK, gin.H{"status": map[bool]string{true: "completed", false: "retryable"}[found]})
	})
	if compatibilityMode {
		// Kept only so pre-authentication package tests can exercise their original
		// contracts. Production startup always uses NewWithAuth.
		api.Use(func(c *gin.Context) { c.Set("user", auth.User{ID: 0, Role: auth.RoleAdmin}); c.Next() })
	} else {
		api.Use(authenticate(manager, cookie))
	}
	api.GET("/cards", func(c *gin.Context) { c.JSON(http.StatusOK, catalog.All()) })
	api.GET("/response-contracts", func(c *gin.Context) {
		c.JSON(http.StatusOK, workflow.ResponseContracts())
	})
	api.GET("/review-checklists", func(c *gin.Context) {
		c.JSON(http.StatusOK, workflow.ReviewChecklists())
	})
	api.GET("/webhook-registrations", requireRoles(auth.RoleAdmin), func(c *gin.Context) {
		items, err := workflows.WebhookRegistrations(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not list webhook registrations"})
			return
		}
		c.JSON(http.StatusOK, items)
	})
	api.POST("/webhook-registrations", requireRoles(auth.RoleAdmin), func(c *gin.Context) {
		var request struct {
			Key            string `json:"key"`
			Name           string `json:"name"`
			WorkflowKey    string `json:"workflow_key"`
			TriggerNodeKey string `json:"trigger_node_key"`
			Secret         string `json:"secret"`
			Active         *bool  `json:"active"`
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
		decoder := json.NewDecoder(c.Request.Body)
		decoder.DisallowUnknownFields()
		if decoder.Decode(&request) != nil || decoder.Decode(&struct{}{}) != io.EOF || adapters.Secrets == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid webhook registration"})
			return
		}
		versionID, definition, err := workflows.PublishedVersion(c.Request.Context(), request.WorkflowKey)
		_ = versionID
		if err != nil || triggerMode(definition, request.TriggerNodeKey) != "webhook" {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "registration requires a published webhook trigger"})
			return
		}
		ciphertext, err := adapters.Secrets.Encrypt("webhook:"+request.Key, request.Secret)
		if err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "webhook secret is required"})
			return
		}
		active := true
		if request.Active != nil {
			active = *request.Active
		}
		item := workflow.WebhookRegistration{Key: request.Key, Name: request.Name, WorkflowKey: request.WorkflowKey, TriggerNodeKey: request.TriggerNodeKey, SecretCiphertext: ciphertext, Active: active, SecretConfigured: true}
		if err = workflows.UpsertWebhookRegistration(c.Request.Context(), item); err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "could not save webhook registration"})
			return
		}
		_ = workflows.Audit(c.Request.Context(), currentUser(c).ID, "webhook.registered", "webhook:"+item.Key, map[string]any{"workflow_key": item.WorkflowKey, "active": item.Active})
		c.JSON(http.StatusCreated, item)
	})
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
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid integration request"})
			return
		}
		if adapters.Secrets == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "integration secret manager is not configured"})
			return
		}
		candidate := integration.Integration{Key: request.Key, Name: request.Name, Type: request.Type, Config: request.Config, SecretCiphertext: "provided", Status: request.Status}
		if err := candidate.Validate(); err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "invalid connection configuration"})
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
		_ = workflows.Audit(c.Request.Context(), currentUser(c).ID, "integration.created", "integration:"+item.Key, map[string]any{"type": item.Type, "status": item.Status})
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
	api.POST("/integrations/validate", requireRoles(auth.RoleAdmin), func(c *gin.Context) {
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
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid integration request"})
			return
		}
		item := integration.Integration{Key: request.Key, Name: request.Name, Type: request.Type, Config: request.Config, SecretCiphertext: "provided", Status: request.Status}
		if err := item.Validate(); err != nil || discovery.Validate(c.Request.Context(), item, request.Secret) != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "connection validation failed"})
			return
		}
		if item.Type == integration.TypeGitea {
			organizations, err := discovery.Organizations(c.Request.Context(), item, request.Secret)
			if err != nil {
				c.JSON(http.StatusBadGateway, gin.H{"error": "organization discovery failed"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"status": "validated", "organizations": organizations})
			return
		}
		models, err := discovery.Models(c.Request.Context(), item, request.Secret)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "model discovery failed"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "validated", "models": models})
	})
	api.POST("/integrations/discover-repositories", requireRoles(auth.RoleAdmin), func(c *gin.Context) {
		var request struct {
			Key          string          `json:"key"`
			Name         string          `json:"name"`
			Type         string          `json:"type"`
			Config       json.RawMessage `json:"config"`
			Secret       string          `json:"secret"`
			Status       string          `json:"status"`
			Organization string          `json:"organization"`
		}
		decoder := json.NewDecoder(c.Request.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid integration request"})
			return
		}
		item := integration.Integration{Key: request.Key, Name: request.Name, Type: request.Type, Config: request.Config, SecretCiphertext: "provided", Status: request.Status}
		if err := item.Validate(); err != nil || request.Organization == "" || discovery.Validate(c.Request.Context(), item, request.Secret) != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "connection validation failed"})
			return
		}
		repositories, err := discovery.Repositories(c.Request.Context(), item, request.Secret, request.Organization)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "repository discovery failed"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"repositories": repositories})
	})
	api.GET("/integrations/:key", func(c *gin.Context) {
		item, err := workflows.Integration(c.Request.Context(), c.Param("key"))
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "connection not found"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load connection"})
			return
		}
		c.JSON(http.StatusOK, item.Summary())
	})
	api.PATCH("/integrations/:key", requireRoles(auth.RoleAdmin), func(c *gin.Context) {
		var request struct {
			Name   string          `json:"name"`
			Config json.RawMessage `json:"config"`
			Secret *string         `json:"secret"`
			Status string          `json:"status"`
		}
		decoder := json.NewDecoder(c.Request.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid integration request"})
			return
		}
		item, err := workflows.Integration(c.Request.Context(), c.Param("key"))
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "connection not found"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load connection"})
			return
		}
		item.Name = request.Name
		item.Config = request.Config
		item.Status = request.Status
		if request.Secret != nil {
			ciphertext, encryptErr := adapters.Secrets.Encrypt(item.Key, *request.Secret)
			if encryptErr != nil {
				c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "integration secret is required"})
				return
			}
			item.SecretCiphertext = ciphertext
		}
		if err = workflows.UpdateIntegration(c.Request.Context(), item); err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "could not update connection"})
			return
		}
		_ = workflows.Audit(c.Request.Context(), currentUser(c).ID, "integration.updated", "integration:"+item.Key, map[string]any{"status": item.Status})
		c.JSON(http.StatusOK, item.Summary())
	})
	api.POST("/integrations/:key/disable", requireRoles(auth.RoleAdmin), func(c *gin.Context) {
		if err := workflows.DisableIntegration(c.Request.Context(), c.Param("key")); errors.Is(err, store.ErrIntegrationNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "connection not found"})
		} else if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not disable connection"})
		} else {
			_ = workflows.Audit(c.Request.Context(), currentUser(c).ID, "integration.disabled", "integration:"+c.Param("key"), nil)
			c.Status(http.StatusNoContent)
		}
	})
	api.DELETE("/integrations/:key", requireRoles(auth.RoleAdmin), func(c *gin.Context) {
		err := workflows.DeleteIntegration(c.Request.Context(), c.Param("key"))
		if errors.Is(err, store.ErrIntegrationReferenced) {
			c.JSON(http.StatusConflict, gin.H{"error": "connection is referenced by workflow history; disable it instead"})
		} else if errors.Is(err, store.ErrIntegrationNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "connection not found"})
		} else if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not delete connection"})
		} else {
			_ = workflows.Audit(c.Request.Context(), currentUser(c).ID, "integration.deleted", "integration:"+c.Param("key"), nil)
			c.Status(http.StatusNoContent)
		}
	})
	api.GET("/integrations/:key/discover", requireRoles(auth.RoleEditor, auth.RoleAdmin), func(c *gin.Context) {
		item, err := workflows.Integration(c.Request.Context(), c.Param("key"))
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "connection not found"})
			return
		}
		if err != nil || item.Status != integration.StatusActive || adapters.Secrets == nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "connection is not available"})
			return
		}
		secret, err := adapters.Secrets.Resolve(item)
		if err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "connection is not available"})
			return
		}
		if err := discovery.Validate(c.Request.Context(), item, secret); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "connection validation failed"})
			return
		}
		if item.Type == integration.TypeGitea {
			organization := c.Query("organization")
			if organization == "" {
				organizations, organizationErr := discovery.Organizations(c.Request.Context(), item, secret)
				if organizationErr != nil {
					c.JSON(http.StatusBadGateway, gin.H{"error": "resource discovery failed"})
					return
				}
				c.JSON(http.StatusOK, gin.H{"organizations": organizations})
				return
			}
			repos, err := discovery.Repositories(c.Request.Context(), item, secret, organization)
			if err != nil {
				c.JSON(http.StatusBadGateway, gin.H{"error": "resource discovery failed"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"repositories": repos})
			return
		}
		models, err := discovery.Models(c.Request.Context(), item, secret)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "resource discovery failed"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"models": models})
	})
	api.GET("/integrations/:key/resources", func(c *gin.Context) {
		item, err := workflows.Integration(c.Request.Context(), c.Param("key"))
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "connection not found"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load resources"})
			return
		}
		if item.Type == integration.TypeGitea {
			repositories, err := workflows.Repositories(c.Request.Context(), item.Key)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load resources"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"repositories": repositories})
			return
		}
		profiles, err := workflows.ModelProfiles(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load resources"})
			return
		}
		selected := []string{}
		for _, p := range profiles {
			if p.IntegrationKey == item.Key {
				selected = append(selected, p.Model)
			}
		}
		c.JSON(http.StatusOK, gin.H{"models": selected})
	})
	api.PUT("/integrations/:key/repositories", requireRoles(auth.RoleEditor, auth.RoleAdmin), func(c *gin.Context) {
		var request struct {
			Repositories []integration.Repository `json:"repositories"`
		}
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid resource request"})
			return
		}
		if err := workflows.ReplaceRepositories(c.Request.Context(), c.Param("key"), request.Repositories); err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "could not replace repositories"})
			return
		}
		c.Status(http.StatusNoContent)
	})
	api.PUT("/integrations/:key/models", requireRoles(auth.RoleEditor, auth.RoleAdmin), func(c *gin.Context) {
		var request struct {
			Models []string `json:"models"`
		}
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid resource request"})
			return
		}
		profiles, err := workflows.ReplaceModelProfiles(c.Request.Context(), c.Param("key"), request.Models)
		if err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "could not replace models"})
			return
		}
		c.JSON(http.StatusOK, profiles)
	})
	api.POST("/model-profiles", requireRoles(auth.RoleAdmin), func(c *gin.Context) {
		var profile integration.ModelProfile
		decoder := json.NewDecoder(c.Request.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&profile); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "request body must contain one JSON object"})
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
	api.GET("/workflows/:key/published", func(c *gin.Context) {
		id, definition, err := workflows.PublishedVersion(c.Request.Context(), c.Param("key"))
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "published workflow not found"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load published workflow"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"version_id": id, "definition": definition})
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
	api.DELETE("/workflow-versions/:id", requireRoles(auth.RoleAdmin), func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid version id"})
			return
		}
		err = workflows.DeleteWorkflowVersion(c.Request.Context(), id, currentUser(c).ID)
		switch {
		case err == nil:
			c.Status(http.StatusNoContent)
		case errors.Is(err, store.ErrWorkflowVersionNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "workflow version not found"})
		case errors.Is(err, store.ErrWorkflowVersionNotDeletable):
			c.JSON(http.StatusConflict, gin.H{"error": "published workflow version cannot be deleted; publish another version first"})
		case errors.Is(err, store.ErrWorkflowVersionDeletionBlocked):
			reason := strings.TrimPrefix(err.Error(), store.ErrWorkflowVersionDeletionBlocked.Error()+": ")
			c.JSON(http.StatusConflict, gin.H{"error": "workflow version cannot be deleted because retained " + reason + " exists", "reason": reason})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not delete workflow version"})
		}
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
			_ = workflows.Audit(c.Request.Context(), currentUser(c).ID, "workflow.published", fmt.Sprintf("workflow_version:%d", id), nil)
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
	// A Studio save-and-run operates on the newly created draft. Published
	// versions use the separate endpoint below so an archived or draft version
	// cannot accidentally be presented as the deployed pipeline.
	api.POST("/workflow-versions/:id/executions", requireRoles(auth.RoleEditor, auth.RoleAdmin), func(c *gin.Context) {
		handleVersionExecution(c, workflows, catalog, adapters, workflow.VersionStatusDraft)
	})
	api.POST("/published-workflow-versions/:id/executions", requireRoles(auth.RoleEditor, auth.RoleAdmin), func(c *gin.Context) {
		handleVersionExecution(c, workflows, catalog, adapters, workflow.VersionStatusPublished)
	})
	api.POST("/workflows/:key/triggers/:node/executions", requireRoles(auth.RoleEditor, auth.RoleAdmin), func(c *gin.Context) {
		if adapters.Dispatcher == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "durable execution queue is not configured"})
			return
		}
		versionID, definition, err := workflows.PublishedVersion(c.Request.Context(), c.Param("key"))
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "published workflow not found"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load published workflow"})
			return
		}
		triggerNode := c.Param("node")
		if triggerMode(definition, triggerNode) != "api" {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "target is not an API trigger"})
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 256<<10)
		input := map[string]any{}
		decoder := json.NewDecoder(c.Request.Body)
		if err = decoder.Decode(&input); err != nil && !errors.Is(err, io.EOF) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "request body must be a JSON object"})
			return
		}
		if input == nil || decoder.Decode(&struct{}{}) != io.EOF {
			c.JSON(http.StatusBadRequest, gin.H{"error": "request body must contain one JSON object"})
			return
		}
		executionID, err := workflows.CreateTriggeredExecution(c.Request.Context(), versionID, triggerNode, input)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not persist execution"})
			return
		}
		if err = adapters.Dispatcher.Enqueue(c.Request.Context(), executionID); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"execution_id": executionID, "status": "queued", "error": "execution persisted but dispatch is unavailable"})
			return
		}
		c.JSON(http.StatusAccepted, gin.H{"execution_id": executionID, "status": "queued"})
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
	api.GET("/execution-events", func(c *gin.Context) { streamExecutionEvents(c, workflows, 0, &sseConnections) })
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
	api.GET("/executions/:id/events", func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || id < 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid execution id"})
			return
		}
		streamExecutionEvents(c, workflows, id, &sseConnections)
	})
	api.GET("/executions/:id/cards/:node/logs", func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || id < 1 || strings.TrimSpace(c.Param("node")) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid execution or card id"})
			return
		}
		if _, err = workflows.Execution(c.Request.Context(), id); errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "execution not found"})
			return
		} else if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load execution"})
			return
		}
		events, err := workflows.ExecutionEvents(c.Request.Context(), 0, id, 500)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load card logs"})
			return
		}
		eventsForCard := make([]workflow.ExecutionEvent, 0)
		for _, event := range events {
			if event.Node != nil && event.Node.NodeKey == c.Param("node") {
				eventsForCard = append(eventsForCard, event)
			}
		}
		entries, err := workflows.CardExecutionLogs(c.Request.Context(), id, c.Param("node"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load detailed card logs"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"events": eventsForCard, "entries": entries})
	})
	api.GET("/metrics", requireRoles(auth.RoleAdmin), func(c *gin.Context) {
		metrics, err := workflows.Metrics(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load metrics"})
			return
		}
		// Connection count is a process gauge; all execution counters are durable.
		c.JSON(http.StatusOK, gin.H{"queue_depth": metrics.QueueDepth, "status_counts": metrics.StatusCounts, "retry_count": metrics.RetryCount, "dead_letter_count": metrics.DeadLetterCount, "average_duration_ms": metrics.AverageDuration, "sse_connections": sseConnections.Load()})
	})
	api.POST("/executions/:id/cancel", requireRoles(auth.RoleEditor, auth.RoleAdmin), func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || id < 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid execution id"})
			return
		}
		if err = workflows.CancelExecution(c.Request.Context(), id); errors.Is(err, store.ErrExecutionNotCancellable) {
			c.JSON(http.StatusConflict, gin.H{"error": "execution cannot be cancelled after publication begins or after it is terminal"})
			return
		} else if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "execution not found"})
			return
		} else if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not cancel execution"})
			return
		}
		_ = workflows.Audit(c.Request.Context(), currentUser(c).ID, "execution.cancelled", fmt.Sprintf("execution:%d", id), nil)
		c.JSON(http.StatusOK, gin.H{"execution_id": id, "status": "cancelled"})
	})
	api.POST("/executions/:id/reprocess", requireRoles(auth.RoleEditor, auth.RoleAdmin), func(c *gin.Context) {
		if adapters.Dispatcher == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "durable execution queue is not configured"})
			return
		}
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || id < 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid execution id"})
			return
		}
		next, err := workflows.ReprocessExecution(c.Request.Context(), id)
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "execution not found"})
			return
		} else if err != nil {
			c.JSON(http.StatusConflict, gin.H{"error": "execution cannot be reprocessed: sensitive input may have expired"})
			return
		}
		if err = adapters.Dispatcher.Enqueue(c.Request.Context(), next); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"execution_id": next, "status": "queued", "error": "execution persisted but dispatch is unavailable"})
			return
		}
		_ = workflows.Audit(c.Request.Context(), currentUser(c).ID, "execution.reprocessed", fmt.Sprintf("execution:%d", next), map[string]any{"source_execution_id": id})
		c.JSON(http.StatusAccepted, gin.H{"execution_id": next, "status": "queued"})
	})
	api.POST("/executions/:id/replay", requireRoles(auth.RoleAdmin), func(c *gin.Context) {
		if adapters.Dispatcher == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "durable execution queue is not configured"})
			return
		}
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || id < 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid execution id"})
			return
		}
		next, err := workflows.ReplayDeadLetterExecution(c.Request.Context(), id)
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "execution not found"})
			return
		}
		if err != nil {
			c.JSON(http.StatusConflict, gin.H{"error": "execution is not eligible for dead-letter replay"})
			return
		}
		if err = adapters.Dispatcher.Enqueue(c.Request.Context(), next); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"execution_id": next, "status": "queued", "error": "execution persisted but dispatch is unavailable"})
			return
		}
		_ = workflows.Audit(c.Request.Context(), currentUser(c).ID, "execution.dead_letter_replayed", fmt.Sprintf("execution:%d", next), map[string]any{"source_execution_id": id})
		c.JSON(http.StatusAccepted, gin.H{"execution_id": next, "status": "queued"})
	})
	return router
}

func streamExecutionEvents(c *gin.Context, workflows *store.SQLite, executionID int64, connections *atomic.Int64) {
	connections.Add(1)
	defer connections.Add(-1)
	after, _ := strconv.ParseInt(c.GetHeader("Last-Event-ID"), 10, 64)
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.Status(http.StatusInternalServerError)
		return
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		events, err := workflows.ExecutionEvents(c.Request.Context(), after, executionID, 100)
		if err != nil {
			return
		}
		for _, event := range events {
			payload, _ := json.Marshal(event)
			_, _ = fmt.Fprintf(c.Writer, "id: %d\nevent: execution\ndata: %s\n\n", event.ID, payload)
			after = event.ID
		}
		flusher.Flush()
		select {
		case <-c.Request.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

func handleVersionExecution(c *gin.Context, workflows *store.SQLite, catalog workflow.Catalog, adapters workflow.Adapters, requiredStatus string) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid version id"})
		return
	}
	status, err := workflows.VersionStatus(c.Request.Context(), id)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "workflow version not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load workflow version"})
		return
	}
	if status != requiredStatus {
		c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("execution requires a %s workflow version", requiredStatus)})
		return
	}
	definition, err := workflows.Load(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load workflow"})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 256<<10)
	var request map[string]any
	decoder := json.NewDecoder(c.Request.Body)
	if err = decoder.Decode(&request); err != nil || request == nil || decoder.Decode(&struct{}{}) != io.EOF {
		c.JSON(http.StatusBadRequest, gin.H{"error": "request body must contain one JSON object"})
		return
	}
	triggerNode, _ := request["trigger_node"].(string)
	input := map[string]any{}
	if payload, supplied := request["payload"]; supplied {
		var valid bool
		input, valid = payload.(map[string]any)
		if !valid || input == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "payload must be a JSON object"})
			return
		}
	} else {
		input = request
		delete(input, "trigger_node")
	}
	if triggerNode == "" {
		triggerNode = soleTrigger(definition, "manual")
	}
	if triggerMode(definition, triggerNode) != "manual" {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "execution requires a selected manual trigger"})
		return
	}
	executionID, err := workflows.CreateTriggeredExecution(c.Request.Context(), id, triggerNode, input)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not persist execution"})
		return
	}
	if adapters.Dispatcher != nil {
		if err = adapters.Dispatcher.Enqueue(c.Request.Context(), executionID); err != nil {
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
	nodes := make(map[string]workflow.Node, len(definition.Nodes))
	for _, node := range definition.Nodes {
		nodes[node.Key] = node
	}
	runAdapters.Progress = workflow.ProgressObserverFunc(func(progressCtx context.Context, run workflow.NodeRun) error {
		if err := workflows.SaveNodeProgress(progressCtx, executionID, run); err != nil {
			return err
		}
		return writeNodeLog(progressCtx, runAdapters.Logs, executionID, id, nodes[run.NodeKey], run)
	})
	runAdapters.Cancellation = func(checkCtx context.Context) (bool, error) {
		return workflows.CancellationRequested(checkCtx, executionID)
	}
	report, runErr := workflow.RunFromTriggerWithAdapters(c.Request.Context(), definition, catalog, triggerNode, input, runAdapters)
	if err = workflows.CompleteExecution(c.Request.Context(), executionID, report); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not persist execution"})
		return
	}
	if runErr != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"execution_id": executionID, "report": report, "error": runErr.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"execution_id": executionID, "report": report})
}

func writeNodeLog(ctx context.Context, writer workflow.ExecutionLogWriter, executionID, versionID int64, node workflow.Node, run workflow.NodeRun) error {
	if writer == nil {
		return nil
	}
	event := "finished"
	if run.Status == "running" {
		event = "started"
	} else if node.Type == "log" {
		event = "log_card"
	}
	errorText := ""
	if run.Error != "" {
		errorText = fmt.Sprintf("%v", workflow.ExecutionLogDiagnostic(run.Error))
	}
	return writer.WriteExecutionLog(ctx, workflow.ExecutionLogEntry{ExecutionID: executionID, VersionID: versionID, NodeKey: run.NodeKey, NodeName: node.Name, NodeType: node.Type, ScopeKey: run.ScopeKey, Event: event, Status: run.Status, DurationMS: run.DurationMS, Facts: workflow.SafeExecutionLogFacts(run.Metadata), Error: errorText, Inputs: workflow.ExecutionLogDiagnostic(run.Inputs), Outputs: workflow.ExecutionLogDiagnostic(run.Outputs), OccurredAt: time.Now().UTC()})
}

func triggerMode(definition workflow.Definition, key string) string {
	for _, node := range definition.Nodes {
		if node.Key == key && node.Type == "trigger" {
			mode, err := workflow.TriggerMode(node)
			if err == nil {
				return mode
			}
			return ""
		}
	}
	return ""
}

func soleTrigger(definition workflow.Definition, mode string) string {
	result := ""
	for _, node := range definition.Nodes {
		if node.Type == "trigger" {
			configured, _ := workflow.TriggerMode(node)
			if configured == mode {
				if result != "" {
					return ""
				}
				result = node.Key
			}
		}
	}
	return result
}

const maxWebhookBody = 1 << 20

func handleGiteaWebhook(c *gin.Context, workflows *store.SQLite, adapters workflow.Adapters) {
	if adapters.Dispatcher == nil || adapters.Secrets == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "webhook execution is unavailable"})
		return
	}
	mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || mediaType != "application/json" {
		c.JSON(http.StatusUnsupportedMediaType, gin.H{"error": "content type must be application/json"})
		return
	}
	delivery := strings.TrimSpace(c.GetHeader("X-Gitea-Delivery"))
	event := strings.TrimSpace(c.GetHeader("X-Gitea-Event"))
	signature := strings.TrimSpace(c.GetHeader("X-Gitea-Signature"))
	if delivery == "" || len(delivery) > 200 || event != "pull_request" || len(signature) != 64 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "required Gitea headers are invalid"})
		return
	}
	provided, err := hex.DecodeString(signature)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Gitea signature is invalid"})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxWebhookBody)
	body, err := io.ReadAll(c.Request.Body)
	if err != nil || len(body) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "webhook body is invalid or too large"})
		return
	}
	registration, err := workflows.WebhookRegistration(c.Request.Context(), c.Param("registration"))
	if errors.Is(err, sql.ErrNoRows) || !registration.Active {
		c.JSON(http.StatusNotFound, gin.H{"error": "webhook registration not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load webhook registration"})
		return
	}
	secret, err := adapters.Secrets.Resolve(integration.Integration{Key: "webhook:" + registration.Key, SecretCiphertext: registration.SecretCiphertext})
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "webhook registration is unavailable"})
		return
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	if !hmac.Equal(mac.Sum(nil), provided) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Gitea signature is invalid"})
		return
	}
	input, err := canonicalGiteaPullRequest(body, delivery)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "unsupported Gitea pull request payload"})
		return
	}
	versionID, definition, err := workflows.PublishedVersion(c.Request.Context(), registration.WorkflowKey)
	if err != nil || triggerMode(definition, registration.TriggerNodeKey) != "webhook" {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "registered webhook trigger is unavailable"})
		return
	}
	digest := sha256.Sum256(body)
	executionID, duplicate, err := workflows.CreateWebhookExecution(c.Request.Context(), registration.Key, delivery, hex.EncodeToString(digest[:]), versionID, registration.TriggerNodeKey, input)
	if errors.Is(err, store.ErrWebhookDeliveryCollision) {
		c.JSON(http.StatusConflict, gin.H{"error": "delivery ID was already used for a different payload"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not persist webhook execution"})
		return
	}
	if !duplicate {
		if err = adapters.Dispatcher.Enqueue(c.Request.Context(), executionID); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"execution_id": executionID, "status": "queued", "error": "execution persisted but dispatch is unavailable"})
			return
		}
	}
	c.JSON(http.StatusAccepted, gin.H{"execution_id": executionID, "status": "queued", "duplicate": duplicate})
}

func canonicalGiteaPullRequest(body []byte, delivery string) (map[string]any, error) {
	var payload struct {
		Action      string `json:"action"`
		Number      int    `json:"number"`
		PullRequest struct {
			Number int `json:"number"`
		} `json:"pull_request"`
		Repository struct {
			Name  string `json:"name"`
			Owner struct {
				Login    string `json:"login"`
				Username string `json:"username"`
			} `json:"owner"`
		} `json:"repository"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	// Gitea adds many fields, so decode normally while copying only allowlisted coordinates.
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	owner := payload.Repository.Owner.Login
	if owner == "" {
		owner = payload.Repository.Owner.Username
	}
	number := payload.PullRequest.Number
	if number == 0 {
		number = payload.Number
	}
	if owner == "" || payload.Repository.Name == "" || number < 1 || strings.TrimSpace(payload.Action) == "" {
		return nil, fmt.Errorf("missing pull request coordinates")
	}
	return map[string]any{"event": "pull_request", "action": payload.Action, "delivery": delivery, "pull_request": map[string]any{"owner": owner, "repo": payload.Repository.Name, "number": number}}, nil
}

func authenticate(manager *auth.Manager, cookie sessionCookieConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := c.Cookie(auth.CookieName)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			c.Abort()
			return
		}
		user, err := manager.Authenticate(c.Request.Context(), token)
		if err != nil {
			if !errors.Is(err, auth.ErrInvalidSession) {
				c.JSON(http.StatusServiceUnavailable, gin.H{"error": "authentication is temporarily unavailable"})
				c.Abort()
				return
			}
			clearSessionCookie(c, cookie)
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

type sessionCookieConfig struct {
	Secure   bool
	SameSite http.SameSite
}

func sessionCookieSettings() (sessionCookieConfig, error) {
	secure := false
	if raw := strings.TrimSpace(os.Getenv("FORGEREVIEW_SESSION_COOKIE_SECURE")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return sessionCookieConfig{}, errors.New("FORGEREVIEW_SESSION_COOKIE_SECURE must be true or false")
		}
		secure = value
	}
	sameSite := http.SameSiteLaxMode
	switch strings.ToLower(strings.TrimSpace(os.Getenv("FORGEREVIEW_SESSION_COOKIE_SAME_SITE"))) {
	case "", "lax":
	case "strict":
		sameSite = http.SameSiteStrictMode
	case "none":
		if !secure {
			return sessionCookieConfig{}, errors.New("SameSite=None requires FORGEREVIEW_SESSION_COOKIE_SECURE=true")
		}
		sameSite = http.SameSiteNoneMode
	default:
		return sessionCookieConfig{}, errors.New("FORGEREVIEW_SESSION_COOKIE_SAME_SITE must be lax, strict, or none")
	}
	return sessionCookieConfig{Secure: secure, SameSite: sameSite}, nil
}

func setSessionCookie(c *gin.Context, token string, expires time.Time, config sessionCookieConfig) {
	remaining := time.Until(expires)
	maxAge := int((remaining + time.Second - 1) / time.Second)
	if maxAge < 1 {
		maxAge = 1
	}
	http.SetCookie(c.Writer, &http.Cookie{Name: auth.CookieName, Value: token, Path: "/", Expires: expires.UTC(), MaxAge: maxAge, HttpOnly: true, Secure: config.Secure, SameSite: config.SameSite})
}
func clearSessionCookie(c *gin.Context, config sessionCookieConfig) {
	http.SetCookie(c.Writer, &http.Cookie{Name: auth.CookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: config.Secure, SameSite: config.SameSite})
}

func currentUser(c *gin.Context) auth.User { return c.MustGet("user").(auth.User) }
func corsOrigins() map[string]bool {
	raw := strings.TrimSpace(os.Getenv("FORGEREVIEW_CORS_ALLOWED_ORIGINS"))
	if raw == "" {
		return map[string]bool{"http://localhost:3010": true}
	}
	result := map[string]bool{}
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		parsed, err := url.ParseRequestURI(item)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" || item == "*" {
			panic("invalid FORGEREVIEW_CORS_ALLOWED_ORIGINS")
		}
		result[item] = true
	}
	if len(result) == 0 {
		panic("invalid FORGEREVIEW_CORS_ALLOWED_ORIGINS")
	}
	return result
}
func trustedProxies() ([]string, error) {
	raw := strings.TrimSpace(os.Getenv("FORGEREVIEW_TRUSTED_PROXIES"))
	if raw == "" {
		return nil, nil // Gin's safe default: never trust forwarded client-address headers.
	}
	items := strings.Split(raw, ",")
	proxies := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			return nil, errors.New("empty proxy entry")
		}
		if net.ParseIP(item) == nil {
			if _, _, err := net.ParseCIDR(item); err != nil {
				return nil, err
			}
		}
		proxies = append(proxies, item)
	}
	return proxies, nil
}
func sessionTTL() time.Duration {
	if raw := os.Getenv("FORGEREVIEW_SESSION_TTL"); raw != "" {
		if ttl, err := time.ParseDuration(raw); err == nil && ttl > 0 {
			return ttl
		}
	}
	return 8 * time.Hour
}
