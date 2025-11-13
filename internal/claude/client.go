package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const anthropicVersion = "2023-06-01"

// Client is a Claude API client
type Client struct {
	apiKey     string
	apiURL     string
	model      string
	httpClient *http.Client
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
	return &Client{
		apiKey:     apiKey,
		apiURL:     apiURL,
		model:      model,
		httpClient: httpClient,
	}
}

// RewriteToCorporate rewrites text into polite corporate style
func (c *Client) RewriteToCorporate(ctx context.Context, text string) (string, int, error) {
	prompt := fmt.Sprintf(`You are a corporate communication assistant.

Task:
- Rewrite the user's message into a polite, professional reply suitable for workplace communication.
- The reply should sound natural and human, like a real person writing in a work chat or email - not robotic or overly formal.
- Preserve the original meaning and intent, but adjust the tone to be neutral and appropriate for professional settings.
- IMPORTANT: Detect the language of the user's message and respond in the SAME language (e.g., if the message is in Russian, respond in Russian; if in English, respond in English).
- Avoid slang, sarcasm, offensive language, or overly emotional expressions.
- Keep it conversational and friendly, but professional. Use natural phrasing that sounds like how colleagues actually talk to each other.
- Keep the response focused and reasonably short (a few sentences unless the input is clearly long and detailed).
- Output ONLY the final rewritten text, without any preamble, explanation, or commentary. Do not add phrases like "Here's a professional version" or "I apologize" - just output the rewritten text directly.

User message:
%s`, text)

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
		return "", 0, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var claudeResp Response
	if err := json.Unmarshal(body, &claudeResp); err != nil {
		return "", 0, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if len(claudeResp.Content) == 0 {
		return "", 0, fmt.Errorf("no content in response")
	}

	totalTokens := claudeResp.Usage.InputTokens + claudeResp.Usage.OutputTokens
	return claudeResp.Content[0].Text, totalTokens, nil
}
