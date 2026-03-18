package ai

import "fmt"

func NewClient(provider, apiKey, model string) AIClient {
	switch provider {
	case "openai":
		return NewOpenAIClient(apiKey, "https://api.openai.com/v1", model)
	case "groq":
		return NewOpenAIClient(apiKey, "https://api.groq.com/openai/v1", model)
	case "gemini":
		client, _ := NewGeminiClient(apiKey, model)
		return client
	case "anthropic":
		return NewAnthropicClient(apiKey, model)
	default:
		fmt.Printf("Unknown provider: %s\n", provider)
		return nil
	}
}
