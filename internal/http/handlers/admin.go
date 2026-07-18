package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"gitea-agents/internal/config"
	"gitea-agents/internal/http/responses"
	"gitea-agents/internal/integrations/gitea"
	"gitea-agents/internal/queue"
	"gitea-agents/internal/review"
	"gitea-agents/internal/security"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type AdminHandler struct {
	db         *sql.DB
	cfg        config.Config
	reviews    *review.Service
	reviewRepo *review.Repository
	observer   queue.Observer
}

var resources = map[string]string{"ai/providers": "ai_providers", "ai/connections": "ai_connections", "ai/models": "ai_models", "ai/model-parameters": "model_parameters", "review/profiles": "review_profiles", "review/prompts": "review_prompts", "review/policies": "review_policies", "gitea/instances": "gitea_instances", "repositories": "repositories"}

func NewAdminHandler(db *sql.DB, cfg config.Config, reviews *review.Service, repo *review.Repository, observer queue.Observer) *AdminHandler {
	return &AdminHandler{db: db, cfg: cfg, reviews: reviews, reviewRepo: repo, observer: observer}
}
func (h *AdminHandler) Register(r *gin.RouterGroup) {
	r.GET("/status", h.status)
	r.Any("/setup/:action", h.setup)
	r.GET("/observability/metrics", h.metrics)
	r.GET("/observability/logs", func(c *gin.Context) { responses.OK(c, 200, gin.H{"lines": []string{}, "available": false}) })
	r.GET("/observability/reviews", h.observabilityReviews)
	r.GET("/observability/progress", h.observabilityProgress)
	r.POST("/reviews/manual", h.manual)
	r.GET("/reviews/pending", h.pending)
	r.Any("/reviews/pending/:action", h.pending)
	r.POST("/gitea/instances/test", h.testGitea)
	r.POST("/gitea/instances/:id/test", h.testGitea)
	r.POST("/gitea/instances/:id/organizations", h.organizations)
	r.POST("/gitea/instances/:id/repositories", h.giteaRepositories)
	r.POST("/gitea/instances/:id/pull-requests", h.pullRequests)
	for path := range resources {
		h.resource(r, path)
	}
}
func (h *AdminHandler) resource(r *gin.RouterGroup, path string) {
	r.GET("/"+path, h.list)
	r.POST("/"+path, h.create)
	r.GET("/"+path+"/:id", h.one)
	r.PUT("/"+path+"/:id", h.update)
	r.PATCH("/"+path+"/:id", h.update)
	r.DELETE("/"+path+"/:id", h.delete)
	r.POST("/"+path+"/:id/set-default", h.setDefault)
	if path == "ai/connections" {
		r.POST("/"+path+"/:id/test", h.testConnection)
	}
}
func (h *AdminHandler) table(path string) (string, bool) {
	table, ok := resources[strings.Trim(path, "/")]
	return table, ok
}
func (h *AdminHandler) list(c *gin.Context) {
	table, ok := h.table(c.Param("path"))
	if !ok {
		table, ok = h.table(strings.TrimPrefix(c.Request.URL.Path, "/api/admin/"))
	}
	if !ok {
		responses.Fail(c, 404, "NOT_FOUND", "resource not found")
		return
	}
	rows, err := h.db.QueryContext(c, `SELECT * FROM `+table+` ORDER BY id`)
	if err != nil {
		responses.Fail(c, 500, "DATABASE_ERROR", "could not list resource")
		return
	}
	defer rows.Close()
	items, err := scanRows(rows)
	if err != nil {
		responses.Fail(c, 500, "DATABASE_ERROR", "could not read resource")
		return
	}
	responses.Legacy(c, 200, items)
}
func (h *AdminHandler) one(c *gin.Context) {
	table, ok := h.tableFromRequest(c)
	if !ok {
		return
	}
	cols, err := columns(c, h.db, table)
	if err != nil {
		responses.Fail(c, 500, "DATABASE_ERROR", "could not inspect resource")
		return
	}
	item, err := scanOne(h.db.QueryRowContext(c, `SELECT * FROM `+table+` WHERE id=?`, c.Param("id")), cols)
	if err == sql.ErrNoRows {
		responses.LegacyError(c, 404, "not found")
		return
	}
	if err != nil {
		responses.LegacyError(c, 500, "database error")
		return
	}
	responses.Legacy(c, 200, item)
}
func (h *AdminHandler) create(c *gin.Context) { h.write(c, 0) }
func (h *AdminHandler) update(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	h.write(c, id)
}
func (h *AdminHandler) write(c *gin.Context, id int64) {
	table, ok := h.tableFromRequest(c)
	if !ok {
		return
	}
	var payload map[string]any
	if err := c.ShouldBindJSON(&payload); err != nil {
		responses.LegacyError(c, 400, "invalid JSON")
		return
	}
	delete(payload, "id")
	delete(payload, "created_at")
	delete(payload, "updated_at")
	apiKey := stringValue(payload["api_key"])
	delete(payload, "api_key")
	delete(payload, "api_key_ciphertext")
	token := stringValue(payload["token"])
	delete(payload, "token")
	delete(payload, "token_ciphertext")
	cols, err := columns(c, h.db, table)
	if err != nil {
		responses.LegacyError(c, 500, "database error")
		return
	}
	allowed := map[string]bool{}
	for _, col := range cols {
		allowed[col] = true
	}
	if table == "ai_connections" {
		delete(payload, "requires_auth")
		if apiKey != "" {
			encrypted, e := security.Encrypt(apiKey)
			if e != nil {
				responses.LegacyError(c, 400, e.Error())
				return
			}
			payload["api_key_ciphertext"] = encrypted
		}
		if apiKey == "" {
			delete(payload, "api_key_ciphertext")
		}
	}
	if table == "gitea_instances" && token != "" {
		encrypted, e := security.Encrypt(token)
		if e != nil {
			responses.LegacyError(c, 400, e.Error())
			return
		}
		payload["token_ciphertext"] = encrypted
	}
	keys := make([]string, 0)
	for key := range payload {
		if allowed[key] {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		responses.LegacyError(c, 400, "no writable fields")
		return
	}
	args := make([]any, 0, len(keys))
	for _, key := range keys {
		args = append(args, payload[key])
	}
	var resultID int64
	if id == 0 {
		placeholders := strings.TrimRight(strings.Repeat("?,", len(keys)), ",")
		q := `INSERT INTO ` + table + ` (` + strings.Join(keys, ",") + ") VALUES (" + placeholders + ")"
		result, err := h.db.ExecContext(c, q, args...)
		if err != nil {
			responses.LegacyError(c, 400, "could not save resource")
			return
		}
		resultID, _ = result.LastInsertId()
	} else {
		sets := make([]string, len(keys))
		for i, key := range keys {
			sets[i] = key + "=?"
		}
		args = append(args, id)
		result, err := h.db.ExecContext(c, "UPDATE "+table+" SET "+strings.Join(sets, ",")+",updated_at=CURRENT_TIMESTAMP WHERE id=?", args...)
		if err != nil {
			responses.LegacyError(c, 400, "could not save resource")
			return
		}
		affected, _ := result.RowsAffected()
		if affected == 0 {
			responses.LegacyError(c, 404, "not found")
			return
		}
		resultID = id
	}
	h.oneByID(c, table, resultID, func(status int, value any) { responses.Legacy(c, status, value) })
}
func (h *AdminHandler) delete(c *gin.Context) {
	table, ok := h.tableFromRequest(c)
	if !ok {
		return
	}
	id := c.Param("id")
	if _, err := h.db.ExecContext(c, "DELETE FROM "+table+" WHERE id=?", id); err != nil {
		responses.LegacyError(c, 400, "could not delete resource")
		return
	}
	c.Status(http.StatusNoContent)
}
func (h *AdminHandler) oneByID(c *gin.Context, table string, id int64, done func(int, any)) {
	cols, err := columns(c, h.db, table)
	if err != nil {
		done(500, gin.H{"error": "database error"})
		return
	}
	item, err := scanOne(h.db.QueryRowContext(c, `SELECT * FROM `+table+` WHERE id=?`, id), cols)
	if err != nil {
		done(500, gin.H{"error": "database error"})
		return
	}
	done(http.StatusOK, item)
}
func (h *AdminHandler) setDefault(c *gin.Context) {
	table, ok := h.tableFromRequest(c)
	if !ok {
		return
	}
	id := c.Param("id")
	scope := ""
	scopeArgs := []any{}
	if table == "ai_connections" {
		scope = " AND provider_id=(SELECT provider_id FROM ai_connections WHERE id=?)"
		scopeArgs = append(scopeArgs, id)
	} else if table == "ai_models" {
		scope = " AND connection_id=(SELECT connection_id FROM ai_models WHERE id=?)"
		scopeArgs = append(scopeArgs, id)
	}
	if _, err := h.db.ExecContext(c, "UPDATE "+table+" SET is_default=0 WHERE is_default=1"+scope, scopeArgs...); err != nil {
		responses.LegacyError(c, 400, "could not update default")
		return
	}
	if _, err := h.db.ExecContext(c, "UPDATE "+table+" SET is_default=1 WHERE id=?", id); err != nil {
		responses.LegacyError(c, 400, "could not update default")
		return
	}
	h.one(c)
}
func (h *AdminHandler) tableFromRequest(c *gin.Context) (string, bool) {
	path := strings.TrimPrefix(c.Request.URL.Path, "/api/admin/")
	for key, table := range resources {
		if path == key || strings.HasPrefix(path, key+"/") {
			return table, true
		}
	}
	responses.Fail(c, 404, "NOT_FOUND", "resource not found")
	return "", false
}
func (h *AdminHandler) status(c *gin.Context) {
	counts := map[string]any{}
	for name, table := range map[string]string{"providers": "ai_providers", "connections": "ai_connections", "models": "ai_models", "profiles": "review_profiles", "prompts": "review_prompts", "policies": "review_policies", "repositories": "repositories"} {
		var n int
		_ = h.db.QueryRowContext(c, "SELECT count(*) FROM "+table).Scan(&n)
		counts[name] = n
	}
	responses.Legacy(c, 200, counts)
}
func (h *AdminHandler) metrics(c *gin.Context) {
	value := gin.H{"queue": gin.H{"connected": false, "stream_length": 0, "pending": 0, "workers": []any{}}, "reviews": gin.H{"total": 0, "bytes": 0}}
	if h.observer != nil {
		if metrics, err := h.observer.Metrics(c); err == nil {
			value["queue"] = metrics
		}
	}
	if items, err := h.reviewRepo.List(c, 100); err == nil {
		value["reviews"] = gin.H{"total": len(items), "bytes": 0}
	}
	responses.Legacy(c, 200, value)
}
func (h *AdminHandler) observabilityReviews(c *gin.Context) {
	items, err := h.reviewRepo.List(c, 100)
	if err != nil {
		responses.LegacyError(c, 500, "could not read reviews")
		return
	}
	out := make([]gin.H, 0, len(items))
	for _, item := range items {
		out = append(out, gin.H{"name": item.ID, "updated_at": item.UpdatedAt, "files": 0, "bytes": 0, "owner": item.Owner, "repo": item.Repository, "pr_number": item.PullRequest})
	}
	responses.Legacy(c, 200, out)
}
func (h *AdminHandler) observabilityProgress(c *gin.Context) {
	id := c.Query("name")
	if id == "" {
		items, _ := h.reviewRepo.List(c, 1)
		if len(items) > 0 {
			id = items[0].ID
		}
	}
	if id == "" {
		responses.Legacy(c, 200, gin.H{"active": false})
		return
	}
	item, err := h.reviewRepo.Get(c, id)
	if err != nil {
		responses.Legacy(c, 200, gin.H{"active": false})
		return
	}
	events := make([]gin.H, 0, len(item.Steps))
	for _, step := range item.Steps {
		stage, status, percent := progressMapping(step.Step, step.Status)
		events = append(events, gin.H{"stage": stage, "status": status, "percent": percent, "message": step.Message, "timestamp": step.StartedAt})
	}
	stage, status, percent := progressMapping(item.Status, "")
	if len(events) > 0 {
		last := events[len(events)-1]
		stage, status, percent = stringValue(last["stage"]), stringValue(last["status"]), intValue(last["percent"], 0)
	}
	active := item.Status != review.StatusCompleted && item.Status != review.StatusFailed && item.Status != review.StatusCancelled
	responses.Legacy(c, 200, gin.H{"active": active, "review": gin.H{"name": item.ID, "updated_at": item.UpdatedAt, "percent": percent, "stage": stage, "status": status, "message": item.Error, "events": events, "owner": item.Owner, "repo": item.Repository, "pr_number": item.PullRequest}})
}

func progressMapping(step, rawStatus string) (string, string, int) {
	stage := step
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
	status := rawStatus
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
	percent := map[string]int{"enfileirado": 5, "buscando_diff": 20, "enviando_para_ia": 45, "recebendo_resposta_parcial": 60, "agregando_resultado": 80, "pre-publicacao": 90, "publicando_comentario": 95, "publicacao": 100}[step]
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
func (h *AdminHandler) manual(c *gin.Context) {
	var body struct {
		InstanceID int64  `json:"instance_id"`
		Owner      string `json:"owner"`
		Repo       string `json:"repo"`
		PRNumber   int    `json:"pr_number"`
		URL        string `json:"url"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		responses.LegacyError(c, 400, "invalid JSON")
		return
	}
	if body.Owner == "" || body.Repo == "" || body.PRNumber <= 0 {
		responses.LegacyError(c, 400, "owner, repo e pr_number são obrigatórios")
		return
	}
	item, err := h.reviews.Enqueue(c, queue.ReviewJob{GiteaInstanceID: body.InstanceID, Owner: body.Owner, Repository: body.Repo, PullRequest: body.PRNumber, Sender: "manual", RequestedReviewer: h.cfg.GiteaBotUsername, Manual: true}, "manual")
	if err != nil {
		responses.LegacyError(c, 500, "could not enqueue review")
		return
	}
	responses.Legacy(c, 202, gin.H{"accepted": true, "review_id": item.ID, "owner": body.Owner, "repo": body.Repo, "pr_number": body.PRNumber})
}
func (h *AdminHandler) pending(c *gin.Context) {
	id := c.Query("name")
	if id == "" {
		responses.LegacyError(c, 400, "name é obrigatório")
		return
	}
	pending, err := h.reviewRepo.Pending(c, id)
	if err == sql.ErrNoRows {
		responses.LegacyError(c, 404, "não há review aguardando autorização")
		return
	}
	if err != nil {
		responses.LegacyError(c, 500, "could not load pending review")
		return
	}
	switch c.Param("action") {
	case "":
		comments := make([]gin.H, 0, len(pending.Result.Comments))
		for _, item := range pending.Result.Comments {
			comments = append(comments, gin.H{"path": item.File, "new_position": item.Line, "body": item.Comment})
		}
		responses.Legacy(c, 200, gin.H{"name": id, "job": pending.Job, "final_review": pending.Result.FinalReview, "publication": gin.H{"event": pending.Result.FinalReview.GiteaEvent, "body": pending.Result.FinalReview.Summary, "comments": comments}})
	case "approve":
		instanceID, err := h.resolveGiteaInstance(c, pending.Job)
		if err != nil {
			responses.LegacyError(c, 400, err.Error())
			return
		}
		client, err := h.savedGiteaByID(c, instanceID)
		if err != nil {
			responses.LegacyError(c, 400, err.Error())
			return
		}
		if err := client.Publish(c, pending.Job.Owner, pending.Job.Repository, pending.Job.PullRequest, pending.Result); err != nil {
			responses.LegacyError(c, 502, "não foi possível publicar a revisão no Gitea")
			return
		}
		_ = h.reviewRepo.DeletePending(c, id)
		_ = h.reviewRepo.SetStatus(c, id, review.StatusCompleted, "")
		responses.Legacy(c, 200, gin.H{"accepted": true, "action": "approve"})
	case "reject":
		_ = h.reviewRepo.DeletePending(c, id)
		_ = h.reviewRepo.SetStatus(c, id, review.StatusCancelled, "review negada sem publicação")
		responses.Legacy(c, 200, gin.H{"accepted": true, "action": "reject"})
	case "rerun":
		if err := h.reviewRepo.DeletePending(c, id); err != nil {
			responses.LegacyError(c, 500, "could not reset pending review")
			return
		}
		pending.Job.ReviewID = id
		if err := h.reviews.EnqueueExisting(c, pending.Job); err != nil {
			responses.LegacyError(c, 503, "não foi possível reexecutar a revisão")
			return
		}
		responses.Legacy(c, http.StatusAccepted, gin.H{"accepted": true, "action": "rerun"})
	default:
		responses.LegacyError(c, 404, "not found")
	}
}
func (h *AdminHandler) testConnection(c *gin.Context) {
	var provider, baseURL, ciphertext, authType string
	if err := h.db.QueryRowContext(c, `SELECT p.name,COALESCE(NULLIF(c.base_url,''),p.base_url),c.api_key_ciphertext,p.auth_type FROM ai_connections c JOIN ai_providers p ON p.id=c.provider_id WHERE c.id=? AND c.is_enabled=1`, c.Param("id")).Scan(&provider, &baseURL, &ciphertext, &authType); err != nil {
		responses.LegacyError(c, 404, "connection not found")
		return
	}
	key := ""
	if ciphertext != "" {
		key, _ = security.Decrypt(ciphertext)
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/api/tags"
	if provider == "openrouter" || authType == "bearer" {
		endpoint = strings.TrimRight(baseURL, "/") + "/models"
	}
	req, err := http.NewRequestWithContext(c, http.MethodGet, endpoint, nil)
	if err != nil {
		responses.LegacyError(c, 400, "endpoint inválido")
		return
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	res, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil || res.StatusCode < 200 || res.StatusCode >= 300 {
		if res != nil {
			_ = res.Body.Close()
		}
		responses.LegacyError(c, 502, "provider indisponível")
		return
	}
	_ = res.Body.Close()
	responses.Legacy(c, 200, gin.H{"ok": true, "provider": provider})
}
func (h *AdminHandler) setup(c *gin.Context) {
	switch c.Param("action") {
	case "catalog":
		h.catalog(c)
	case "complete":
		h.completeSetup(c)
	case "add-model":
		h.addModel(c)
	default:
		responses.LegacyError(c, 404, "not found")
	}
}

func (h *AdminHandler) catalog(c *gin.Context) {
	var input struct {
		Provider   string `json:"provider"`
		Connection struct {
			ID          int64  `json:"id"`
			BaseURL     string `json:"base_url"`
			APIKey      string `json:"api_key"`
			HTTPReferer string `json:"http_referer"`
			AppTitle    string `json:"app_title"`
		} `json:"connection"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		responses.LegacyError(c, 400, "invalid JSON")
		return
	}
	provider := input.Provider
	if provider == "ollama-cloud" {
		provider = "ollama"
	}
	if input.Connection.ID > 0 {
		var dbURL, ciphertext string
		if err := h.db.QueryRowContext(c, "SELECT base_url,api_key_ciphertext FROM ai_connections WHERE id=? AND is_enabled=1", input.Connection.ID).Scan(&dbURL, &ciphertext); err != nil {
			responses.LegacyError(c, 400, "connection not found")
			return
		}
		if input.Connection.BaseURL == "" {
			input.Connection.BaseURL = dbURL
		}
		if input.Connection.APIKey == "" && ciphertext != "" {
			input.Connection.APIKey, _ = security.Decrypt(ciphertext)
		}
	}
	if input.Connection.BaseURL == "" {
		if provider == "openrouter" {
			input.Connection.BaseURL = "https://openrouter.ai/api/v1"
		} else if input.Provider == "ollama-cloud" {
			input.Connection.BaseURL = "https://ollama.com"
		} else {
			input.Connection.BaseURL = "http://localhost:11434"
		}
	}
	endpoint := strings.TrimRight(input.Connection.BaseURL, "/")
	if provider == "openrouter" {
		endpoint += "/models?output_modalities=text"
	} else {
		endpoint += "/api/tags"
	}
	req, err := http.NewRequestWithContext(c, http.MethodGet, endpoint, nil)
	if err != nil {
		responses.LegacyError(c, 400, "endpoint inválido")
		return
	}
	if input.Connection.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+input.Connection.APIKey)
	}
	if input.Connection.HTTPReferer != "" {
		req.Header.Set("HTTP-Referer", input.Connection.HTTPReferer)
	}
	if input.Connection.AppTitle != "" {
		req.Header.Set("X-OpenRouter-Title", input.Connection.AppTitle)
	}
	res, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		responses.LegacyError(c, 502, "provider indisponível")
		return
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		responses.LegacyError(c, 502, fmt.Sprintf("provider respondeu HTTP %d", res.StatusCode))
		return
	}
	if provider == "openrouter" {
		var payload struct {
			Data []struct {
				ID                  string   `json:"id"`
				Name                string   `json:"name"`
				ContextLength       int      `json:"context_length"`
				SupportedParameters []string `json:"supported_parameters"`
				TopProvider         struct {
					MaxCompletionTokens int `json:"max_completion_tokens"`
				} `json:"top_provider"`
			} `json:"data"`
		}
		if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
			responses.LegacyError(c, 502, "catálogo inválido")
			return
		}
		models := make([]gin.H, 0, len(payload.Data))
		for _, item := range payload.Data {
			models = append(models, gin.H{"id": item.ID, "name": item.Name, "context_length": item.ContextLength, "max_completion_tokens": item.TopProvider.MaxCompletionTokens, "supported_parameters": item.SupportedParameters})
		}
		responses.Legacy(c, 200, gin.H{"provider": "openrouter", "connection_ok": true, "models": models})
		return
	}
	var payload struct {
		Models []struct {
			Name  string `json:"name"`
			Model string `json:"model"`
			Size  int64  `json:"size"`
		} `json:"models"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		responses.LegacyError(c, 502, "catálogo inválido")
		return
	}
	models := make([]gin.H, 0, len(payload.Models))
	for _, item := range payload.Models {
		id := item.Model
		if id == "" {
			id = item.Name
		}
		models = append(models, gin.H{"id": id, "name": item.Name, "context_length": 0, "max_completion_tokens": 0, "supported_parameters": []string{"temperature", "top_p", "repeat_penalty", "num_ctx", "num_predict", "keep_alive"}, "size": item.Size})
	}
	responses.Legacy(c, 200, gin.H{"provider": "ollama", "connection_ok": true, "models": models})
}
func (h *AdminHandler) completeSetup(c *gin.Context) {
	var d struct {
		Provider   string         `json:"provider"`
		Connection map[string]any `json:"connection"`
		Model      map[string]any `json:"model"`
		Parameters map[string]any `json:"parameters"`
		Profile    map[string]any `json:"profile"`
		Policy     map[string]any `json:"policy"`
	}
	if err := c.ShouldBindJSON(&d); err != nil {
		responses.LegacyError(c, 400, "invalid JSON")
		return
	}
	providerName := d.Provider
	if providerName == "ollama-cloud" {
		providerName = "ollama"
	}
	tx, err := h.db.BeginTx(c, nil)
	if err != nil {
		responses.LegacyError(c, 500, "could not start setup transaction")
		return
	}
	defer tx.Rollback()
	var providerID int64
	if err := tx.QueryRowContext(c, "SELECT id FROM ai_providers WHERE name=?", providerName).Scan(&providerID); err != nil {
		responses.LegacyError(c, 400, "provider not found")
		return
	}
	name := stringValue(d.Connection["name"])
	baseURL := stringValue(d.Connection["base_url"])
	apiKey := stringValue(d.Connection["api_key"])
	enc := ""
	if apiKey != "" {
		enc, err = security.Encrypt(apiKey)
		if err != nil {
			responses.LegacyError(c, 400, "não foi possível proteger a API key")
			return
		}
	}
	_, err = tx.ExecContext(c, "UPDATE ai_connections SET is_default=0 WHERE provider_id=? AND is_default=1", providerID)
	if err != nil {
		responses.LegacyError(c, 400, "could not update connection default")
		return
	}
	result, err := tx.ExecContext(c, `INSERT INTO ai_connections(provider_id,name,base_url,api_key_ciphertext,http_referer,app_title,is_default,is_enabled) VALUES(?,?,?,?,?,?,1,1)`, providerID, name, baseURL, enc, stringValue(d.Connection["http_referer"]), stringValue(d.Connection["app_title"]))
	if err != nil {
		responses.LegacyError(c, 400, "could not create connection")
		return
	}
	connectionID, _ := result.LastInsertId()
	modelID, err := h.insertModelExec(c, tx, connectionID, d.Model)
	if err != nil {
		responses.LegacyError(c, 400, err.Error())
		return
	}
	_, err = tx.ExecContext(c, `INSERT INTO model_parameters(model_id,temperature,top_p,timeout_seconds,keep_alive,unload_model_after_review) VALUES(?,?,?,?,?,?)`, modelID, floatValue(d.Parameters["temperature"]), floatValue(d.Parameters["top_p"]), intValue(d.Parameters["timeout_seconds"], h.cfg.ReviewRequestTimeoutSeconds), stringValue(d.Parameters["keep_alive"]), boolIntValue(d.Parameters["unload_model_after_review"]))
	if err != nil {
		responses.LegacyError(c, 400, "could not create model parameters")
		return
	}
	profileName := stringValue(d.Profile["name"])
	result, err = tx.ExecContext(c, `INSERT INTO review_profiles(name,description,model_id,is_default,is_enabled) VALUES(?,?,?,1,1)`, profileName, stringValue(d.Profile["description"]), modelID)
	if err != nil {
		responses.LegacyError(c, 400, "could not create profile")
		return
	}
	profileID, _ := result.LastInsertId()
	_, err = tx.ExecContext(c, `INSERT INTO review_policies(profile_id,max_block_chars,max_files_per_block,review_concurrency,review_final_retries,publish_manual_reviews,allow_autonomous_rejection) VALUES(?,?,?,?,?,?,?)`, profileID, intValue(d.Policy["max_block_chars"], h.cfg.ReviewMaxBlockChars), intValue(d.Policy["max_files_per_block"], h.cfg.ReviewMaxFilesPerBlock), intValue(d.Policy["review_concurrency"], 1), intValue(d.Policy["review_final_retries"], 5), boolIntValue(d.Policy["publish_manual_reviews"]), boolIntValue(d.Policy["allow_autonomous_rejection"]))
	if err != nil {
		responses.LegacyError(c, 400, "could not create review policy")
		return
	}
	if err = tx.Commit(); err != nil {
		responses.LegacyError(c, 500, "could not commit setup")
		return
	}
	responses.Legacy(c, 201, gin.H{"connection_id": connectionID, "model_id": modelID, "profile_id": profileID})
}
func (h *AdminHandler) addModel(c *gin.Context) {
	var d struct {
		ConnectionID int64          `json:"connection_id"`
		Model        map[string]any `json:"model"`
	}
	if err := c.ShouldBindJSON(&d); err != nil {
		responses.LegacyError(c, 400, "invalid JSON")
		return
	}
	id, err := h.insertModel(c, d.ConnectionID, d.Model)
	if err != nil {
		responses.LegacyError(c, 400, err.Error())
		return
	}
	responses.Legacy(c, 201, gin.H{"id": id})
}
func (h *AdminHandler) insertModel(c *gin.Context, connectionID int64, m map[string]any) (int64, error) {
	return h.insertModelExec(c, h.db, connectionID, m)
}
func (h *AdminHandler) insertModelExec(ctx context.Context, exec interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, connectionID int64, m map[string]any) (int64, error) {
	result, err := exec.ExecContext(ctx, `INSERT INTO ai_models(connection_id,provider_model_name,display_name,context_window,max_output_tokens,supports_json,is_enabled) VALUES(?,?,?,?,?,?,1)`, connectionID, stringValue(m["id"]), stringValue(m["name"]), intValue(m["context_length"], 0), intValue(m["max_completion_tokens"], 0), 1)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}
func (h *AdminHandler) testGitea(c *gin.Context) {
	var input struct {
		BaseURL     string `json:"base_url"`
		Token       string `json:"token"`
		BotUsername string `json:"bot_username"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		responses.LegacyError(c, 400, "invalid JSON")
		return
	}
	if input.Token == "" && c.Param("id") != "" {
		client, err := h.savedGitea(c)
		if err != nil {
			responses.LegacyError(c, 400, err.Error())
			return
		}
		if err := client.TestConnection(c); err != nil {
			responses.LegacyError(c, 502, "falha ao conectar ao Gitea")
			return
		}
		responses.Legacy(c, 200, gin.H{"ok": true})
		return
	}
	if strings.TrimSpace(input.BaseURL) == "" || strings.TrimSpace(input.Token) == "" {
		responses.LegacyError(c, 400, "base_url e token são obrigatórios")
		return
	}
	if err := gitea.New(input.BaseURL, input.Token).TestConnection(c); err != nil {
		responses.LegacyError(c, 502, "falha ao conectar ao Gitea")
		return
	}
	responses.Legacy(c, 200, gin.H{"ok": true, "bot_username": input.BotUsername})
}

