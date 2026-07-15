// Package admin exposes administrative configuration only. Secrets are referenced
// by environment-variable name and are never read back through CRUD responses.
package admin

import (
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"gitea-agents/internal/queue"
	"gitea-agents/internal/store"
)

type Handler struct {
	db                 *sql.DB
	username, password string
	http               *http.Client
	observer           queue.Observer
	logDir             string
}

var resources = map[string]string{
	"ai/providers": "ai_providers", "ai/connections": "ai_connections", "ai/models": "ai_models", "ai/model-parameters": "model_parameters",
	"review/profiles": "review_profiles", "review/prompts": "review_prompts", "review/policies": "review_policies",
	"gitea/instances": "gitea_instances", "repositories": "repositories",
}

func Register(mux *http.ServeMux, s *store.Store, username, password string, optional ...any) {
	h := Handler{db: s.DB, username: username, password: password, http: &http.Client{Timeout: 15 * time.Second}, logDir: "/logs/diffs"}
	for _, value := range optional {
		if observer, ok := value.(queue.Observer); ok {
			h.observer = observer
		}
		if dir, ok := value.(string); ok && dir != "" {
			h.logDir = dir
		}
	}
	mux.HandleFunc("/api/admin/", h.authorize(h.route))
}
func (h Handler) authorize(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || subtle.ConstantTimeCompare([]byte(u), []byte(h.username)) != 1 || subtle.ConstantTimeCompare([]byte(p), []byte(h.password)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="ForgeReview admin"`)
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		next(w, r)
	}
}

func (h Handler) route(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/admin/"), "/")
	if path == "status" && r.Method == http.MethodGet {
		h.status(w, r)
		return
	}
	if strings.HasPrefix(path, "setup/") {
		h.setupRoute(w, r, strings.TrimPrefix(path, "setup/"))
		return
	}
	if strings.HasPrefix(path, "observability/") {
		h.observabilityRoute(w, r, strings.TrimPrefix(path, "observability/"))
		return
	}
	key, table, rest := matchResource(path)
	if table == "" {
		http.NotFound(w, r)
		return
	}
	if key == "review/prompts" && r.Method != http.MethodGet {
		writeError(w, http.StatusForbidden, "prompts são internos e não podem ser alterados pelo painel")
		return
	}
	parts := []string{}
	if rest != "" {
		parts = strings.Split(rest, "/")
	}
	var id int64
	if len(parts) > 0 {
		id, _ = strconv.ParseInt(parts[0], 10, 64)
	}
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}
	if key == "ai/connections" && id > 0 && action == "test" && r.Method == http.MethodPost {
		h.testConnection(w, r, id)
		return
	}
	if action == "set-default" && r.Method == http.MethodPost {
		h.setDefault(w, r, table, id)
		return
	}
	switch r.Method {
	case http.MethodGet:
		if id > 0 {
			h.one(w, r, table, id)
		} else {
			h.list(w, r, table)
		}
	case http.MethodPost:
		if id > 0 {
			writeError(w, 400, "unexpected id")
			return
		}
		h.create(w, r, table)
	case http.MethodPut, http.MethodPatch:
		h.update(w, r, table, id)
	case http.MethodDelete:
		h.delete(w, r, table, id)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
func matchResource(path string) (string, string, string) {
	keys := make([]string, 0, len(resources))
	for k := range resources {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
	for _, k := range keys {
		if path == k {
			return k, resources[k], ""
		}
		if strings.HasPrefix(path, k+"/") {
			return k, resources[k], strings.TrimPrefix(path, k+"/")
		}
	}
	return "", "", ""
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
func (h Handler) status(w http.ResponseWriter, r *http.Request) {
	m := map[string]any{}
	for k, t := range map[string]string{"providers": "ai_providers", "connections": "ai_connections", "models": "ai_models", "parameters": "model_parameters", "profiles": "review_profiles", "prompts": "review_prompts", "policies": "review_policies", "repositories": "repositories"} {
		var n int
		if err := h.db.QueryRowContext(r.Context(), "SELECT count(*) FROM "+t).Scan(&n); err != nil {
			writeError(w, 500, err.Error())
			return
		}
		m[k] = n
	}
	var provider, model, profile sql.NullString
	_ = h.db.QueryRowContext(r.Context(), `SELECT ap.display_name,am.display_name,rp.name FROM review_profiles rp JOIN ai_models am ON am.id=rp.model_id AND am.is_enabled=1 JOIN ai_connections ac ON ac.id=am.connection_id AND ac.is_enabled=1 JOIN ai_providers ap ON ap.id=ac.provider_id AND ap.is_enabled=1 JOIN model_parameters mp ON mp.model_id=am.id JOIN review_policies pol ON pol.profile_id=rp.id WHERE rp.is_default=1 AND rp.is_enabled=1 LIMIT 1`).Scan(&provider, &model, &profile)
	m["default_provider"] = provider.String
	m["default_model"] = model.String
	m["default_profile"] = profile.String
	m["configured"] = profile.Valid
	writeJSON(w, 200, m)
}
func (h Handler) list(w http.ResponseWriter, r *http.Request, t string) {
	rows, e := h.db.QueryContext(r.Context(), "SELECT * FROM "+t+" ORDER BY id")
	if e != nil {
		writeError(w, 500, e.Error())
		return
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	items := []map[string]any{}
	for rows.Next() {
		m, e := scanRow(rows, cols)
		if e != nil {
			writeError(w, 500, e.Error())
			return
		}
		items = append(items, m)
	}
	writeJSON(w, 200, items)
}
func (h Handler) one(w http.ResponseWriter, r *http.Request, t string, id int64) {
	cols, e := h.columns(r, t)
	if e != nil {
		writeError(w, 500, e.Error())
		return
	}
	row := h.db.QueryRowContext(r.Context(), "SELECT * FROM "+t+" WHERE id=?", id)
	m, e := scanSingle(row, cols)
	if e == sql.ErrNoRows {
		writeError(w, 404, "not found")
		return
	}
	if e != nil {
		writeError(w, 500, e.Error())
		return
	}
	writeJSON(w, 200, m)
}
func scanRow(rows *sql.Rows, cols []string) (map[string]any, error) {
	v := make([]any, len(cols))
	p := make([]any, len(cols))
	for i := range v {
		p[i] = &v[i]
	}
	if err := rows.Scan(p...); err != nil {
		return nil, err
	}
	return rowMap(cols, v), nil
}
func scanSingle(row *sql.Row, cols []string) (map[string]any, error) {
	v := make([]any, len(cols))
	p := make([]any, len(cols))
	for i := range v {
		p[i] = &v[i]
	}
	if err := row.Scan(p...); err != nil {
		return nil, err
	}
	return rowMap(cols, v), nil
}
func rowMap(cols []string, v []any) map[string]any {
	m := map[string]any{}
	for i, c := range cols {
		if b, ok := v[i].([]byte); ok {
			m[c] = string(b)
		} else {
			m[c] = v[i]
		}
	}
	return m
}
func (h Handler) columns(r *http.Request, t string) ([]string, error) {
	rows, e := h.db.QueryContext(r.Context(), "SELECT * FROM "+t+" LIMIT 0")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	return rows.Columns()
}
func (h Handler) create(w http.ResponseWriter, r *http.Request, t string) {
	var m map[string]any
	if json.NewDecoder(r.Body).Decode(&m) != nil {
		writeError(w, 400, "invalid JSON")
		return
	}
	h.write(w, r, t, 0, m)
}
func (h Handler) update(w http.ResponseWriter, r *http.Request, t string, id int64) {
	if id <= 0 {
		writeError(w, 400, "id required")
		return
	}
	var m map[string]any
	if json.NewDecoder(r.Body).Decode(&m) != nil {
		writeError(w, 400, "invalid JSON")
		return
	}
	h.write(w, r, t, id, m)
}
func (h Handler) write(w http.ResponseWriter, r *http.Request, t string, id int64, m map[string]any) {
	delete(m, "id")
	delete(m, "api_key_ciphertext")
	delete(m, "created_at")
	delete(m, "updated_at")
	cols, e := h.columns(r, t)
	if e != nil {
		writeError(w, 500, e.Error())
		return
	}
	allowed := map[string]bool{}
	for _, c := range cols {
		allowed[c] = true
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		if allowed[k] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		writeError(w, 400, "no writable fields")
		return
	}
	args := make([]any, 0, len(keys)+1)
	for _, k := range keys {
		args = append(args, m[k])
	}
	if id == 0 {
		q := "INSERT INTO " + t + " (" + strings.Join(keys, ",") + ") VALUES (" + strings.TrimRight(strings.Repeat("?,", len(keys)), ",") + ")"
		res, e := h.db.ExecContext(r.Context(), q, args...)
		if e != nil {
			writeError(w, 400, e.Error())
			return
		}
		id, _ = res.LastInsertId()
		h.respondOne(w, r, t, id, http.StatusCreated)
		return
	}
	sets := make([]string, len(keys))
	for i, k := range keys {
		sets[i] = k + "=?"
	}
	args = append(args, id)
	res, e := h.db.ExecContext(r.Context(), "UPDATE "+t+" SET "+strings.Join(sets, ",")+",updated_at=CURRENT_TIMESTAMP WHERE id=?", args...)
	if e != nil {
		writeError(w, 400, e.Error())
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeError(w, 404, "not found")
		return
	}
	h.respondOne(w, r, t, id, 200)
}
func (h Handler) respondOne(w http.ResponseWriter, r *http.Request, t string, id int64, status int) {
	cols, e := h.columns(r, t)
	if e != nil {
		writeError(w, 500, e.Error())
		return
	}
	m, e := scanSingle(h.db.QueryRowContext(r.Context(), "SELECT * FROM "+t+" WHERE id=?", id), cols)
	if e != nil {
		writeError(w, 500, e.Error())
		return
	}
	writeJSON(w, status, m)
}
func (h Handler) delete(w http.ResponseWriter, r *http.Request, t string, id int64) {
	if id <= 0 {
		writeError(w, 400, "id required")
		return
	}
	res, e := h.db.ExecContext(r.Context(), "DELETE FROM "+t+" WHERE id=?", id)
	if e != nil {
		writeError(w, 400, e.Error())
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeError(w, 404, "not found")
		return
	}
	w.WriteHeader(204)
}
func (h Handler) setDefault(w http.ResponseWriter, r *http.Request, t string, id int64) {
	if id <= 0 {
		writeError(w, 400, "id required")
		return
	}
	tx, e := h.db.BeginTx(r.Context(), nil)
	if e != nil {
		writeError(w, 500, e.Error())
		return
	}
	defer tx.Rollback()
	resetQuery := ""
	resetArgs := []any{}
	switch t {
	case "ai_models":
		var connectionID int64
		e = tx.QueryRowContext(r.Context(), "SELECT connection_id FROM ai_models WHERE id=? AND is_enabled=1", id).Scan(&connectionID)
		resetQuery = "UPDATE ai_models SET is_default=0 WHERE connection_id=? AND is_default=1"
		resetArgs = append(resetArgs, connectionID)
	case "ai_connections", "review_profiles", "gitea_instances":
		var found int64
		e = tx.QueryRowContext(r.Context(), "SELECT id FROM "+t+" WHERE id=? AND is_enabled=1", id).Scan(&found)
		resetQuery = "UPDATE " + t + " SET is_default=0 WHERE is_default=1"
	default:
		writeError(w, 400, "resource does not support a default")
		return
	}
	if e == sql.ErrNoRows {
		writeError(w, 404, "not found or disabled")
		return
	}
	if e == nil {
		_, e = tx.ExecContext(r.Context(), resetQuery, resetArgs...)
	}
	if e == nil {
		var result sql.Result
		result, e = tx.ExecContext(r.Context(), "UPDATE "+t+" SET is_default=1,updated_at=CURRENT_TIMESTAMP WHERE id=?", id)
		if e == nil {
			var affected int64
			affected, e = result.RowsAffected()
			if e == nil && affected != 1 {
				e = fmt.Errorf("default target was not updated")
			}
		}
	}
	if e == nil {
		e = tx.Commit()
	}
	if e != nil {
		writeError(w, 400, e.Error())
		return
	}
	h.one(w, r, t, id)
}
func (h Handler) testConnection(w http.ResponseWriter, r *http.Request, id int64) {
	var provider, baseURL, keyEnv string
	e := h.db.QueryRowContext(r.Context(), `SELECT p.name,COALESCE(NULLIF(c.base_url,''),p.base_url),c.api_key_env_name FROM ai_connections c JOIN ai_providers p ON p.id=c.provider_id WHERE c.id=? AND c.is_enabled=1`, id).Scan(&provider, &baseURL, &keyEnv)
	if e != nil {
		writeError(w, 404, "connection not found or disabled")
		return
	}
	url := strings.TrimRight(baseURL, "/")
	if provider == "ollama" {
		url += "/api/tags"
	} else {
		url += "/models"
	}
	req, e := http.NewRequestWithContext(r.Context(), http.MethodGet, url, nil)
	if e != nil {
		writeError(w, 400, e.Error())
		return
	}
	if keyEnv != "" {
		key := os.Getenv(keyEnv)
		if key == "" {
			writeError(w, 400, fmt.Sprintf("environment variable %s is not set", keyEnv))
			return
		}
		req.Header.Set("Authorization", "Bearer "+key)
	}
	res, e := h.http.Do(req)
	if e != nil {
		writeError(w, 502, e.Error())
		return
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 1024))
		writeError(w, 502, fmt.Sprintf("provider returned %d: %s", res.StatusCode, strings.TrimSpace(string(b))))
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "provider": provider, "status": res.StatusCode})
}
