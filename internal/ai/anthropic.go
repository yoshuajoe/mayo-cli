package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type AnthropicClient struct {
	APIKey  string
	Model   string
	Version string
}

func NewAnthropicClient(apiKey, defaultModel string) *AnthropicClient {
	if defaultModel == "" {
		defaultModel = "claude-3-5-sonnet-20241022"
	}
	return &AnthropicClient{
		APIKey:  apiKey,
		Model:   defaultModel,
		Version: "2023-06-01",
	}
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
	MaxTokens int                `json:"max_tokens"`
}

type anthropicResponse struct {
	Content []struct {
		Text string `json:"text"`
		Type string `json:"type"`
	} `json:"content"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

func (c *AnthropicClient) GenerateResponse(ctx context.Context, systemPrompt, userPrompt string) (string, *TokenUsage, error) {
	reqBody := anthropicRequest{
		Model:     c.Model,
		System:    systemPrompt,
		Messages:  []anthropicMessage{{Role: "user", Content: userPrompt}},
		MaxTokens: 4096,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.anthropic.com/v1/messages", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.APIKey)
	req.Header.Set("anthropic-version", c.Version)

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

	var anthroResp anthropicResponse
	if err := json.Unmarshal(body, &anthroResp); err != nil {
		return "", nil, err
	}

	if anthroResp.Error.Message != "" {
		return "", nil, fmt.Errorf("anthropic error (%s): %s", anthroResp.Error.Type, anthroResp.Error.Message)
	}

	usage := &TokenUsage{
		PromptTokens:     anthroResp.Usage.InputTokens,
		CompletionTokens: anthroResp.Usage.OutputTokens,
		TotalTokens:      anthroResp.Usage.InputTokens + anthroResp.Usage.OutputTokens,
	}

	if len(anthroResp.Content) > 0 {
		return anthroResp.Content[0].Text, usage, nil
	}

	return "", usage, fmt.Errorf("no response from anthropic")
}

func (c *AnthropicClient) SetModel(modelName string) {
	c.Model = modelName
}

func (c *AnthropicClient) GetModel() string {
	return c.Model
}

func (c *AnthropicClient) GetProvider() string {
	return "anthropic"
}
