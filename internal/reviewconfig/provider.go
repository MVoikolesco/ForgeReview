package reviewconfig

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"gitea-agents/internal/store"
)

type ReviewConfig struct {
	Provider   ProviderConfig
	Connection ConnectionConfig
	Model      ModelConfig
	Parameters ModelParameters
	Policy     ReviewPolicyConfig
}
type ProviderConfig struct{ Name, BaseURL, AuthType string }
type ConnectionConfig struct{ Name, BaseURL, APIKeyEnvName, HTTPReferer, AppTitle string }
type ModelConfig struct {
	Name                                           string
	ContextWindow, MaxOutputTokens                 int
	SupportsJSON, SupportsTools, SupportsStreaming bool
}
type ModelParameters struct {
	Temperature, TopP, RepeatPenalty               float64
	NumCtx, NumThreads, NumPredict, TimeoutSeconds int
	KeepAlive                                      string
	UnloadModelAfterReview                         bool
}
type ReviewPolicyConfig struct {
	MaxBlockChars, MaxFilesPerBlock, ReviewConcurrency, ReviewFinalRetries                                                                 int
	ReviewWIPPullRequests, ReviewOwnPullRequests, LogSensitiveData, UnloadModelAfterReview, PublishManualReviews, AllowAutonomousRejection bool
}
type Provider interface {
	GetConfig(context.Context, string) (*ReviewConfig, error)
}

var ErrConfigNotFound = errors.New("no enabled review profile is configured")

type SQLiteProvider struct{ Store *store.Store }

func (p SQLiteProvider) GetConfig(ctx context.Context, repo string) (*ReviewConfig, error) {
	q := `SELECT ap.name, COALESCE(NULLIF(ac.base_url,''),ap.base_url), ap.auth_type, ac.name, COALESCE(NULLIF(ac.base_url,''),ap.base_url), ac.api_key_env_name,ac.http_referer,ac.app_title,am.provider_model_name,am.context_window,am.max_output_tokens,am.supports_json,am.supports_tools,am.supports_streaming,COALESCE(mp.temperature,0),COALESCE(mp.top_p,0),COALESCE(mp.repeat_penalty,0),COALESCE(mp.num_ctx,0),COALESCE(mp.num_threads,0),COALESCE(mp.num_predict,0),mp.keep_alive,mp.timeout_seconds,mp.unload_model_after_review,rp.id,pol.max_block_chars,pol.max_files_per_block,pol.review_concurrency,pol.review_wip_pull_requests,pol.review_own_pull_requests,pol.log_sensitive_data,pol.unload_model_after_review,pol.review_final_retries,pol.publish_manual_reviews,pol.allow_autonomous_rejection FROM review_profiles rp JOIN ai_models am ON am.id=rp.model_id AND am.is_enabled=1 JOIN ai_connections ac ON ac.id=am.connection_id AND ac.is_enabled=1 JOIN ai_providers ap ON ap.id=ac.provider_id AND ap.is_enabled=1 JOIN model_parameters mp ON mp.model_id=am.id JOIN review_policies pol ON pol.profile_id=rp.id LEFT JOIN repositories r ON r.review_profile_id=rp.id AND r.full_name=? AND r.is_enabled=1 WHERE rp.is_enabled=1 AND (r.id IS NOT NULL OR rp.is_default=1) ORDER BY r.id DESC, rp.is_default DESC LIMIT 1`
	var c ReviewConfig
	var profile int64
	var a, b, d, e, f, g, h bool
	err := p.Store.DB.QueryRowContext(ctx, q, repo).Scan(&c.Provider.Name, &c.Provider.BaseURL, &c.Provider.AuthType, &c.Connection.Name, &c.Connection.BaseURL, &c.Connection.APIKeyEnvName, &c.Connection.HTTPReferer, &c.Connection.AppTitle, &c.Model.Name, &c.Model.ContextWindow, &c.Model.MaxOutputTokens, &a, &b, &d, &c.Parameters.Temperature, &c.Parameters.TopP, &c.Parameters.RepeatPenalty, &c.Parameters.NumCtx, &c.Parameters.NumThreads, &c.Parameters.NumPredict, &c.Parameters.KeepAlive, &c.Parameters.TimeoutSeconds, &e, &profile, &c.Policy.MaxBlockChars, &c.Policy.MaxFilesPerBlock, &c.Policy.ReviewConcurrency, &f, &g, &h, &c.Policy.UnloadModelAfterReview, &c.Policy.ReviewFinalRetries, &c.Policy.PublishManualReviews, &c.Policy.AllowAutonomousRejection)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrConfigNotFound
		}
		return nil, fmt.Errorf("load review config: %w", err)
	}
	c.Model.SupportsJSON = a
	c.Model.SupportsTools = b
	c.Model.SupportsStreaming = d
	c.Parameters.UnloadModelAfterReview = e
	c.Policy.ReviewWIPPullRequests = f
	c.Policy.ReviewOwnPullRequests = g
	c.Policy.LogSensitiveData = h
	if c.Connection.BaseURL == "" || c.Model.Name == "" {
		return nil, fmt.Errorf("invalid review configuration: connection URL and model are required")
	}
	if c.Policy.MaxBlockChars <= 0 || c.Policy.MaxFilesPerBlock <= 0 || c.Policy.ReviewConcurrency <= 0 || c.Policy.ReviewFinalRetries <= 0 {
		return nil, fmt.Errorf("invalid review policy: limits and concurrency must be positive")
	}
	return &c, nil
}
