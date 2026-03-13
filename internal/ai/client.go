package ai

import "context"

type TokenUsage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

type AIClient interface {
	GenerateResponse(ctx context.Context, systemPrompt, userPrompt string) (string, *TokenUsage, error)
	SetModel(modelName string)
	GetModel() string
	GetProvider() string
}
