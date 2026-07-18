package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"gitea-agents/internal/http/responses"
	"gitea-agents/internal/security"

	"github.com/gin-gonic/gin"
)

type setupCatalogRequest struct {
	Provider   string `json:"provider"`
	Connection struct {
		ID          int64  `json:"id"`
		BaseURL     string `json:"base_url"`
		APIKey      string `json:"api_key"`
		HTTPReferer string `json:"http_referer"`
		AppTitle    string `json:"app_title"`
	} `json:"connection"`
}

type setupRequest struct {
	Provider   string         `json:"provider"`
	Connection map[string]any `json:"connection"`
	Model      map[string]any `json:"model"`
	Parameters map[string]any `json:"parameters"`
	Profile    map[string]any `json:"profile"`
	Policy     map[string]any `json:"policy"`
}

type modelRequest struct {
	ConnectionID int64          `json:"connection_id"`
	Model        map[string]any `json:"model"`
}

type sqlExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

// testConnection validates a saved AI connection against its provider's
// catalog endpoint and returns the detected provider name.
func (h *AdminHandler) testConnection(c *gin.Context) {
	var provider, baseURL, ciphertext, authType string
	err := h.db.QueryRowContext(
		c,
		`SELECT p.name,COALESCE(NULLIF(c.base_url,''),p.base_url),c.api_key_ciphertext,p.auth_type
		 FROM ai_connections c
		 JOIN ai_providers p ON p.id=c.provider_id
		 WHERE c.id=? AND c.is_enabled=1`,
		c.Param("id"),
	).Scan(&provider, &baseURL, &ciphertext, &authType)
	if err != nil {
		responses.LegacyError(c, http.StatusNotFound, "connection not found")
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
		responses.LegacyError(c, http.StatusBadRequest, "endpoint inválido")
		return
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}

	res, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil || res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
		if res != nil {
			_ = res.Body.Close()
		}
		responses.LegacyError(c, http.StatusBadGateway, "provider indisponível")
		return
	}
	_ = res.Body.Close()

	responses.Legacy(c, http.StatusOK, gin.H{"ok": true, "provider": provider})
}

// setup dispatches setup actions to catalog discovery, complete setup, or
// model creation while preserving the legacy administrative contract.
func (h *AdminHandler) setup(c *gin.Context) {
	switch c.Param("action") {
	case "catalog":
		h.catalog(c)
	case "complete":
		h.completeSetup(c)
	case "add-model":
		h.addModel(c)
	default:
		responses.LegacyError(c, http.StatusNotFound, "not found")
	}
}

// catalog validates an Ollama or OpenRouter connection and returns its current
// model catalog in the shape expected by the setup wizard.
func (h *AdminHandler) catalog(c *gin.Context) {
	var input setupCatalogRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		responses.LegacyError(c, http.StatusBadRequest, "invalid JSON")
		return
	}

	provider := input.Provider
	if provider == "ollama-cloud" {
		provider = "ollama"
	}

	if input.Connection.ID > 0 {
		var dbURL, ciphertext string
		err := h.db.QueryRowContext(
			c,
			"SELECT base_url,api_key_ciphertext FROM ai_connections WHERE id=? AND is_enabled=1",
			input.Connection.ID,
		).Scan(&dbURL, &ciphertext)
		if err != nil {
			responses.LegacyError(c, http.StatusBadRequest, "connection not found")
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
		switch {
		case provider == "openrouter":
			input.Connection.BaseURL = "https://openrouter.ai/api/v1"
		case input.Provider == "ollama-cloud":
			input.Connection.BaseURL = "https://ollama.com"
		default:
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
		responses.LegacyError(c, http.StatusBadRequest, "endpoint inválido")
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
		responses.LegacyError(c, http.StatusBadGateway, "provider indisponível")
		return
	}
	defer res.Body.Close()

	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
		responses.LegacyError(c, http.StatusBadGateway, fmt.Sprintf("provider respondeu HTTP %d", res.StatusCode))
		return
	}

	if provider == "openrouter" {
		h.openRouterCatalogResponse(c, res)
		return
	}
	h.ollamaCatalogResponse(c, res)
}

