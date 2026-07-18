package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"gitea-agents/internal/ai"
	"gitea-agents/internal/secrets"
)

type setupConnection struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	BaseURL     string `json:"base_url"`
	APIKey      string `json:"api_key"`
	HTTPReferer string `json:"http_referer"`
	AppTitle    string `json:"app_title"`
}
type catalogModel struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	ContextLength       int      `json:"context_length"`
	MaxCompletionTokens int      `json:"max_completion_tokens"`
	SupportedParameters []string `json:"supported_parameters"`
	PromptPrice         string   `json:"prompt_price"`
	CompletionPrice     string   `json:"completion_price"`
	Size                int64    `json:"size,omitempty"`
}
type catalogRequest struct {
	Provider   string          `json:"provider"`
	Connection setupConnection `json:"connection"`
}
type setupParameters struct {
	Temperature            float64 `json:"temperature"`
	TopP                   float64 `json:"top_p"`
	MaxOutputTokens        int     `json:"max_output_tokens"`
	RepeatPenalty          float64 `json:"repeat_penalty"`
	NumCtx                 int     `json:"num_ctx"`
	NumThreads             int     `json:"num_threads"`
	NumPredict             int     `json:"num_predict"`
	TimeoutSeconds         int     `json:"timeout_seconds"`
	KeepAlive              string  `json:"keep_alive"`
	UnloadModelAfterReview bool    `json:"unload_model_after_review"`
}
type setupProfile struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}
type setupPolicy struct {
	MaxBlockChars            int  `json:"max_block_chars"`
	MaxFilesPerBlock         int  `json:"max_files_per_block"`
	ReviewConcurrency        int  `json:"review_concurrency"`
	ReviewFinalRetries       int  `json:"review_final_retries"`
	ReviewWIPPullRequests    bool `json:"review_wip_pull_requests"`
	ReviewOwnPullRequests    bool `json:"review_own_pull_requests"`
	PublishManualReviews     bool `json:"publish_manual_reviews"`
	AllowAutonomousRejection bool `json:"allow_autonomous_rejection"`
	UnloadModelAfterReview   bool `json:"unload_model_after_review"`
}
type completeSetupRequest struct {
	Provider   string          `json:"provider"`
	Connection setupConnection `json:"connection"`
	Model      catalogModel    `json:"model"`
	Parameters setupParameters `json:"parameters"`
	Profile    setupProfile    `json:"profile"`
	Policy     setupPolicy     `json:"policy"`
}
type addModelRequest struct {
	ConnectionID int64           `json:"connection_id"`
	Model        catalogModel    `json:"model"`
	Parameters   setupParameters `json:"parameters"`
	MakeDefault  bool            `json:"make_default"`
}

func (h Handler) setupRoute(w http.ResponseWriter, r *http.Request, action string) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	switch action {
	case "catalog":
		h.providerCatalog(w, r)
	case "complete":
		h.completeSetup(w, r)
	case "add-model":
		h.addModel(w, r, 0)
	default:
		http.NotFound(w, r)
	}
}

func normalizeConnection(provider string, c setupConnection) setupConnection {
	cloudOllama := provider == "ollama-cloud"
	c.BaseURL = strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if c.Name == "" {
		if provider == "openrouter" {
			c.Name = "OpenRouter principal"
		} else if cloudOllama {
			c.Name = "Ollama Cloud"
		} else {
			c.Name = "Ollama local"
		}
	}
	if c.BaseURL == "" {
		if provider == "openrouter" {
			c.BaseURL = "https://openrouter.ai/api/v1"
		} else if cloudOllama {
			c.BaseURL = "https://ollama.com"
		} else {
			c.BaseURL = "http://localhost:11434"
		}
	}
	return c
}

func setupProviderName(provider string) string {
	if provider == "ollama-cloud" {
		return "ollama"
	}
	return provider
}

func setupRequiresAuthentication(provider, baseURL string) bool {
	return ai.ConnectionRequiresAuthentication(setupProviderName(provider), map[string]string{"openrouter": "bearer"}[setupProviderName(provider)], baseURL)
}

