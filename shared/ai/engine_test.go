package ai

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// mockProvider implements Provider for testing.
type mockProvider struct {
	name    string
	hasKey  bool
	calls   int32
	genFunc func(ctx context.Context, req Request) (string, error)
}

func (m *mockProvider) Name() string { return m.name }
func (m *mockProvider) HasKey() bool { return m.hasKey }
func (m *mockProvider) Generate(ctx context.Context, req Request) (string, error) {
	atomic.AddInt32(&m.calls, 1)
	if m.genFunc != nil {
		return m.genFunc(ctx, req)
	}
	return "mock-response", nil
}

func TestEngine_PrimarySuccess(t *testing.T) {
	groq := &mockProvider{
		name:   "groq",
		hasKey: true,
		genFunc: func(ctx context.Context, req Request) (string, error) {
			return `{"status":"ok"}`, nil
		},
	}
	gemini := &mockProvider{name: "gemini", hasKey: true}

	engine := NewEngine([]Provider{groq, gemini}, WithEngineRetryDelay(10*time.Millisecond))

	resp, provider, err := engine.GenerateWithProvider(context.Background(), Request{UserPrompt: "hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider != "groq" {
		t.Errorf("provider = %s, want groq", provider)
	}
	if resp != `{"status":"ok"}` {
		t.Errorf("resp = %s, want status:ok", resp)
	}
	if gemini.calls != 0 {
		t.Errorf("gemini should not be called, got %d calls", gemini.calls)
	}
}

func TestEngine_RateLimitFastSkip(t *testing.T) {
	groq := &mockProvider{
		name:   "groq",
		hasKey: true,
		genFunc: func(ctx context.Context, req Request) (string, error) {
			return "", ErrRateLimited
		},
	}
	gemini := &mockProvider{
		name:   "gemini",
		hasKey: true,
		genFunc: func(ctx context.Context, req Request) (string, error) {
			return "gemini-success", nil
		},
	}

	start := time.Now()
	engine := NewEngine([]Provider{groq, gemini}, WithEngineRetryDelay(500*time.Millisecond))

	resp, provider, err := engine.GenerateWithProvider(context.Background(), Request{UserPrompt: "test"})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider != "gemini" {
		t.Errorf("provider = %s, want gemini", provider)
	}
	if resp != "gemini-success" {
		t.Errorf("resp = %s, want gemini-success", resp)
	}
	if groq.calls != 1 {
		t.Errorf("groq calls = %d, want exactly 1 (no retry on 429)", groq.calls)
	}
	// Verify fast skip without retry delay
	if elapsed > 300*time.Millisecond {
		t.Errorf("expected fast-skip without retry delay, took %v", elapsed)
	}
}

func TestEngine_TransientRetryAndFallback(t *testing.T) {
	groq := &mockProvider{
		name:   "groq",
		hasKey: true,
		genFunc: func(ctx context.Context, req Request) (string, error) {
			return "", errors.New("500 internal server error")
		},
	}
	gemini := &mockProvider{
		name:   "gemini",
		hasKey: true,
		genFunc: func(ctx context.Context, req Request) (string, error) {
			return "gemini-fallback-success", nil
		},
	}

	engine := NewEngine([]Provider{groq, gemini}, WithEngineRetryDelay(10*time.Millisecond))

	resp, provider, err := engine.GenerateWithProvider(context.Background(), Request{UserPrompt: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider != "gemini" {
		t.Errorf("provider = %s, want gemini", provider)
	}
	if resp != "gemini-fallback-success" {
		t.Errorf("resp = %s, want gemini-fallback-success", resp)
	}
	// Groq should have been called twice (initial + 1 retry)
	if groq.calls != 2 {
		t.Errorf("groq calls = %d, want 2 (1 attempt + 1 retry)", groq.calls)
	}
}

func TestEngine_GenerateJSON_MalformedJSONFallback(t *testing.T) {
	groq := &mockProvider{
		name:   "groq",
		hasKey: true,
		genFunc: func(ctx context.Context, req Request) (string, error) {
			// Returns malformed non-JSON
			return "Sorry, I cannot produce JSON right now.", nil
		},
	}
	gemini := &mockProvider{
		name:   "gemini",
		hasKey: true,
		genFunc: func(ctx context.Context, req Request) (string, error) {
			return "```json\n{\"message\":\"hello from gemini\"}\n```", nil
		},
	}

	engine := NewEngine([]Provider{groq, gemini}, WithEngineRetryDelay(10*time.Millisecond))

	type Target struct {
		Message string `json:"message"`
	}
	var res Target

	provider, err := engine.GenerateJSON(context.Background(), "system", "user", &res)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider != "gemini" {
		t.Errorf("winning provider = %s, want gemini", provider)
	}
	if res.Message != "hello from gemini" {
		t.Errorf("res.Message = %q, want 'hello from gemini'", res.Message)
	}
}

func TestEngine_AllProvidersFail(t *testing.T) {
	groq := &mockProvider{
		name:   "groq",
		hasKey: true,
		genFunc: func(ctx context.Context, req Request) (string, error) {
			return "", errors.New("groq down")
		},
	}
	gemini := &mockProvider{
		name:   "gemini",
		hasKey: false, // unconfigured
	}
	openrouter := &mockProvider{
		name:   "openrouter",
		hasKey: true,
		genFunc: func(ctx context.Context, req Request) (string, error) {
			return "", errors.New("openrouter down")
		},
	}

	engine := NewEngine([]Provider{groq, gemini, openrouter}, WithEngineRetryDelay(10*time.Millisecond))

	_, _, err := engine.GenerateWithProvider(context.Background(), Request{UserPrompt: "test"})
	if err == nil {
		t.Fatal("expected error when all providers fail, got nil")
	}
	if !errors.Is(err, ErrAllProvidersFailed) {
		t.Errorf("error = %v, want ErrAllProvidersFailed", err)
	}
	if gemini.calls != 0 {
		t.Errorf("unconfigured gemini was called %d times, want 0", gemini.calls)
	}
}
