package aiinsights

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// HTTPLLMAdapter implements LLMAdapter using standard HTTP REST calls
type HTTPLLMAdapter struct {
	config     LLMConfig
	httpClient *http.Client
}

// NewHTTPLLMAdapter creates a new HTTP-based LLM adapter
func NewHTTPLLMAdapter(config LLMConfig) *HTTPLLMAdapter {
	return &HTTPLLMAdapter{
		config: config,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
	}
}

// GenericLLMRequest represents a generic LLM API request
type GenericLLMRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float32   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
}

// Message represents a chat message
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// GenericLLMResponse represents a generic LLM API response
type GenericLLMResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// Analyze sends a prompt to the LLM and returns JSON response
func (h *HTTPLLMAdapter) Analyze(ctx context.Context, prompt string) ([]byte, error) {
	startTime := time.Now()

	// Build request
	reqBody := GenericLLMRequest{
		Model: h.config.Model,
		Messages: []Message{
			{
				Role:    "system",
				Content: "You are an OpenShift upgrade analyst. Always respond with valid JSON only. No markdown, no explanations outside the JSON structure.",
			},
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Temperature: h.config.Temperature,
		MaxTokens:   h.config.MaxTokens,
	}

	reqBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", h.config.APIEndpoint, bytes.NewReader(reqBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if h.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+h.config.APIKey)
	}

	// Send request
	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("LLM API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse response
	var llmResp GenericLLMResponse
	if err := json.Unmarshal(respBody, &llmResp); err != nil {
		return nil, fmt.Errorf("failed to parse LLM response: %w", err)
	}

	if len(llmResp.Choices) == 0 {
		return nil, fmt.Errorf("LLM returned no choices")
	}

	duration := time.Since(startTime)
	content := llmResp.Choices[0].Message.Content

	// Log basic info (would use proper logger in production)
	fmt.Printf("LLM request completed in %v, tokens used: %d\n", duration, llmResp.Usage.TotalTokens)

	// Return just the content as bytes
	return []byte(content), nil
}

// Name returns the model name
func (h *HTTPLLMAdapter) Name() string {
	return h.config.Model
}

// MaxTokens returns the max tokens
func (h *HTTPLLMAdapter) MaxTokens() int {
	return h.config.MaxTokens
}
