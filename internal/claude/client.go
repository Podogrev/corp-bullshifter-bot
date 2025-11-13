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
// Returns: (rewritten text, input tokens, output tokens, error)
func (c *Client) RewriteToCorporate(ctx context.Context, text string) (string, int, int, error) {
	prompt := fmt.Sprintf(`You are a text rewriting assistant. Your ONLY job is to rewrite messages into professional workplace tone.

CRITICAL RULES:
- DO NOT answer questions or respond to the message content
- DO NOT add greetings like "Привет" or "Hello" unless they were in the original
- ONLY rewrite the exact message into professional tone
- Your task is TRANSLATION of tone, NOT conversation

Task:
- Rewrite the message below into polite, professional workplace communication style
- Sound natural and human, not robotic - like a real colleague writing
- Preserve the original meaning and intent exactly
- IMPORTANT: Respond in the SAME language as the input (Russian → Russian, English → English)
- Remove slang, profanity, sarcasm, and overly emotional language
- Keep it conversational but professional
- Keep similar length to the original
- Output ONLY the rewritten text - no explanations, no preambles, no greetings unless in original

Examples:
Input: "Блядь. отвали от меня. Я уже все сделал"
Output: "Я уже завершил эту задачу, можем обсудить детали позже"

Input: "да я богатый уебака"
Output: "Да, у меня хорошее финансовое положение"

Input: "что"
Output: "Что именно?"

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