func (h *AdminHandler) savedGitea(c *gin.Context) (*gitea.Client, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("instância Gitea inválida")
	}
	return h.savedGiteaByID(c, id)
}

func (h *AdminHandler) savedGiteaByID(c *gin.Context, id int64) (*gitea.Client, error) {
	var baseURL, ciphertext string
	if err := h.db.QueryRowContext(c, "SELECT base_url,token_ciphertext FROM gitea_instances WHERE id=? AND is_enabled=1", id).Scan(&baseURL, &ciphertext); err != nil {
		return nil, err
	}
	token, err := security.Decrypt(ciphertext)
	if err != nil || token == "" {
		return nil, fmt.Errorf("token Gitea indisponível")
	}
	return gitea.New(baseURL, token), nil
}

func (h *AdminHandler) resolveGiteaInstance(c *gin.Context, job queue.ReviewJob) (int64, error) {
	if job.GiteaInstanceID > 0 {
		return job.GiteaInstanceID, nil
	}
	var id int64
	err := h.db.QueryRowContext(c, `SELECT gi.id FROM gitea_instances gi JOIN repositories rep ON rep.gitea_instance_id=gi.id WHERE rep.owner=? AND rep.name=? AND gi.is_enabled=1 ORDER BY rep.id LIMIT 1`, job.Owner, job.Repository).Scan(&id)
	if err == sql.ErrNoRows {
		err = h.db.QueryRowContext(c, `SELECT id FROM gitea_instances WHERE is_enabled=1 ORDER BY is_default DESC, id LIMIT 1`).Scan(&id)
	}
	if err == sql.ErrNoRows {
		return 0, fmt.Errorf("nenhuma instância Gitea configurada")
	}
	return id, err
}