func (h Handler) providerCatalog(w http.ResponseWriter, r *http.Request) {
	var input catalogRequest
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		writeError(w, 400, "JSON inválido")
		return
	}
	input.Connection = normalizeConnection(input.Provider, input.Connection)
	ollamaCloud := setupRequiresAuthentication(input.Provider, input.Connection.BaseURL)
	if input.Connection.ID > 0 {
		var providerName, authType, ciphertext, baseURL string
		if err := h.db.QueryRowContext(r.Context(), "SELECT p.name,p.auth_type,COALESCE(NULLIF(c.base_url,''),p.base_url),c.api_key_ciphertext FROM ai_connections c JOIN ai_providers p ON p.id=c.provider_id WHERE c.id=? AND c.is_enabled=1", input.Connection.ID).Scan(&providerName, &authType, &baseURL, &ciphertext); err != nil {
			writeError(w, 400, "conexão armazenada ausente ou inativa")
			return
		}
		input.Connection.BaseURL = baseURL
		ollamaCloud = ai.ConnectionRequiresAuthentication(providerName, authType, baseURL)
		if ollamaCloud {
			ollamaCloud = true
			if input.Connection.APIKey == "" {
				key, err := secrets.Decrypt(ciphertext, secrets.ConnectionKeyAAD(input.Connection.ID))
				if err != nil || key == "" {
					writeError(w, 400, "API key armazenada ausente ou inválida")
					return
				}
				input.Connection.APIKey = key
			}
		}
	}
	if ollamaCloud && strings.TrimSpace(input.Connection.APIKey) == "" {
		writeError(w, 400, "API key é obrigatória para esta conexão")
		return
	}
	switch setupProviderName(input.Provider) {
	case "openrouter":
		h.openRouterCatalog(w, r, input.Connection)
	case "ollama":
		h.ollamaCatalog(w, r, input.Connection, ollamaCloud)
	default:
		writeError(w, 400, "O wizard oferece suporte a Ollama e OpenRouter")
	}
}

func providerRequest(ctx context.Context, method, url, key string, c setupConnection) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, err
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	if c.HTTPReferer != "" {
		req.Header.Set("HTTP-Referer", c.HTTPReferer)
	}
	if c.AppTitle != "" {
		req.Header.Set("X-OpenRouter-Title", c.AppTitle)
	}
	return req, nil
}
func (h Handler) openRouterCatalog(w http.ResponseWriter, r *http.Request, c setupConnection) {
	key := c.APIKey
	if key == "" {
		writeError(w, 400, "API key é obrigatória para OpenRouter")
		return
	}
	keyReq, _ := providerRequest(r.Context(), http.MethodGet, c.BaseURL+"/key", key, c)
	keyRes, err := h.http.Do(keyReq)
	if err != nil {
		writeError(w, 502, "não foi possível validar a chave com OpenRouter")
		return
	}
	defer keyRes.Body.Close()
	if keyRes.StatusCode < 200 || keyRes.StatusCode >= 300 {
		writeError(w, 502, fmt.Sprintf("OpenRouter recusou a chave (HTTP %d)", keyRes.StatusCode))
		return
	}
	var keyInfo map[string]any
	if json.NewDecoder(keyRes.Body).Decode(&keyInfo) != nil {
		keyInfo = map[string]any{}
	}
	modelsReq, _ := providerRequest(r.Context(), http.MethodGet, c.BaseURL+"/models?output_modalities=text", key, c)
	modelsRes, err := h.http.Do(modelsReq)
	if err != nil {
		writeError(w, 502, "não foi possível consultar modelos OpenRouter")
		return
	}
	defer modelsRes.Body.Close()
	if modelsRes.StatusCode < 200 || modelsRes.StatusCode >= 300 {
		writeError(w, 502, fmt.Sprintf("Falha ao consultar modelos OpenRouter (HTTP %d)", modelsRes.StatusCode))
		return
	}
	var payload struct {
		Data []struct {
			ID, Name            string
			ContextLength       int      `json:"context_length"`
			SupportedParameters []string `json:"supported_parameters"`
			Pricing             struct{ Prompt, Completion string }
			TopProvider         struct {
				MaxCompletionTokens int `json:"max_completion_tokens"`
			} `json:"top_provider"`
		} `json:"data"`
	}
	if err := json.NewDecoder(modelsRes.Body).Decode(&payload); err != nil {
		writeError(w, 502, "OpenRouter retornou um catálogo de modelos inválido")
		return
	}
	models := make([]catalogModel, 0, len(payload.Data))
	for _, m := range payload.Data {
		models = append(models, catalogModel{ID: m.ID, Name: m.Name, ContextLength: m.ContextLength, MaxCompletionTokens: m.TopProvider.MaxCompletionTokens, SupportedParameters: m.SupportedParameters, PromptPrice: m.Pricing.Prompt, CompletionPrice: m.Pricing.Completion})
	}
	writeJSON(w, 200, map[string]any{"provider": "openrouter", "connection_ok": true, "models": models, "account": keyInfo["data"]})
}

