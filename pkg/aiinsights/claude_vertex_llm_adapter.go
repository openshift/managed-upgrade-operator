package aiinsights

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// ClaudeVertexLLMAdapter implements LLMAdapter using Claude on Google Vertex AI
type ClaudeVertexLLMAdapter struct {
	config     LLMConfig
	httpClient *http.Client
	projectID  string
	location   string
	modelID    string
}

// VertexLLMConfig extends LLMConfig with Vertex-specific fields
type VertexLLMConfig struct {
	LLMConfig
	ProjectID string
	Location  string
	ModelID   string
}

// NewClaudeVertexLLMAdapter creates a new Claude Vertex AI adapter
func NewClaudeVertexLLMAdapter(config VertexLLMConfig) *ClaudeVertexLLMAdapter {
	return &ClaudeVertexLLMAdapter{
		config: config.LLMConfig,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		projectID: config.ProjectID,
		location:  config.Location,
		modelID:   config.ModelID,
	}
}

// ClaudeMessage represents a message in Claude's format
type ClaudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ClaudeVertexRequest represents a request to Vertex AI Claude API
type ClaudeVertexRequest struct {
	AnthropicVersion string          `json:"anthropic_version"`
	Messages         []ClaudeMessage `json:"messages"`
	MaxTokens        int             `json:"max_tokens"`
	Temperature      float32         `json:"temperature,omitempty"`
	System           string          `json:"system,omitempty"`
}

// ClaudeVertexResponse represents a response from Vertex AI Claude API
type ClaudeVertexResponse struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Role    string `json:"role"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Model        string `json:"model"`
	StopReason   string `json:"stop_reason"`
	StopSequence string `json:"stop_sequence,omitempty"`
	Usage        struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// Analyze sends a prompt to Claude on Vertex AI and returns JSON response
func (c *ClaudeVertexLLMAdapter) Analyze(ctx context.Context, prompt string) ([]byte, error) {
	startTime := time.Now()

	// Get GCP access token
	accessToken, err := c.getAccessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get GCP access token: %w", err)
	}

	// Build Vertex AI endpoint URL
	endpoint := fmt.Sprintf(
		"https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/anthropic/models/%s:rawPredict",
		c.location,
		c.projectID,
		c.location,
		c.modelID,
	)

	// Build request body with Vertex-specific format
	reqBody := ClaudeVertexRequest{
		AnthropicVersion: "vertex-2023-10-16",
		Messages: []ClaudeMessage{
			{
				Role:    "user",
				Content: prompt,
			},
		},
		MaxTokens:   c.config.MaxTokens,
		Temperature: c.config.Temperature,
		System:      "You are an expert OpenShift upgrade analyst. Always respond with valid JSON only. No markdown code blocks, no explanations outside the JSON structure. Focus on providing actionable insights based on cluster events and logs.",
	}

	reqBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Vertex request: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(reqBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create Vertex request: %w", err)
	}

	// Set headers for Vertex AI
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send Vertex request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read Vertex response: %w", err)
	}

	// Handle non-200 status codes
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Vertex AI API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse Vertex response
	var vertexResp ClaudeVertexResponse
	if err := json.Unmarshal(respBody, &vertexResp); err != nil {
		return nil, fmt.Errorf("failed to parse Vertex response: %w", err)
	}

	if len(vertexResp.Content) == 0 {
		return nil, fmt.Errorf("Vertex returned no content blocks")
	}

	duration := time.Since(startTime)

	// Extract text content from all content blocks
	var fullContent string
	for _, content := range vertexResp.Content {
		if content.Type == "text" {
			fullContent += content.Text
		}
	}

	if fullContent == "" {
		return nil, fmt.Errorf("Vertex returned empty text content")
	}

	// Log basic info (would use proper logger in production)
	fmt.Printf("Vertex AI request completed in %v, input tokens: %d, output tokens: %d, total: %d\n",
		duration,
		vertexResp.Usage.InputTokens,
		vertexResp.Usage.OutputTokens,
		vertexResp.Usage.InputTokens+vertexResp.Usage.OutputTokens)

	// Return the content as bytes
	return []byte(fullContent), nil
}

// getAccessToken retrieves GCP access token using gcloud or service account
func (c *ClaudeVertexLLMAdapter) getAccessToken(ctx context.Context) (string, error) {
	// If APIKey is provided, use it as the access token (for service account JSON key)
	if c.config.APIKey != "" {
		// In production, you would use google-auth-library to properly handle
		// service account authentication. For now, assume APIKey is a pre-generated token
		return c.config.APIKey, nil
	}

	// Fall back to gcloud auth
	cmd := exec.CommandContext(ctx, "gcloud", "auth", "print-access-token")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to run gcloud auth: %w", err)
	}

	token := strings.TrimSpace(string(output))
	if token == "" {
		return "", fmt.Errorf("gcloud returned empty access token")
	}

	return token, nil
}

// Name returns the model name
func (c *ClaudeVertexLLMAdapter) Name() string {
	return c.modelID
}

// MaxTokens returns the max tokens
func (c *ClaudeVertexLLMAdapter) MaxTokens() int {
	return c.config.MaxTokens
}
