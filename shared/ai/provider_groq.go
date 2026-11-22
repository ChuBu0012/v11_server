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
	defaultGroqBaseURL = "https://api.groq.com/openai/v1"
	defaultGroqModel   = "openai/gpt-oss-120b"
)

// GroqProvider implements Provider for Groq's high-speed inference engine.
type GroqProvider struct {
	apiKey  string
	baseURL string
	model   string
	client  *http.Client
	timeout time.Duration
}

// GroqOption configures a GroqProvider.
type GroqOption func(*GroqProvider)

// WithGroqBaseURL overrides the API endpoint (useful for mock tests).
func WithGroqBaseURL(url string) GroqOption {
	return func(p *GroqProvider) { p.baseURL = strings.TrimRight(url, "/") }
}

// WithGroqModel overrides the default model.
func WithGroqModel(model string) GroqOption {
	return func(p *GroqProvider) {
		if strings.TrimSpace(model) != "" {
			p.model = strings.TrimSpace(model)
		}
	}
}

// WithGroqHTTPClient sets a custom HTTP client.
func WithGroqHTTPClient(hc *http.Client) GroqOption {
	return func(p *GroqProvider) {
		if hc != nil {
			p.client = hc
		}
	}
}

// NewGroqProvider creates a new Groq client.
func NewGroqProvider(apiKey string, opts ...GroqOption) *GroqProvider {
	p := &GroqProvider{
		apiKey:  strings.TrimSpace(apiKey),
		baseURL: defaultGroqBaseURL,
		model:   defaultGroqModel,
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

func (p *GroqProvider) Name() string {
	return "groq"
}

func (p *GroqProvider) HasKey() bool {
	return p.apiKey != ""
}

type groqMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type groqResponseFormat struct {
	Type string `json:"type"`
}

type groqChatRequest struct {
	Model          string              `json:"model"`
	Messages       []groqMessage       `json:"messages"`
	Temperature    float64             `json:"temperature"`
	MaxTokens      int                 `json:"max_tokens,omitempty"`
	ResponseFormat *groqResponseFormat `json:"response_format,omitempty"`
}

type groqChatChoice struct {
	Message groqMessage `json:"message"`
}

type groqChatResponse struct {
	Choices []groqChatChoice `json:"choices"`
	Error   *struct {
		Message string      `json:"message"`
		Type    string      `json:"type"`
		Code    interface{} `json:"code"`
	} `json:"error,omitempty"`
}

func (p *GroqProvider) Generate(ctx context.Context, req Request) (string, error) {
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
		Model:       p.model,
		Messages:    messages,
		Temperature: temp,
		MaxTokens:   req.MaxTokens,
	}

	if req.JSONMode {
		payload.ResponseFormat = &groqResponseFormat{Type: "json_object"}
	}

	reqBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("groq: marshal payload failed: %w", err)
	}

	endpoint := fmt.Sprintf("%s/chat/completions", p.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(reqBytes))
	if err != nil {
		return "", fmt.Errorf("groq: create request failed: %s", RedactKeys(err.Error(), p.apiKey))
	}

	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("groq: request failed: %s", RedactKeys(err.Error(), p.apiKey))
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return "", ErrRateLimited
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return "", fmt.Errorf("groq: read response failed: %s", RedactKeys(err.Error(), p.apiKey))
	}

	var chatResp groqChatResponse
	if err := json.Unmarshal(bodyBytes, &chatResp); err != nil {
		return "", fmt.Errorf("groq: unmarshal response failed (status %d): %s",
			resp.StatusCode, RedactKeys(string(bodyBytes), p.apiKey))
	}

	if chatResp.Error != nil {
		errMsg := RedactKeys(chatResp.Error.Message, p.apiKey)
		if resp.StatusCode == http.StatusTooManyRequests || strings.Contains(strings.ToLower(errMsg), "rate limit") {
			return "", ErrRateLimited
		}
		return "", fmt.Errorf("groq: api error (%v): %s", chatResp.Error.Code, errMsg)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("groq: unexpected http status %d: %s",
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
