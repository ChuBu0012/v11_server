package ai

import (
	"context"
	"errors"
	"strings"
	"time"
)

// Common AI errors.
var (
	// ErrRateLimited indicates an HTTP 429 Too Many Requests response.
	ErrRateLimited = errors.New("ai: rate limited (HTTP 429)")

	// ErrAllProvidersFailed indicates that all configured providers in the loop failed.
	ErrAllProvidersFailed = errors.New("ai: all fallback providers failed or were unconfigured")
)

// Request defines the standard parameters for an AI generation request.
type Request struct {
	SystemPrompt string        `json:"system_prompt,omitempty"`
	UserPrompt   string        `json:"user_prompt"`
	JSONMode     bool          `json:"json_mode,omitempty"`
	Temperature  float64       `json:"temperature,omitempty"`
	MaxTokens    int           `json:"max_tokens,omitempty"`
	Timeout      time.Duration `json:"timeout,omitempty"`
}

// Provider defines the interface implemented by each LLM provider.
type Provider interface {
	// Name returns the provider identifier (e.g. "groq", "gemini", "openrouter").
	Name() string

	// HasKey returns true if the provider has a non-empty API key configured.
	HasKey() bool

	// Generate executes the prompt against the provider and returns the raw response string.
	Generate(ctx context.Context, req Request) (string, error)
}

// AIEngine is the generalized interface accepted by consumer modules (proofreader,
// schedule parser, incident summary, monitor insight). Both *Engine and *Client satisfy it.
type AIEngine interface {
	Generate(ctx context.Context, systemPrompt, userPrompt string) (string, error)
	HasAPIKey() bool
}

// RedactKeys replaces secret keys with redaction placeholders to prevent credential leakage in logs.
func RedactKeys(text string, keys ...string) string {
	if text == "" {
		return ""
	}
	out := text
	for _, k := range keys {
		trimmed := strings.TrimSpace(k)
		if trimmed != "" && len(trimmed) >= 8 {
			out = strings.ReplaceAll(out, trimmed, "[REDACTED_API_KEY]")
		}
	}
	return out
}

// CleanJSONResponse extracts a valid JSON object or array from LLM output,
// stripping any Markdown code fences (e.g. ```json ... ```) and whitespace.
func CleanJSONResponse(raw string) string {
	cleaned := strings.TrimSpace(raw)
	if strings.HasPrefix(cleaned, "```") {
		lines := strings.Split(cleaned, "\n")
		if len(lines) >= 2 {
			if strings.HasPrefix(lines[0], "```") {
				lines = lines[1:]
			}
			if len(lines) > 0 && strings.HasPrefix(lines[len(lines)-1], "```") {
				lines = lines[:len(lines)-1]
			}
			cleaned = strings.TrimSpace(strings.Join(lines, "\n"))
		}
	}
	return cleaned
}
