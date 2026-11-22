package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
)

// Engine orchestrates multi-provider AI requests with automatic fallback,
// fast-skipping on HTTP 429 rate limits, and 1-retry with backoff on transient errors.
type Engine struct {
	providers  []Provider
	timeout    time.Duration
	retryDelay time.Duration
}

// EngineOption configures the Engine.
type EngineOption func(*Engine)

// WithEngineTimeout overrides the per-provider timeout (default: 15s).
func WithEngineTimeout(d time.Duration) EngineOption {
	return func(e *Engine) {
		if d > 0 {
			e.timeout = d
		}
	}
}

// WithEngineRetryDelay overrides the transient error retry delay (default: 1.5s).
func WithEngineRetryDelay(d time.Duration) EngineOption {
	return func(e *Engine) {
		if d > 0 {
			e.retryDelay = d
		}
	}
}

// NewEngine creates an Engine with an explicit list of providers.
func NewEngine(providers []Provider, opts ...EngineOption) *Engine {
	e := &Engine{
		providers:  providers,
		timeout:    15 * time.Second,
		retryDelay: 1500 * time.Millisecond,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// NewDefaultEngine constructs the standard Pocket Sentinel priority chain:
// 1. Groq (Primary: ultra-low latency, native JSON)
// 2. Gemini (Fallback 1: high quota, large context)
// 3. OpenRouter (Fallback 2: free tier safety net)
func NewDefaultEngine(groqKey, geminiKey, openRouterKey string, opts ...EngineOption) *Engine {
	providers := []Provider{
		NewGroqProvider(groqKey),
		NewGeminiProvider(geminiKey),
		NewOpenRouterProvider(openRouterKey),
	}
	return NewEngine(providers, opts...)
}

// Providers returns the list of configured providers.
func (e *Engine) Providers() []Provider {
	return e.providers
}

// HasAPIKey returns true if at least one configured provider has a non-empty key.
func (e *Engine) HasAPIKey() bool {
	for _, p := range e.providers {
		if p != nil && p.HasKey() {
			return true
		}
	}
	return false
}

// Generate satisfies the AIEngine and insight.TextGenerator interfaces.
func (e *Engine) Generate(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	content, _, err := e.GenerateWithProvider(ctx, Request{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		JSONMode:     false,
	})
	return content, err
}

// GenerateWithProvider executes the request across the provider fallback chain,
// returning the generated content and the name of the winning provider.
func (e *Engine) GenerateWithProvider(ctx context.Context, req Request) (string, string, error) {
	if !e.HasAPIKey() {
		return "", "", ErrAllProvidersFailed
	}

	var lastErr error
	for _, p := range e.providers {
		if p == nil || !p.HasKey() {
			continue
		}

		providerName := p.Name()
		timeout := e.timeout
		if req.Timeout > 0 {
			timeout = req.Timeout
		}

		// First attempt
		content, err := e.executeSingle(ctx, p, req, timeout)
		if err == nil {
			return content, providerName, nil
		}

		lastErr = err

		// Check if immediate skip is required (e.g. 429 Rate Limit)
		if isRateLimited(err) {
			log.Printf("[ai:engine] provider %s hit rate limit (429), advancing to next provider immediately", providerName)
			continue
		}

		// Stop immediately if parent context was cancelled
		if ctx.Err() != nil {
			return "", providerName, ctx.Err()
		}

		// Retry once on transient error
		log.Printf("[ai:engine] provider %s failed (%v), retrying once in %v...", providerName, err, e.retryDelay)
		select {
		case <-time.After(e.retryDelay):
		case <-ctx.Done():
			return "", providerName, ctx.Err()
		}

		content, err = e.executeSingle(ctx, p, req, timeout)
		if err == nil {
			log.Printf("[ai:engine] provider %s succeeded on retry", providerName)
			return content, providerName, nil
		}

		lastErr = err
		log.Printf("[ai:engine] provider %s retry failed (%v), advancing to next provider", providerName, err)
	}

	if lastErr != nil {
		return "", "", fmt.Errorf("%w: last error: %v", ErrAllProvidersFailed, lastErr)
	}
	return "", "", ErrAllProvidersFailed
}

// GenerateWithModel executes the request prioritizing the named provider if specified.
// If model is empty or "auto", it delegates to GenerateWithProvider.
func (e *Engine) GenerateWithModel(ctx context.Context, model string, req Request) (string, string, error) {
	norm := strings.ToLower(strings.TrimSpace(model))
	if norm != "" && norm != "auto" {
		for _, p := range e.providers {
			if p != nil && strings.ToLower(p.Name()) == norm && p.HasKey() {
				timeout := e.timeout
				if req.Timeout > 0 {
					timeout = req.Timeout
				}
				content, err := e.executeSingle(ctx, p, req, timeout)
				if err == nil {
					return content, p.Name(), nil
				}
				log.Printf("[ai:engine] requested provider %s failed (%v), falling back to priority chain", norm, err)
				break
			}
		}
	}
	return e.GenerateWithProvider(ctx, req)
}

// GenerateJSON requests structured JSON output and automatically unmarshals into target.
// If a provider returns unparseable JSON, it falls back to the next provider in the chain.
func (e *Engine) GenerateJSON(ctx context.Context, systemPrompt, userPrompt string, target interface{}) (string, error) {
	if !e.HasAPIKey() {
		return "", ErrAllProvidersFailed
	}

	req := Request{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		JSONMode:     true,
	}

	var lastErr error
	for _, p := range e.providers {
		if p == nil || !p.HasKey() {
			continue
		}

		providerName := p.Name()
		timeout := e.timeout
		if req.Timeout > 0 {
			timeout = req.Timeout
		}

		content, err := e.executeSingle(ctx, p, req, timeout)
		if err != nil {
			lastErr = err
			if isRateLimited(err) {
				log.Printf("[ai:engine] provider %s (json) hit rate limit (429), advancing to next provider", providerName)
				continue
			}
			// Transient retry
			select {
			case <-time.After(e.retryDelay):
			case <-ctx.Done():
				return "", ctx.Err()
			}
			content, err = e.executeSingle(ctx, p, req, timeout)
			if err != nil {
				lastErr = err
				log.Printf("[ai:engine] provider %s (json) retry failed (%v), advancing to next provider", providerName, err)
				continue
			}
		}

		// Validate & unmarshal JSON
		cleaned := CleanJSONResponse(content)
		if unmarshalErr := json.Unmarshal([]byte(cleaned), target); unmarshalErr != nil {
			lastErr = fmt.Errorf("json parse error from %s: %w (raw: %s)", providerName, unmarshalErr, cleaned)
			log.Printf("[ai:engine] provider %s returned unparseable JSON (%v), advancing to next provider", providerName, unmarshalErr)
			continue
		}

		return providerName, nil
	}

	if lastErr != nil {
		return "", fmt.Errorf("%w: last error: %v", ErrAllProvidersFailed, lastErr)
	}
	return "", ErrAllProvidersFailed
}

func (e *Engine) executeSingle(ctx context.Context, p Provider, req Request, timeout time.Duration) (string, error) {
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return p.Generate(callCtx, req)
}

func isRateLimited(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrRateLimited) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "429") ||
		strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "resource_exhausted") ||
		strings.Contains(msg, "quota")
}
