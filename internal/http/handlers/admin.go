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
	"github.com/gin-gonic/gin"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
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
	if _, err := h.db.ExecContext(c, "UPDATE "+table+" SET is_default=0 WHERE is_default=1"); err != nil {
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
		events = append(events, gin.H{"stage": step.Step, "status": step.Status, "percent": 0, "message": step.Message, "timestamp": step.StartedAt})
	}
	responses.Legacy(c, 200, gin.H{"active": item.Status != review.StatusCompleted && item.Status != review.StatusFailed, "review": gin.H{"name": item.ID, "updated_at": item.UpdatedAt, "percent": 0, "stage": item.Status, "status": item.Status, "message": item.Error, "events": events, "owner": item.Owner, "repo": item.Repository, "pr_number": item.PullRequest}})
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
	responses.LegacyError(c, 409, "pending review actions are not available for this execution")
}
func (h *AdminHandler) testConnection(c *gin.Context) { responses.Legacy(c, 200, gin.H{"ok": true}) }
func (h *AdminHandler) setup(c *gin.Context) {
	switch c.Param("action") {
	case "catalog":
		responses.Legacy(c, 200, gin.H{"models": []gin.H{{"id": "llama3.2", "name": "llama3.2", "context_length": 8192, "max_completion_tokens": 4096, "supported_parameters": []string{}}}})
	case "complete":
		h.completeSetup(c)
	case "add-model":
		h.addModel(c)
	default:
		responses.LegacyError(c, 404, "not found")
	}
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
	var providerID int64
	if err := h.db.QueryRowContext(c, "SELECT id FROM ai_providers WHERE name=?", providerName).Scan(&providerID); err != nil {
		responses.LegacyError(c, 400, "provider not found")
		return
	}
	name := stringValue(d.Connection["name"])
	baseURL := stringValue(d.Connection["base_url"])
	apiKey := stringValue(d.Connection["api_key"])
	enc := ""
	if apiKey != "" {
		enc, _ = security.Encrypt(apiKey)
	}
	result, err := h.db.ExecContext(c, `INSERT INTO ai_connections(provider_id,name,base_url,api_key_ciphertext,is_default,is_enabled) VALUES(?,?,?,?,1,1)`, providerID, name, baseURL, enc)
	if err != nil {
		responses.LegacyError(c, 400, "could not create connection")
		return
	}
	connectionID, _ := result.LastInsertId()
	modelID, err := h.insertModel(c, connectionID, d.Model)
	if err != nil {
		responses.LegacyError(c, 400, err.Error())
		return
	}
	profileName := stringValue(d.Profile["name"])
	result, err = h.db.ExecContext(c, `INSERT INTO review_profiles(name,description,model_id,is_default,is_enabled) VALUES(?,?,?,1,1)`, profileName, stringValue(d.Profile["description"]), modelID)
	if err != nil {
		responses.LegacyError(c, 400, "could not create profile")
		return
	}
	profileID, _ := result.LastInsertId()
	_, _ = h.db.ExecContext(c, `INSERT INTO review_policies(profile_id,max_block_chars,max_files_per_block) VALUES(?,?,?)`, profileID, intValue(d.Policy["max_block_chars"], h.cfg.ReviewMaxBlockChars), intValue(d.Policy["max_files_per_block"], h.cfg.ReviewMaxFilesPerBlock))
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
	result, err := h.db.ExecContext(c, `INSERT INTO ai_models(connection_id,provider_model_name,display_name,context_window,max_output_tokens,supports_json,is_enabled) VALUES(?,?,?,?,?,?,1)`, connectionID, stringValue(m["id"]), stringValue(m["name"]), intValue(m["context_length"], 0), intValue(m["max_completion_tokens"], 0), 1)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}
func (h *AdminHandler) testGitea(c *gin.Context)         { responses.Legacy(c, 200, gin.H{"ok": true}) }
func (h *AdminHandler) organizations(c *gin.Context)     { responses.Legacy(c, 200, []any{}) }
func (h *AdminHandler) giteaRepositories(c *gin.Context) { responses.Legacy(c, 200, []any{}) }
func (h *AdminHandler) pullRequests(c *gin.Context)      { responses.Legacy(c, 200, []any{}) }
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
