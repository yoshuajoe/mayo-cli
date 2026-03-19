package ai

import (
	"context"
	"fmt"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

type GeminiClient struct {
	client    *genai.Client
	model     *genai.GenerativeModel
	modelName string
}

func NewGeminiClient(apiKey, modelName string) (*GeminiClient, error) {
	if modelName == "" {
		modelName = "gemini-1.5-flash"
	}
	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, err
	}

	model := client.GenerativeModel(modelName)
	return &GeminiClient{
		client:    client,
		model:     model,
		modelName: modelName,
	}, nil
}

func (g *GeminiClient) GenerateResponse(ctx context.Context, systemPrompt string, userPrompt string) (string, *TokenUsage, error) {
	// Gemini-pro currently uses a simpler chat pattern, but we can simulate system prompt by prepending it
	// or using the newer system instruction if supported by the SDK version.
	g.model.SystemInstruction = &genai.Content{
		Parts: []genai.Part{genai.Text(systemPrompt)},
	}

	resp, err := g.model.GenerateContent(ctx, genai.Text(userPrompt))
	if err != nil {
		return "", nil, err
	}

	usage := &TokenUsage{}
	if resp.UsageMetadata != nil {
		usage.PromptTokens = int(resp.UsageMetadata.PromptTokenCount)
		usage.CompletionTokens = int(resp.UsageMetadata.CandidatesTokenCount)
		usage.TotalTokens = int(resp.UsageMetadata.TotalTokenCount)
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return "", usage, fmt.Errorf("no response from Gemini")
	}

	part := resp.Candidates[0].Content.Parts[0]
	if text, ok := part.(genai.Text); ok {
		return string(text), usage, nil
	}

	return "", usage, fmt.Errorf("unexpected response format")
}

func (g *GeminiClient) GetEmbedding(ctx context.Context, text string) ([]float32, error) {
	modelName := "text-embedding-004"
	// Use model from config if it looks like an embedding model
	if g.modelName != "" && (g.modelName == "text-embedding-004" || g.modelName == "embedding-001") {
		modelName = g.modelName
	}

	em := g.client.EmbeddingModel(modelName)
	resp, err := em.EmbedContent(ctx, genai.Text(text))
	if err != nil {
		return nil, err
	}

	if resp.Embedding == nil {
		return nil, fmt.Errorf("no embedding returned from Gemini")
	}

	return resp.Embedding.Values, nil
}

func (g *GeminiClient) Close() {
	g.client.Close()
}

func (g *GeminiClient) SetModel(modelName string) {
	g.model = g.client.GenerativeModel(modelName)
	g.modelName = modelName
}

func (g *GeminiClient) GetModel() string {
	return g.modelName
}

func (g *GeminiClient) GetProvider() string {
	return "gemini"
}