func (h *AdminHandler) organizations(c *gin.Context) {
	client, err := h.savedGitea(c)
	if err != nil {
		responses.LegacyError(c, 400, err.Error())
		return
	}
	items, err := client.ListOrganizations(c)
	if err != nil {
		responses.LegacyError(c, 502, "falha ao listar organizações")
		return
	}
	responses.Legacy(c, 200, items)
}

func (h *AdminHandler) giteaRepositories(c *gin.Context) {
	client, err := h.savedGitea(c)
	if err != nil {
		responses.LegacyError(c, 400, err.Error())
		return
	}
	var body struct {
		Organization string `json:"organization"`
		Repositories []struct {
			Owner    string `json:"owner"`
			Name     string `json:"name"`
			FullName string `json:"full_name"`
		} `json:"repositories"`
	}
	_ = c.ShouldBindJSON(&body)
	if body.Repositories != nil {
		tx, txErr := h.db.BeginTx(c, nil)
		if txErr == nil {
			_, txErr = tx.ExecContext(c, "DELETE FROM repositories WHERE gitea_instance_id=?", c.Param("id"))
			for _, item := range body.Repositories {
				if txErr != nil {
					break
				}
				full := item.FullName
				if full == "" {
					full = item.Owner + "/" + item.Name
				}
				_, txErr = tx.ExecContext(c, "INSERT INTO repositories(gitea_instance_id,owner,name,full_name) VALUES(?,?,?,?)", c.Param("id"), item.Owner, item.Name, full)
			}
			if txErr == nil {
				txErr = tx.Commit()
			} else {
				_ = tx.Rollback()
			}
		}
		if txErr != nil {
			responses.LegacyError(c, 400, "could not save repositories")
			return
		}
		responses.Legacy(c, 200, gin.H{"saved": len(body.Repositories)})
		return
	}
	items, err := client.ListRepositories(c, body.Organization)
	if err != nil {
		responses.LegacyError(c, 502, "falha ao listar repositórios")
		return
	}
	responses.Legacy(c, 200, items)
}

