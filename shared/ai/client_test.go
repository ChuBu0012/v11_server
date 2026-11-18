package ai_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"v11-server/shared/ai"
)

func TestClient_FreeModelEnforced(t *testing.T) {
	var capturedModel string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ai.ChatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		capturedModel = req.Model

		resp := ai.ChatCompletionResponse{
			ID:    "gen-123",
			Model: req.Model,
			Choices: []ai.ChatChoice{
				{
					Index: 0,
					Message: ai.ChatMessage{
						Role:    "assistant",
						Content: "Hello from free model",
					},
					FinishReason: "stop",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := ai.NewClient("test-secret-key-12345", ai.WithBaseURL(server.URL))

	if client.Model() != ai.FreeModel {
		t.Fatalf("expected client model to be %q, got %q", ai.FreeModel, client.Model())
	}

	content, err := client.Generate(context.Background(), "system", "user prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if content != "Hello from free model" {
		t.Fatalf("unexpected content: %q", content)
	}

	if capturedModel != ai.FreeModel {
		t.Fatalf("request model payload was %q; expected strictly %q", capturedModel, ai.FreeModel)
	}
}

func TestClient_NoAPIKeyError(t *testing.T) {
	client := ai.NewClient("")
	if client.HasAPIKey() {
		t.Fatal("expected HasAPIKey to be false when no key provided")
	}

	_, err := client.Complete(context.Background(), []ai.ChatMessage{
		{Role: "user", Content: "hello"},
	})
	if err != ai.ErrNoAPIKey {
		t.Fatalf("expected ErrNoAPIKey, got: %v", err)
	}
}

func TestClient_InputRuneLimit(t *testing.T) {
	client := ai.NewClient("test-key")

	// 2001 runes exceeds limit
	longPrompt := strings.Repeat("ก", 2001)

	_, err := client.Generate(context.Background(), "", longPrompt)
	if err != ai.ErrInputTooLong {
		t.Fatalf("expected ErrInputTooLong, got: %v", err)
	}
}

func TestClient_RequestTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"late"}}]}`))
	}))
	defer server.Close()

	// Configure client with short timeout (20ms)
	client := ai.NewClient("test-key",
		ai.WithBaseURL(server.URL),
		ai.WithTimeout(20*time.Millisecond),
	)

	_, err := client.Generate(context.Background(), "", "fast test")
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestClient_SecretRedaction(t *testing.T) {
	secretKey := "sk-or-v1-super-secret-token-abcdef123456"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Mock server echoes the auth header back in error body
		auth := r.Header.Get("Authorization")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(fmt.Sprintf(`{"error":{"message":"Invalid token: %s"}}`, auth)))
	}))
	defer server.Close()

	client := ai.NewClient(secretKey, ai.WithBaseURL(server.URL))

	// Verify Stringer does not leak key
	strRep := client.String()
	if strings.Contains(strRep, secretKey) {
		t.Fatalf("client.String() leaked secret key: %s", strRep)
	}

	// Verify RedactKey helper
	sample := fmt.Sprintf("Error connecting with %s", secretKey)
	redacted := client.RedactKey(sample)
	if strings.Contains(redacted, secretKey) {
		t.Fatalf("RedactKey failed to scrub secret key: %s", redacted)
	}
	if !strings.Contains(redacted, "[REDACTED_OPENROUTER_KEY]") {
		t.Fatalf("expected redaction placeholder, got: %s", redacted)
	}

	// Verify error returned from Complete scrubs key
	_, err := client.Generate(context.Background(), "", "test")
	if err == nil {
		t.Fatal("expected error from 401 response, got nil")
	}
	if strings.Contains(err.Error(), secretKey) {
		t.Fatalf("error message leaked secret key: %v", err)
	}
}

func TestClient_MaxResponseBytesLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Send valid json prefix then 300KB of padding
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"`))
		_, _ = w.Write([]byte(strings.Repeat("A", 300*1024)))
		_, _ = w.Write([]byte(`"}}]}`))
	}))
	defer server.Close()

	client := ai.NewClient("test-key", ai.WithBaseURL(server.URL))

	// Because 300KB > 256KB limit, the reader is truncated, causing json unmarshal error rather than OOM
	_, err := client.Generate(context.Background(), "", "test large body")
	if err == nil {
		t.Fatal("expected unmarshal error due to truncated json body, got nil")
	}
}

func TestClient_EmptyChoicesError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"gen-1","choices":[]}`))
	}))
	defer server.Close()

	client := ai.NewClient("test-key", ai.WithBaseURL(server.URL))

	_, err := client.Generate(context.Background(), "", "test empty choices")
	if err != ai.ErrEmptyResponse {
		t.Fatalf("expected ErrEmptyResponse, got: %v", err)
	}
}