func (h Handler) ollamaCatalog(w http.ResponseWriter, r *http.Request, c setupConnection, cloud bool) {
	key := ""
	if cloud {
		key = c.APIKey
	}
	req, _ := providerRequest(r.Context(), http.MethodGet, c.BaseURL+"/api/tags", key, c)
	res, err := h.http.Do(req)
	if err != nil {
		writeError(w, 502, "não foi possível acessar o Ollama")
		return
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		writeError(w, 502, fmt.Sprintf("Ollama respondeu HTTP %d", res.StatusCode))
		return
	}
	var payload struct {
		Models []struct {
			Name, Model string
			Size        int64
		}
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		writeError(w, 502, "Ollama retornou um catálogo de modelos inválido")
		return
	}
	models := make([]catalogModel, 0, len(payload.Models))
	for _, m := range payload.Models {
		id := m.Model
		if id == "" {
			id = m.Name
		}
		models = append(models, catalogModel{ID: id, Name: m.Name, Size: m.Size, SupportedParameters: []string{"temperature", "top_p", "repeat_penalty", "num_ctx", "num_predict", "keep_alive"}})
	}
	writeJSON(w, 200, map[string]any{"provider": "ollama", "connection_ok": true, "models": models})
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
func requirePositive(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func (h Handler) completeSetup(w http.ResponseWriter, r *http.Request) {
	var input completeSetupRequest
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		writeError(w, 400, "JSON inválido")
		return
	}
	if input.Provider != "ollama" && input.Provider != "ollama-cloud" && input.Provider != "openrouter" {
		writeError(w, 400, "Provider não suportado pelo wizard")
		return
	}
	input.Connection = normalizeConnection(input.Provider, input.Connection)
	providerName := setupProviderName(input.Provider)
	if input.Model.ID == "" || input.Profile.Name == "" {
		writeError(w, 400, "Modelo e profile são obrigatórios")
		return
	}
	requiresAuth := setupRequiresAuthentication(input.Provider, input.Connection.BaseURL)
	if requiresAuth && strings.TrimSpace(input.Connection.APIKey) == "" {
		writeError(w, 400, "API key é obrigatória para esta conexão")
		return
	}
	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	defer tx.Rollback()
	var providerID int64
	if err = tx.QueryRowContext(r.Context(), "SELECT id FROM ai_providers WHERE name=? AND is_enabled=1", providerName).Scan(&providerID); err != nil {
		writeError(w, 400, "Provider não está disponível")
		return
	}
	_, _ = tx.ExecContext(r.Context(), "UPDATE ai_connections SET is_default=0 WHERE provider_id=? AND is_default=1", providerID)
	connectionResult, err := tx.ExecContext(r.Context(), `INSERT INTO ai_connections(provider_id,name,base_url,requires_auth,http_referer,app_title,is_default,is_enabled) VALUES(?,?,?,?,?,?,1,1)`, providerID, input.Connection.Name, input.Connection.BaseURL, boolInt(requiresAuth), input.Connection.HTTPReferer, input.Connection.AppTitle)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	connectionID, _ := connectionResult.LastInsertId()
	if requiresAuth && input.Connection.APIKey != "" {
		ciphertext, cryptErr := secrets.Encrypt(input.Connection.APIKey, secrets.ConnectionKeyAAD(connectionID))
		if cryptErr != nil {
			writeError(w, 400, "não foi possível proteger a API key")
			return
		}
		if _, err = tx.ExecContext(r.Context(), "UPDATE ai_connections SET api_key_ciphertext=? WHERE id=?", ciphertext, connectionID); err != nil {
			writeError(w, 500, err.Error())
			return
		}
	}
	maxTokens := 0
	if providerName == "openrouter" {
		// max_completion_tokens from the catalog is a model capability, not a
		// sensible amount to request on every completion. Keep the operational
		// review limit independent from that capability.
		maxTokens = requirePositive(input.Parameters.MaxOutputTokens, 4096)
		if input.Model.MaxCompletionTokens > 0 && maxTokens > input.Model.MaxCompletionTokens {
			maxTokens = input.Model.MaxCompletionTokens
		}
		if maxTokens > 32768 {
			maxTokens = 32768
		}
		if input.Model.ContextLength > 0 && maxTokens >= input.Model.ContextLength {
			maxTokens = requirePositive(input.Model.ContextLength/4, 1)
		}
	}
	supportsJSON := contains(input.Model.SupportedParameters, "response_format") || contains(input.Model.SupportedParameters, "structured_outputs")
	supportsTools := contains(input.Model.SupportedParameters, "tools")
	modelResult, err := tx.ExecContext(r.Context(), `INSERT INTO ai_models(connection_id,provider_model_name,display_name,context_window,max_output_tokens,supports_json,supports_tools,supports_streaming,is_default,is_enabled) VALUES(?,?,?,?,?,?,?,?,1,1)`, connectionID, input.Model.ID, input.Model.Name, input.Model.ContextLength, maxTokens, boolInt(supportsJSON), boolInt(supportsTools), 1)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	modelID, _ := modelResult.LastInsertId()
	p := input.Parameters
	p.TimeoutSeconds = requirePositive(p.TimeoutSeconds, 900)
	_, err = tx.ExecContext(r.Context(), `INSERT INTO model_parameters(model_id,temperature,top_p,repeat_penalty,num_ctx,num_threads,num_predict,keep_alive,timeout_seconds,unload_model_after_review) VALUES(?,?,?,?,?,?,?,?,?,?)`, modelID, p.Temperature, p.TopP, p.RepeatPenalty, p.NumCtx, p.NumThreads, p.NumPredict, p.KeepAlive, p.TimeoutSeconds, boolInt(p.UnloadModelAfterReview))
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	var profileID int64
	err = tx.QueryRowContext(r.Context(), "SELECT id FROM review_profiles WHERE is_default=1 LIMIT 1").Scan(&profileID)
	if err == nil {
		_, err = tx.ExecContext(r.Context(), "UPDATE review_profiles SET model_id=?,updated_at=CURRENT_TIMESTAMP WHERE id=?", modelID, profileID)
	} else if err == sql.ErrNoRows {
		profileResult, insertErr := tx.ExecContext(r.Context(), `INSERT INTO review_profiles(name,description,model_id,is_default,is_enabled) VALUES(?,?,?,1,1)`, input.Profile.Name, input.Profile.Description, modelID)
		if insertErr != nil {
			writeError(w, 400, insertErr.Error())
			return
		}
		profileID, _ = profileResult.LastInsertId()
		policy := input.Policy
		policy.MaxBlockChars = requirePositive(policy.MaxBlockChars, 12000)
		policy.MaxFilesPerBlock = requirePositive(policy.MaxFilesPerBlock, 4)
		policy.ReviewConcurrency = requirePositive(policy.ReviewConcurrency, 1)
		policy.ReviewFinalRetries = requirePositive(policy.ReviewFinalRetries, 5)
		_, err = tx.ExecContext(r.Context(), `INSERT INTO review_policies(profile_id,max_block_chars,max_files_per_block,review_concurrency,review_wip_pull_requests,review_own_pull_requests,unload_model_after_review,review_final_retries,publish_manual_reviews,allow_autonomous_rejection) VALUES(?,?,?,?,?,?,?,?,?,?)`, profileID, policy.MaxBlockChars, policy.MaxFilesPerBlock, policy.ReviewConcurrency, boolInt(policy.ReviewWIPPullRequests), boolInt(policy.ReviewOwnPullRequests), boolInt(policy.UnloadModelAfterReview), policy.ReviewFinalRetries, boolInt(policy.PublishManualReviews), boolInt(policy.AllowAutonomousRejection))
	}
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if err = tx.Commit(); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "connection_id": connectionID, "model_id": modelID, "profile_id": profileID})
}

