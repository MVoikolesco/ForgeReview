package providers

import (
	"context"
	"strings"
	"time"

	"gitea-agents/internal/contracts"
)

// Input contains the pull request context, prompt, and diff sent to an AI
// provider for one review block.
type Input struct {
	Owner       string
	Repository  string
	PullRequest int
	Diff        string
	Prompt      string
}

// LLMProvider reviews one diff block and returns the provider-independent
// structured review contract.
type LLMProvider interface {
	// Name returns the provider identifier stored in review metadata.
	Name() string
	// Review sends input to the provider and returns a parsed review result.
	Review(ctx context.Context, input Input) (contracts.Result, error)
}

// Config contains the connection, model, credential, and timeout used by one
// provider adapter.
type Config struct {
	Name    string
	BaseURL string
	APIKey  string
	Model   string
	Timeout time.Duration
}

// New selects and returns an LLMProvider adapter from cfg.Name. Unknown names
// use the OpenAI-compatible protocol so configured services such as Groq work.
func New(cfg Config) LLMProvider {
	switch strings.ToLower(cfg.Name) {
	case "ollama":
		return &ollama{cfg: cfg}
	case "google_gemini":
		return &gemini{cfg: cfg}
	default:
		return &openAICompatible{cfg: cfg}
	}
}
