package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	defaultOpenRouterBaseURL = "https://openrouter.ai/api/v1"
	openRouterFreeModel      = "openrouter/free"
)

// OpenRouterProvider implements Provider for OpenRouter strictly adhering to the free tier.
type OpenRouterProvider struct {
	apiKey  string
	baseURL string
	client  *http.Client
	timeout time.Duration
}

// OpenRouterOption configures an OpenRouterProvider.
type OpenRouterOption func(*OpenRouterProvider)

// WithOpenRouterBaseURL overrides the API endpoint (useful for mock tests).
func WithOpenRouterBaseURL(url string) OpenRouterOption {
	return func(p *OpenRouterProvider) { p.baseURL = strings.TrimRight(url, "/") }
}

// WithOpenRouterHTTPClient sets a custom HTTP client.
func WithOpenRouterHTTPClient(hc *http.Client) OpenRouterOption {
	return func(p *OpenRouterProvider) {
		if hc != nil {
			p.client = hc
		}
	}
}

// NewOpenRouterProvider creates a new OpenRouter client.
func NewOpenRouterProvider(apiKey string, opts ...OpenRouterOption) *OpenRouterProvider {
	p := &OpenRouterProvider{
		apiKey:  strings.TrimSpace(apiKey),
		baseURL: defaultOpenRouterBaseURL,
		timeout: 15 * time.Second,
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

func (p *OpenRouterProvider) Name() string {
	return "openrouter"
}

func (p *OpenRouterProvider) HasKey() bool {
	return p.apiKey != ""
}

func (p *OpenRouterProvider) Generate(ctx context.Context, req Request) (string, error) {
	if !p.HasKey() {
		return "", ErrNoAPIKey
	}

	messages := make([]groqMessage, 0, 2)
	if strings.TrimSpace(req.SystemPrompt) != "" {
		messages = append(messages, groqMessage{
			Role:    "system",
			Content: strings.TrimSpace(req.SystemPrompt),
		})
	}
	messages = append(messages, groqMessage{
		Role:    "user",
		Content: strings.TrimSpace(req.UserPrompt),
	})

	temp := 0.2
	if req.Temperature > 0 {
		temp = req.Temperature
	}

	payload := groqChatRequest{
		Model:       openRouterFreeModel,
		Messages:    messages,
		Temperature: temp,
		MaxTokens:   req.MaxTokens,
	}

	reqBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("openrouter: marshal payload failed: %w", err)
	}

	endpoint := fmt.Sprintf("%s/chat/completions", p.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(reqBytes))
	if err != nil {
		return "", fmt.Errorf("openrouter: create request failed: %s", RedactKeys(err.Error(), p.apiKey))
	}

	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("HTTP-Referer", "https://github.com/v11-server/pocket-sentinel")
	httpReq.Header.Set("X-Title", "Pocket Sentinel")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("openrouter: request failed: %s", RedactKeys(err.Error(), p.apiKey))
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return "", ErrRateLimited
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return "", fmt.Errorf("openrouter: read response failed: %s", RedactKeys(err.Error(), p.apiKey))
	}

	var chatResp groqChatResponse
	if err := json.Unmarshal(bodyBytes, &chatResp); err != nil {
		return "", fmt.Errorf("openrouter: unmarshal response failed (status %d): %s",
			resp.StatusCode, RedactKeys(string(bodyBytes), p.apiKey))
	}

	if chatResp.Error != nil {
		errMsg := RedactKeys(chatResp.Error.Message, p.apiKey)
		if resp.StatusCode == http.StatusTooManyRequests || strings.Contains(strings.ToLower(errMsg), "rate limit") {
			return "", ErrRateLimited
		}
		return "", fmt.Errorf("openrouter: api error (%v): %s", chatResp.Error.Code, errMsg)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("openrouter: unexpected http status %d: %s",
			resp.StatusCode, RedactKeys(string(bodyBytes), p.apiKey))
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
