package ai

import "context"

type ChatResult struct {
	Content          string
	PromptTokens     int
	CompletionTokens int
}

type Client interface {
	Chat(context.Context, string) (string, error)
	Model() string
	ChatWithMetadata(context.Context, string) (ChatResult, error)
	ChatWithMaxTokens(context.Context, string, int) (ChatResult, error)
}
