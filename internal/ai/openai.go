package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type OpenAIClient struct {
	APIKey  string
	BaseURL string
	Model   string
}

func NewOpenAIClient(apiKey, baseURL, defaultModel string) *OpenAIClient {
	return &OpenAIClient{
		APIKey:  apiKey,
		BaseURL: baseURL,
		Model:   defaultModel,
	}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (c *OpenAIClient) GenerateResponse(ctx context.Context, systemPrompt, userPrompt string) (string, *TokenUsage, error) {
	reqBody := chatRequest{
		Model: c.Model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, err
	}

	var chatResp chatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return "", nil, err
	}

	if chatResp.Error.Message != "" {
		return "", nil, fmt.Errorf("API error: %s", chatResp.Error.Message)
	}

	usage := &TokenUsage{
		PromptTokens:     chatResp.Usage.PromptTokens,
		CompletionTokens: chatResp.Usage.CompletionTokens,
		TotalTokens:      chatResp.Usage.TotalTokens,
	}

	if len(chatResp.Choices) > 0 {
		return chatResp.Choices[0].Message.Content, usage, nil
	}

	return "", usage, fmt.Errorf("no response from API")
}

func (c *OpenAIClient) SetModel(modelName string) {
	c.Model = modelName
}

func (c *OpenAIClient) GetModel() string {
	return c.Model
}

func (c *OpenAIClient) GetProvider() string {
	if c.BaseURL == "https://api.groq.com/openai/v1" {
		return "groq"
	}
	return "openai"
}
