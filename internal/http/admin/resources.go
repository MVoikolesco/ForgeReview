package admin

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"gitea-agents/internal/http/responses"
	"gitea-agents/internal/security"

	"github.com/gin-gonic/gin"
)

// list returns every row for an allow-listed administrative resource.
func (h *AdminHandler) list(c *gin.Context) {
	table, ok := h.table(c.Param("path"))
	if !ok {
		table, ok = h.table(strings.TrimPrefix(c.Request.URL.Path, "/api/admin/"))
	}
	if !ok {
		responses.Fail(c, http.StatusNotFound, "NOT_FOUND", "resource not found")
		return
	}

	rows, err := h.db.QueryContext(c, `SELECT * FROM `+table+` ORDER BY id`)
	if err != nil {
		responses.Fail(c, http.StatusInternalServerError, "DATABASE_ERROR", "could not list resource")
		return
	}
	defer rows.Close()

	items, err := scanRows(rows)
	if err != nil {
		responses.Fail(c, http.StatusInternalServerError, "DATABASE_ERROR", "could not read resource")
		return
	}

	responses.Legacy(c, http.StatusOK, items)
}

// one returns one allow-listed administrative resource by its route ID.
func (h *AdminHandler) one(c *gin.Context) {
	table, ok := h.tableFromRequest(c)
	if !ok {
		return
	}

	cols, err := columns(c, h.db, table)
	if err != nil {
		responses.Fail(c, http.StatusInternalServerError, "DATABASE_ERROR", "could not inspect resource")
		return
	}

	item, err := scanOne(
		h.db.QueryRowContext(c, `SELECT * FROM `+table+` WHERE id=?`, c.Param("id")),
		cols,
	)
	if err == sql.ErrNoRows {
		responses.LegacyError(c, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		responses.LegacyError(c, http.StatusInternalServerError, "database error")
		return
	}

	responses.Legacy(c, http.StatusOK, item)
}

// create inserts a new administrative resource from the request JSON.
func (h *AdminHandler) create(c *gin.Context) {
	h.write(c, 0)
}

// update changes an administrative resource identified by the route ID.
func (h *AdminHandler) update(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	h.write(c, id)
}

// write validates writable columns, protects secrets, and inserts or updates
// one administrative resource. An ID of zero selects insertion.
func (h *AdminHandler) write(c *gin.Context, id int64) {
	table, ok := h.tableFromRequest(c)
	if !ok {
		return
	}

	var payload map[string]any
	if err := c.ShouldBindJSON(&payload); err != nil {
		responses.LegacyError(c, http.StatusBadRequest, "invalid JSON")
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
		responses.LegacyError(c, http.StatusInternalServerError, "database error")
		return
	}

	allowed := make(map[string]bool, len(cols))
	for _, col := range cols {
		allowed[col] = true
	}

	if table == "ai_connections" {
		delete(payload, "requires_auth")
		if apiKey != "" {
			encrypted, encryptErr := security.Encrypt(apiKey)
			if encryptErr != nil {
				responses.LegacyError(c, http.StatusBadRequest, encryptErr.Error())
				return
			}
			payload["api_key_ciphertext"] = encrypted
		}
		if apiKey == "" {
			delete(payload, "api_key_ciphertext")
		}
	}

	if table == "gitea_instances" && token != "" {
		encrypted, encryptErr := security.Encrypt(token)
		if encryptErr != nil {
			responses.LegacyError(c, http.StatusBadRequest, encryptErr.Error())
			return
		}
		payload["token_ciphertext"] = encrypted
	}

	keys := make([]string, 0, len(payload))
	for key := range payload {
		if allowed[key] {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)

	if len(keys) == 0 {
		responses.LegacyError(c, http.StatusBadRequest, "no writable fields")
		return
	}

	args := make([]any, 0, len(keys)+1)
	for _, key := range keys {
		args = append(args, payload[key])
	}

	var resultID int64
	if id == 0 {
		placeholders := strings.TrimRight(strings.Repeat("?,", len(keys)), ",")
		query := `INSERT INTO ` + table + ` (` + strings.Join(keys, ",") + `) VALUES (` + placeholders + `)`
		result, execErr := h.db.ExecContext(c, query, args...)
		if execErr != nil {
			responses.LegacyError(c, http.StatusBadRequest, "could not save resource")
			return
		}
		resultID, _ = result.LastInsertId()
	} else {
		sets := make([]string, len(keys))
		for i, key := range keys {
			sets[i] = key + "=?"
		}
		args = append(args, id)

		query := "UPDATE " + table + " SET " + strings.Join(sets, ",") + ",updated_at=CURRENT_TIMESTAMP WHERE id=?"
		result, execErr := h.db.ExecContext(c, query, args...)
		if execErr != nil {
			responses.LegacyError(c, http.StatusBadRequest, "could not save resource")
			return
		}

		affected, _ := result.RowsAffected()
		if affected == 0 {
			responses.LegacyError(c, http.StatusNotFound, "not found")
			return
		}
		resultID = id
	}

	h.oneByID(c, table, resultID, func(status int, value any) {
		responses.Legacy(c, status, value)
	})
}

// delete removes an administrative resource identified by the route ID.
func (h *AdminHandler) delete(c *gin.Context) {
	table, ok := h.tableFromRequest(c)
	if !ok {
		return
	}

	if _, err := h.db.ExecContext(c, "DELETE FROM "+table+" WHERE id=?", c.Param("id")); err != nil {
		responses.LegacyError(c, http.StatusBadRequest, "could not delete resource")
		return
	}

	c.Status(http.StatusNoContent)
}

// oneByID loads one row after a write and passes the status and response value
// to the supplied callback.
func (h *AdminHandler) oneByID(c *gin.Context, table string, id int64, done func(int, any)) {
	cols, err := columns(c, h.db, table)
	if err != nil {
		done(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	item, err := scanOne(h.db.QueryRowContext(c, `SELECT * FROM `+table+` WHERE id=?`, id), cols)
	if err != nil {
		done(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	done(http.StatusOK, item)
}

// setDefault marks the requested row as the default within the correct
// provider or connection scope.
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
		responses.LegacyError(c, http.StatusBadRequest, "could not update default")
		return
	}
	if _, err := h.db.ExecContext(c, "UPDATE "+table+" SET is_default=1 WHERE id=?", id); err != nil {
		responses.LegacyError(c, http.StatusBadRequest, "could not update default")
		return
	}

	h.one(c)
}

// columns returns the column names exposed by a table query.
func columns(ctx context.Context, db *sql.DB, table string) ([]string, error) {
	rows, err := db.QueryContext(ctx, "SELECT * FROM "+table+" LIMIT 0")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return rows.Columns()
}

// scanRows converts database rows into maps suitable for legacy JSON output.
func scanRows(rows *sql.Rows) ([]map[string]any, error) {
	cols, _ := rows.Columns()
	items := []map[string]any{}
	for rows.Next() {
		values := make([]any, len(cols))
		pointers := make([]any, len(cols))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			return nil, err
		}
		items = append(items, cleanRow(cols, values))
	}

	return items, rows.Err()
}

// scanOne converts one database row into a map suitable for legacy JSON output.
func scanOne(row *sql.Row, cols []string) (map[string]any, error) {
	values := make([]any, len(cols))
	pointers := make([]any, len(cols))
	for i := range values {
		pointers[i] = &values[i]
	}
	if err := row.Scan(pointers...); err != nil {
		return nil, err
	}

	return cleanRow(cols, values), nil
}

// cleanRow converts byte slices to strings and removes encrypted secrets from
// administrative responses.
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

// stringValue converts a dynamic administrative field to a string.
func stringValue(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

// intValue converts a dynamic field to an integer or returns the fallback.
func intValue(value any, fallback int) int {
	number, err := strconv.Atoi(stringValue(value))
	if err != nil {
		return fallback
	}
	return number
}

// floatValue converts a dynamic field to a float, returning zero on failure.
func floatValue(value any) float64 {
	number, _ := strconv.ParseFloat(stringValue(value), 64)
	return number
}

// boolIntValue converts a dynamic boolean field to SQLite's integer form.
func boolIntValue(value any) int {
	truthy := strings.EqualFold(stringValue(value), "true") || stringValue(value) == "1"
	if truthy {
		return 1
	}
	return 0
}