func (h Handler) addModel(w http.ResponseWriter, r *http.Request, pathID int64) {
	var input addModelRequest
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		writeError(w, 400, "JSON inválido")
		return
	}
	if input.ConnectionID == 0 {
		input.ConnectionID = pathID
	}
	if input.ConnectionID <= 0 || input.Model.ID == "" {
		writeError(w, 400, "Conexão e modelo são obrigatórios")
		return
	}
	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	defer tx.Rollback()
	var provider string
	if err = tx.QueryRowContext(r.Context(), `SELECT p.name FROM ai_connections c JOIN ai_providers p ON p.id=c.provider_id WHERE c.id=? AND c.is_enabled=1`, input.ConnectionID).Scan(&provider); err != nil {
		writeError(w, 404, "conexão não encontrada ou inativa")
		return
	}
	_ = provider
	var modelID int64
	err = tx.QueryRowContext(r.Context(), "SELECT id FROM ai_models WHERE connection_id=? AND provider_model_name=?", input.ConnectionID, input.Model.ID).Scan(&modelID)
	if err == nil {
		writeError(w, 409, "Este modelo já está cadastrado nesta conexão")
		return
	}
	if err != sql.ErrNoRows {
		writeError(w, 500, err.Error())
		return
	}
	maxTokens := input.Parameters.MaxOutputTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	if input.Model.MaxCompletionTokens > 0 && maxTokens > input.Model.MaxCompletionTokens {
		maxTokens = input.Model.MaxCompletionTokens
	}
	if input.Model.ContextLength > 0 && maxTokens >= input.Model.ContextLength {
		maxTokens = requirePositive(input.Model.ContextLength/4, 1)
	}
	result, err := tx.ExecContext(r.Context(), `INSERT INTO ai_models(connection_id,provider_model_name,display_name,context_window,max_output_tokens,supports_json,supports_tools,supports_streaming,is_default,is_enabled) VALUES(?,?,?,?,?,?,?,?,0,1)`, input.ConnectionID, input.Model.ID, input.Model.Name, input.Model.ContextLength, maxTokens, boolInt(contains(input.Model.SupportedParameters, "response_format") || contains(input.Model.SupportedParameters, "structured_outputs")), boolInt(contains(input.Model.SupportedParameters, "tools")), 1)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	modelID, _ = result.LastInsertId()
	p := input.Parameters
	p.TimeoutSeconds = requirePositive(p.TimeoutSeconds, 900)
	if _, err = tx.ExecContext(r.Context(), `INSERT INTO model_parameters(model_id,temperature,top_p,repeat_penalty,num_ctx,num_threads,num_predict,keep_alive,timeout_seconds,unload_model_after_review) VALUES(?,?,?,?,?,?,?,?,?,?)`, modelID, p.Temperature, p.TopP, p.RepeatPenalty, p.NumCtx, p.NumThreads, p.NumPredict, p.KeepAlive, p.TimeoutSeconds, boolInt(p.UnloadModelAfterReview)); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if input.MakeDefault {
		if _, err = tx.ExecContext(r.Context(), "UPDATE ai_models SET is_default=0 WHERE connection_id=?", input.ConnectionID); err == nil {
			_, err = tx.ExecContext(r.Context(), "UPDATE ai_models SET is_default=1,updated_at=CURRENT_TIMESTAMP WHERE id=?", modelID)
		}
	}
	if err != nil || tx.Commit() != nil {
		if err == nil {
			err = fmt.Errorf("could not save model")
		}
		writeError(w, 400, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "model_id": modelID})
}