// openRouterCatalogResponse decodes and writes an OpenRouter model catalog.
func (h *AdminHandler) openRouterCatalogResponse(c *gin.Context, res *http.Response) {
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
		responses.LegacyError(c, http.StatusBadGateway, "catálogo inválido")
		return
	}

	models := make([]gin.H, 0, len(payload.Data))
	for _, item := range payload.Data {
		models = append(models, gin.H{
			"id":                    item.ID,
			"name":                  item.Name,
			"context_length":        item.ContextLength,
			"max_completion_tokens": item.TopProvider.MaxCompletionTokens,
			"supported_parameters":  item.SupportedParameters,
		})
	}

	responses.Legacy(c, http.StatusOK, gin.H{
		"provider":      "openrouter",
		"connection_ok": true,
		"models":        models,
	})
}

// ollamaCatalogResponse decodes and writes an Ollama model catalog.
func (h *AdminHandler) ollamaCatalogResponse(c *gin.Context, res *http.Response) {
	var payload struct {
		Models []struct {
			Name  string `json:"name"`
			Model string `json:"model"`
			Size  int64  `json:"size"`
		} `json:"models"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		responses.LegacyError(c, http.StatusBadGateway, "catálogo inválido")
		return
	}

	parameters := []string{
		"temperature",
		"top_p",
		"repeat_penalty",
		"num_ctx",
		"num_predict",
		"keep_alive",
	}
	models := make([]gin.H, 0, len(payload.Models))
	for _, item := range payload.Models {
		id := item.Model
		if id == "" {
			id = item.Name
		}
		models = append(models, gin.H{
			"id":                    id,
			"name":                  item.Name,
			"context_length":        0,
			"max_completion_tokens": 0,
			"supported_parameters":  parameters,
			"size":                  item.Size,
		})
	}

	responses.Legacy(c, http.StatusOK, gin.H{
		"provider":      "ollama",
		"connection_ok": true,
		"models":        models,
	})
}

// completeSetup stores a connection, model, parameters, profile, and policy in
// one transaction. It returns the identifiers created by the transaction.
func (h *AdminHandler) completeSetup(c *gin.Context) {
	var input setupRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		responses.LegacyError(c, http.StatusBadRequest, "invalid JSON")
		return
	}

	providerName := input.Provider
	if providerName == "ollama-cloud" {
		providerName = "ollama"
	}

	tx, err := h.db.BeginTx(c, nil)
	if err != nil {
		responses.LegacyError(c, http.StatusInternalServerError, "could not start setup transaction")
		return
	}
	defer tx.Rollback()

	var providerID int64
	err = tx.QueryRowContext(c, "SELECT id FROM ai_providers WHERE name=?", providerName).Scan(&providerID)
	if err != nil {
		responses.LegacyError(c, http.StatusBadRequest, "provider not found")
		return
	}

	apiKey := stringValue(input.Connection["api_key"])
	encryptedKey := ""
	if apiKey != "" {
		encryptedKey, err = security.Encrypt(apiKey)
		if err != nil {
			responses.LegacyError(c, http.StatusBadRequest, "não foi possível proteger a API key")
			return
		}
	}

	_, err = tx.ExecContext(c, "UPDATE ai_connections SET is_default=0 WHERE provider_id=? AND is_default=1", providerID)
	if err != nil {
		responses.LegacyError(c, http.StatusBadRequest, "could not update connection default")
		return
	}

	connectionResult, err := tx.ExecContext(
		c,
		`INSERT INTO ai_connections(
			provider_id,name,base_url,api_key_ciphertext,http_referer,app_title,is_default,is_enabled
		) VALUES(?,?,?,?,?,?,1,1)`,
		providerID,
		stringValue(input.Connection["name"]),
		stringValue(input.Connection["base_url"]),
		encryptedKey,
		stringValue(input.Connection["http_referer"]),
		stringValue(input.Connection["app_title"]),
	)
	if err != nil {
		responses.LegacyError(c, http.StatusBadRequest, "could not create connection")
		return
	}
	connectionID, _ := connectionResult.LastInsertId()

	modelID, err := h.insertModelExec(c, tx, connectionID, input.Model)
	if err != nil {
		responses.LegacyError(c, http.StatusBadRequest, err.Error())
		return
	}

	_, err = tx.ExecContext(
		c,
		`INSERT INTO model_parameters(
			model_id,temperature,top_p,timeout_seconds,keep_alive,unload_model_after_review
		) VALUES(?,?,?,?,?,?)`,
		modelID,
		floatValue(input.Parameters["temperature"]),
		floatValue(input.Parameters["top_p"]),
		intValue(input.Parameters["timeout_seconds"], h.cfg.ReviewRequestTimeoutSeconds),
		stringValue(input.Parameters["keep_alive"]),
		boolIntValue(input.Parameters["unload_model_after_review"]),
	)
	if err != nil {
		responses.LegacyError(c, http.StatusBadRequest, "could not create model parameters")
		return
	}

	profileResult, err := tx.ExecContext(
		c,
		`INSERT INTO review_profiles(name,description,model_id,is_default,is_enabled)
		 VALUES(?,?,?,1,1)`,
		stringValue(input.Profile["name"]),
		stringValue(input.Profile["description"]),
		modelID,
	)
	if err != nil {
		responses.LegacyError(c, http.StatusBadRequest, "could not create profile")
		return
	}
	profileID, _ := profileResult.LastInsertId()

	_, err = tx.ExecContext(
		c,
		`INSERT INTO review_policies(
			profile_id,max_block_chars,max_files_per_block,review_concurrency,
			review_final_retries,publish_manual_reviews,allow_autonomous_rejection
		) VALUES(?,?,?,?,?,?,?)`,
		profileID,
		intValue(input.Policy["max_block_chars"], h.cfg.ReviewMaxBlockChars),
		intValue(input.Policy["max_files_per_block"], h.cfg.ReviewMaxFilesPerBlock),
		intValue(input.Policy["review_concurrency"], 1),
		intValue(input.Policy["review_final_retries"], 5),
		boolIntValue(input.Policy["publish_manual_reviews"]),
		boolIntValue(input.Policy["allow_autonomous_rejection"]),
	)
	if err != nil {
		responses.LegacyError(c, http.StatusBadRequest, "could not create review policy")
		return
	}

	if err = tx.Commit(); err != nil {
		responses.LegacyError(c, http.StatusInternalServerError, "could not commit setup")
		return
	}

	responses.Legacy(c, http.StatusCreated, gin.H{
		"connection_id": connectionID,
		"model_id":      modelID,
		"profile_id":    profileID,
	})
}

// addModel creates one model under an existing AI connection and returns its ID.
func (h *AdminHandler) addModel(c *gin.Context) {
	var input modelRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		responses.LegacyError(c, http.StatusBadRequest, "invalid JSON")
		return
	}

	id, err := h.insertModel(c, input.ConnectionID, input.Model)
	if err != nil {
		responses.LegacyError(c, http.StatusBadRequest, err.Error())
		return
	}

	responses.Legacy(c, http.StatusCreated, gin.H{"id": id})
}

// insertModel stores one model using the handler database and returns its ID.
func (h *AdminHandler) insertModel(c *gin.Context, connectionID int64, model map[string]any) (int64, error) {
	return h.insertModelExec(c, h.db, connectionID, model)
}

// insertModelExec stores one model using either a database or transaction and
// returns its generated ID.
func (h *AdminHandler) insertModelExec(
	ctx context.Context,
	exec sqlExecutor,
	connectionID int64,
	model map[string]any,
) (int64, error) {
	result, err := exec.ExecContext(
		ctx,
		`INSERT INTO ai_models(
			connection_id,provider_model_name,display_name,context_window,
			max_output_tokens,supports_json,is_enabled
		) VALUES(?,?,?,?,?,?,1)`,
		connectionID,
		stringValue(model["id"]),
		stringValue(model["name"]),
		intValue(model["context_length"], 0),
		intValue(model["max_completion_tokens"], 0),
		1,
	)
	if err != nil {
		return 0, err
	}

	return result.LastInsertId()
}
