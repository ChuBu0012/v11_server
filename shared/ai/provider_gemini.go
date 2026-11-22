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
	defaultGeminiBaseURL = "https://generativelanguage.googleapis.com"
	defaultGeminiModel   = "gemini-2.5-flash"
)

// GeminiProvider implements Provider for Google Generative Language API.
type GeminiProvider struct {
	apiKey  string
	baseURL string
	model   string
	client  *http.Client
	timeout time.Duration
}

// GeminiOption configures a GeminiProvider.
type GeminiOption func(*GeminiProvider)

// WithGeminiBaseURL overrides the API endpoint (useful for mock tests).
func WithGeminiBaseURL(url string) GeminiOption {
	return func(p *GeminiProvider) { p.baseURL = strings.TrimRight(url, "/") }
}

// WithGeminiModel overrides the default model.
func WithGeminiModel(model string) GeminiOption {
	return func(p *GeminiProvider) {
		if strings.TrimSpace(model) != "" {
			p.model = strings.TrimSpace(model)
		}
	}
}

// WithGeminiHTTPClient sets a custom HTTP client.
func WithGeminiHTTPClient(hc *http.Client) GeminiOption {
	return func(p *GeminiProvider) {
		if hc != nil {
			p.client = hc
		}
	}
}

// NewGeminiProvider creates a new Gemini client.
func NewGeminiProvider(apiKey string, opts ...GeminiOption) *GeminiProvider {
	p := &GeminiProvider{
		apiKey:  strings.TrimSpace(apiKey),
		baseURL: defaultGeminiBaseURL,
		model:   defaultGeminiModel,
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

func (p *GeminiProvider) Name() string {
	return "gemini"
}

func (p *GeminiProvider) HasKey() bool {
	return p.apiKey != ""
}

type geminiPartPayload struct {
	Text string `json:"text"`
}

type geminiContentPayload struct {
	Parts []geminiPartPayload `json:"parts"`
}

type geminiGenConfig struct {
	Temperature      float64 `json:"temperature"`
	MaxOutputTokens  int     `json:"maxOutputTokens,omitempty"`
	ResponseMimeType string  `json:"responseMimeType,omitempty"`
}

type geminiRESTRequest struct {
	Contents          []geminiContentPayload `json:"contents"`
	SystemInstruction *geminiContentPayload  `json:"systemInstruction,omitempty"`
	GenerationConfig  geminiGenConfig        `json:"generationConfig"`
}

type geminiRESTCandidate struct {
	Content geminiContentPayload `json:"content"`
}

type geminiRESTResponse struct {
	Candidates []geminiRESTCandidate `json:"candidates"`
	Error      *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error,omitempty"`
}

func (p *GeminiProvider) Generate(ctx context.Context, req Request) (string, error) {
	if !p.HasKey() {
		return "", ErrNoAPIKey
	}

	temp := 0.2
	if req.Temperature > 0 {
		temp = req.Temperature
	}

	maxTokens := 2048
	if req.MaxTokens > 0 {
		maxTokens = req.MaxTokens
	}

	payload := geminiRESTRequest{
		Contents: []geminiContentPayload{
			{
				Parts: []geminiPartPayload{{Text: req.UserPrompt}},
			},
		},
		GenerationConfig: geminiGenConfig{
			Temperature:     temp,
			MaxOutputTokens: maxTokens,
		},
	}

	if strings.TrimSpace(req.SystemPrompt) != "" {
		payload.SystemInstruction = &geminiContentPayload{
			Parts: []geminiPartPayload{{Text: strings.TrimSpace(req.SystemPrompt)}},
		}
	}

	if req.JSONMode {
		payload.GenerationConfig.ResponseMimeType = "application/json"
	}

	reqBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("gemini: marshal payload failed: %w", err)
	}

	// Safely pass API key via x-goog-api-key header instead of URL query parameter
	endpoint := fmt.Sprintf("%s/v1beta/models/%s:generateContent", p.baseURL, p.model)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(reqBytes))
	if err != nil {
		return "", fmt.Errorf("gemini: create request failed: %s", RedactKeys(err.Error(), p.apiKey))
	}

	httpReq.Header.Set("x-goog-api-key", p.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("gemini: request failed: %s", RedactKeys(err.Error(), p.apiKey))
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return "", ErrRateLimited
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return "", fmt.Errorf("gemini: read response failed: %s", RedactKeys(err.Error(), p.apiKey))
	}

	var geminiResp geminiRESTResponse
	if err := json.Unmarshal(bodyBytes, &geminiResp); err != nil {
		return "", fmt.Errorf("gemini: unmarshal response failed (status %d): %s",
			resp.StatusCode, RedactKeys(string(bodyBytes), p.apiKey))
	}

	if geminiResp.Error != nil {
		errMsg := RedactKeys(geminiResp.Error.Message, p.apiKey)
		if geminiResp.Error.Code == http.StatusTooManyRequests ||
			geminiResp.Error.Status == "RESOURCE_EXHAUSTED" ||
			strings.Contains(strings.ToLower(errMsg), "quota") ||
			strings.Contains(strings.ToLower(errMsg), "rate limit") {
			return "", ErrRateLimited
		}
		return "", fmt.Errorf("gemini: api error %d (%s): %s",
			geminiResp.Error.Code, geminiResp.Error.Status, errMsg)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("gemini: unexpected http status %d: %s",
			resp.StatusCode, RedactKeys(string(bodyBytes), p.apiKey))
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return "", ErrEmptyResponse
	}

	var sb strings.Builder
	for _, part := range geminiResp.Candidates[0].Content.Parts {
		sb.WriteString(part.Text)
	}
	content := strings.TrimSpace(sb.String())
	if content == "" {
		return "", ErrEmptyResponse
	}

	return content, nil
}
