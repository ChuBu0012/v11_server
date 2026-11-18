package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode/utf8"
)

// Constants defining OpenRouter client constraints and limits.
const (
	// FreeModel is the strictly enforced model identifier for OpenRouter.
	// Paid models and paid fallbacks are strictly prohibited.
	FreeModel = "openrouter/free"

	// DefaultBaseURL is the standard OpenRouter API base URL.
	DefaultBaseURL = "https://openrouter.ai/api/v1"

	// DefaultTimeout is the request timeout limit (15 seconds).
	DefaultTimeout = 15 * time.Second

	// MaxInputRunes limits the input prompt length to prevent memory exhaustion and excessive cost.
	MaxInputRunes = 2000

	// MaxResponseBytes limits the response body reading to 256 KB.
	MaxResponseBytes = 256 * 1024
)

// Common errors.
var (
	ErrNoAPIKey       = errors.New("openrouter: API key is not configured")
	ErrInputTooLong   = fmt.Errorf("openrouter: input exceeds limit of %d runes", MaxInputRunes)
	ErrEmptyResponse  = errors.New("openrouter: empty response received from model")
	ErrPaidModelError = errors.New("openrouter: paid models are strictly forbidden; only openrouter/free is permitted")
)

// ChatMessage represents a single message in a chat completion conversation.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatCompletionRequest is the payload sent to OpenRouter's chat completions endpoint.
type ChatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Temperature float64       `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
}

// ChatChoice represents a choice returned by OpenRouter.
type ChatChoice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

// APIErrorDetail holds details of an error returned by OpenRouter API.
type APIErrorDetail struct {
	Code    interface{} `json:"code"`
	Message string      `json:"message"`
	Type    string      `json:"type"`
}

// ChatCompletionResponse represents the JSON response from OpenRouter.
type ChatCompletionResponse struct {
	ID      string          `json:"id"`
	Model   string          `json:"model"`
	Choices []ChatChoice    `json:"choices"`
	Error   *APIErrorDetail `json:"error,omitempty"`
}

// Client is a server-side OpenRouter client enforcing strict free-model constraints.
type Client struct {
	apiKey  string
	baseURL string
	client  *http.Client
	timeout time.Duration
}

// Option configures the Client.
type Option func(*Client)

// WithBaseURL overrides the API base URL (useful for mock testing with httptest).
func WithBaseURL(url string) Option {
	return func(c *Client) {
		c.baseURL = strings.TrimRight(url, "/")
	}
}

// WithHTTPClient overrides the internal http.Client (useful for testing custom transports).
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		if httpClient != nil {
			c.client = httpClient
		}
	}
}

// WithTimeout overrides the default request timeout.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) {
		if d > 0 {
			c.timeout = d
		}
	}
}

// NewClient creates a new OpenRouter client.
// If apiKey is empty, it attempts to read OPENROUTER_API_KEY from the environment.
// The model is permanently hardcoded to FreeModel ("openrouter/free").
func NewClient(apiKey string, opts ...Option) *Client {
	key := strings.TrimSpace(apiKey)
	if key == "" {
		key = strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY"))
	}

	c := &Client{
		apiKey:  key,
		baseURL: DefaultBaseURL,
		timeout: DefaultTimeout,
		client: &http.Client{
			Timeout: DefaultTimeout,
		},
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// HasAPIKey checks whether the client has an API key configured.
func (c *Client) HasAPIKey() bool {
	return c.apiKey != ""
}

// String implements fmt.Stringer to prevent accidental leakage of the API key in logs.
func (c *Client) String() string {
	return fmt.Sprintf("OpenRouterClient[model=%s, configured=%t]", FreeModel, c.apiKey != "")
}

// Model returns the enforced model name. Always returns FreeModel.
func (c *Client) Model() string {
	return FreeModel
}

// RedactKey strips any occurrence of the client's API key from a string.
func (c *Client) RedactKey(s string) string {
	if c.apiKey == "" || s == "" {
		return s
	}
	return strings.ReplaceAll(s, c.apiKey, "[REDACTED_OPENROUTER_KEY]")
}

// Complete sends chat messages to OpenRouter using strictly the FreeModel and returns the generated content.
func (c *Client) Complete(ctx context.Context, messages []ChatMessage) (string, error) {
	if c.apiKey == "" {
		return "", ErrNoAPIKey
	}

	// Validate input rune length across all messages
	totalRunes := 0
	for _, m := range messages {
		totalRunes += utf8.RuneCountInString(m.Content)
	}
	if totalRunes > MaxInputRunes {
		return "", ErrInputTooLong
	}

	// Build request payload - strictly enforce FreeModel
	reqPayload := ChatCompletionRequest{
		Model:       FreeModel,
		Messages:    messages,
		Temperature: 0.2, // low temperature for deterministic structured outputs
	}

	reqBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return "", fmt.Errorf("openrouter: marshal request failed: %w", err)
	}

	// Apply request timeout context
	reqCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	endpoint := fmt.Sprintf("%s/chat/completions", c.baseURL)
	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, bytes.NewReader(reqBytes))
	if err != nil {
		return "", fmt.Errorf("openrouter: create request failed: %s", c.RedactKey(err.Error()))
	}

	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("HTTP-Referer", "https://github.com/v11-server/pocket-sentinel")
	httpReq.Header.Set("X-Title", "Pocket Sentinel")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("openrouter: request error: %s", c.RedactKey(err.Error()))
	}
	defer resp.Body.Close()

	// Read response with strict size limit to prevent memory blow-up on phone
	limitReader := io.LimitReader(resp.Body, MaxResponseBytes)
	bodyBytes, err := io.ReadAll(limitReader)
	if err != nil {
		return "", fmt.Errorf("openrouter: read response failed: %s", c.RedactKey(err.Error()))
	}

	var chatResp ChatCompletionResponse
	if err := json.Unmarshal(bodyBytes, &chatResp); err != nil {
		return "", fmt.Errorf("openrouter: parse response failed (status %d): %s",
			resp.StatusCode, c.RedactKey(string(bodyBytes)))
	}

	// Check for API-level errors
	if chatResp.Error != nil {
		return "", fmt.Errorf("openrouter: api error: %s", c.RedactKey(chatResp.Error.Message))
	}

	// HTTP status check
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("openrouter: unexpected http status %d: %s",
			resp.StatusCode, c.RedactKey(string(bodyBytes)))
	}

	if len(chatResp.Choices) == 0 {
		return "", ErrEmptyResponse
	}

	content := strings.TrimSpace(chatResp.Choices[0].Message.Content)
	if content == "" {
		return "", ErrEmptyResponse
	}

	return content, nil
}

// Generate is a convenience helper sending a system prompt and a user prompt.
func (c *Client) Generate(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	messages := []ChatMessage{}
	if strings.TrimSpace(systemPrompt) != "" {
		messages = append(messages, ChatMessage{
			Role:    "system",
			Content: strings.TrimSpace(systemPrompt),
		})
	}
	messages = append(messages, ChatMessage{
		Role:    "user",
		Content: strings.TrimSpace(userPrompt),
	})
	return c.Complete(ctx, messages)
}
