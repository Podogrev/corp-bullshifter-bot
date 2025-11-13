package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
)

const anthropicVersion = "2023-06-01"

// Client is a Claude API client
type Client struct {
	apiKey       string
	apiURL       string
	model        string
	httpClient   *http.Client
	systemPrompt string
}

// Request represents a Claude API request
type Request struct {
	Model       string    `json:"model"`
	MaxTokens   int       `json:"max_tokens"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature,omitempty"`
}

// Message represents a message in the conversation
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Response represents a Claude API response
type Response struct {
	ID      string         `json:"id"`
	Type    string         `json:"type"`
	Role    string         `json:"role"`
	Content []ContentBlock `json:"content"`
	Model   string         `json:"model"`
	Usage   Usage          `json:"usage"`
}

// ContentBlock represents a content block in the response
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Usage represents token usage information
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// New creates a new Claude API client
func New(apiKey, apiURL, model string, httpClient *http.Client) *Client {
	// Load system prompt from file
	promptPath := os.Getenv("PROMPT_FILE")
	if promptPath == "" {
		promptPath = "/app/prompts/system_prompt.txt"
	}

	promptBytes, err := os.ReadFile(promptPath)
	if err != nil {
		log.Printf("Warning: failed to load prompt from %s: %v. Using default prompt.", promptPath, err)
		promptBytes = []byte(getDefaultPrompt())
	}

	return &Client{
		apiKey:       apiKey,
		apiURL:       apiURL,
		model:        model,
		httpClient:   httpClient,
		systemPrompt: string(promptBytes),
	}
}

// getDefaultPrompt returns a fallback prompt if file is not found
func getDefaultPrompt() string {
	return `You are a text rewriting assistant. Rewrite messages into professional workplace tone.

Rules:
- Preserve meaning and language
- Remove profanity and slang
- Keep it natural and concise
- Output only the rewritten text`
}

// RewriteToCorporate rewrites text into polite corporate style
// Returns: (rewritten text, input tokens, output tokens, error)
func (c *Client) RewriteToCorporate(ctx context.Context, text string) (string, int, int, error) {
	prompt := fmt.Sprintf("%s%s", c.systemPrompt, text)

	reqBody := Request{
		Model:     c.model,
		MaxTokens: 1024,
		Messages: []Message{
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Temperature: 0.7,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", 0, 0, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", 0, 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", 0, 0, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, 0, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", 0, 0, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var claudeResp Response
	if err := json.Unmarshal(body, &claudeResp); err != nil {
		return "", 0, 0, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if len(claudeResp.Content) == 0 {
		return "", 0, 0, fmt.Errorf("no content in response")
	}

	return claudeResp.Content[0].Text, claudeResp.Usage.InputTokens, claudeResp.Usage.OutputTokens, nil
}