func (h *AdminHandler) pullRequests(c *gin.Context) {
	client, err := h.savedGitea(c)
	if err != nil {
		responses.LegacyError(c, 400, err.Error())
		return
	}
	var body struct {
		Owner string `json:"owner"`
		Repo  string `json:"repo"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Owner == "" || body.Repo == "" {
		responses.LegacyError(c, 400, "owner e repo são obrigatórios")
		return
	}
	items, err := client.ListPullRequests(c, body.Owner, body.Repo)
	if err != nil {
		responses.LegacyError(c, 502, "falha ao listar pull requests")
		return
	}
	responses.Legacy(c, 200, items)
}
func stringValue(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}
func intValue(v any, fallback int) int {
	n, err := strconv.Atoi(stringValue(v))
	if err != nil {
		return fallback
	}
	return n
}
func floatValue(v any) float64 { n, _ := strconv.ParseFloat(stringValue(v), 64); return n }
func boolIntValue(v any) int {
	value := strings.EqualFold(stringValue(v), "true") || stringValue(v) == "1"
	if value {
		return 1
	}
	return 0
}
func columns(ctx context.Context, db *sql.DB, table string) ([]string, error) {
	rows, err := db.QueryContext(ctx, "SELECT * FROM "+table+" LIMIT 0")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return rows.Columns()
}
func scanRows(rows *sql.Rows) ([]map[string]any, error) {
	cols, _ := rows.Columns()
	items := []map[string]any{}
	for rows.Next() {
		values := make([]any, len(cols))
		ptr := make([]any, len(cols))
		for i := range values {
			ptr[i] = &values[i]
		}
		if err := rows.Scan(ptr...); err != nil {
			return nil, err
		}
		items = append(items, cleanRow(cols, values))
	}
	return items, rows.Err()
}
func scanOne(row *sql.Row, cols []string) (map[string]any, error) {
	values := make([]any, len(cols))
	ptr := make([]any, len(cols))
	for i := range values {
		ptr[i] = &values[i]
	}
	if err := row.Scan(ptr...); err != nil {
		return nil, err
	}
	return cleanRow(cols, values), nil
}
func cleanRow(cols []string, values []any) map[string]any {
	item := map[string]any{}
	for i, key := range cols {
		if key == "api_key_ciphertext" {
			item["api_key_configured"] = stringValue(values[i]) != ""
			continue
		}
		if key == "token_ciphertext" {
			continue
		}
		if data, ok := values[i].([]byte); ok {
			item[key] = string(data)
		} else {
			item[key] = values[i]
		}
	}
	return item
}

var _ = json.Valid
var _ = time.Now
var _ = gitea.New
