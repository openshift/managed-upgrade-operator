package aiinsights

import (
	"context"
	"time"
)

// LLMAdapter defines the interface for LLM backends
type LLMAdapter interface {
	// Analyze sends a prompt to the LLM and returns structured JSON response
	Analyze(ctx context.Context, prompt string) ([]byte, error)

	// Name returns the model name/identifier
	Name() string

	// MaxTokens returns the maximum number of tokens this model supports
	MaxTokens() int
}

// LLMRequest represents a request to an LLM
type LLMRequest struct {
	// Prompt is the text sent to the LLM
	Prompt string

	// MaxTokens limits the response length
	MaxTokens int

	// Temperature controls randomness (0.0 = deterministic, 1.0 = creative)
	Temperature float32

	// Timeout for the request
	Timeout time.Duration
}

// LLMResponse represents a response from an LLM
type LLMResponse struct {
	// Content is the model's response
	Content string

	// Model identifies which model generated this response
	Model string

	// TokensUsed tracks consumption
	TokensUsed int

	// Duration is how long the request took
	Duration time.Duration
}

// LLMConfig holds configuration for LLM adapters
type LLMConfig struct {
	// Provider identifies the LLM backend (e.g., "openai", "azure", "mock")
	Provider string

	// APIEndpoint is the LLM API URL
	APIEndpoint string

	// APIKey for authentication (if needed)
	APIKey string

	// Model specifies which model to use
	Model string

	// MaxTokens is the default max tokens for requests
	MaxTokens int

	// Timeout is the default request timeout
	Timeout time.Duration

	// Temperature is the default temperature setting
	Temperature float32
}

// DefaultLLMConfig returns safe defaults for LLM configuration
func DefaultLLMConfig() LLMConfig {
	return LLMConfig{
		Provider:    "mock",
		Model:       "mock-model-v1",
		MaxTokens:   4096,
		Timeout:     30 * time.Second,
		Temperature: 0.2, // Low temperature for more deterministic output
	}
}
