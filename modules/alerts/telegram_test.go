package alerts

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestTelegramNotifierSuccess(t *testing.T) {
	var receivedChatID, receivedText, receivedParseMode string

	mockClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			_ = req.ParseForm()
			receivedChatID = req.FormValue("chat_id")
			receivedText = req.FormValue("text")
			receivedParseMode = req.FormValue("parse_mode")

			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(`{"ok": true}`)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	notifier := NewTelegramNotifier("secret-bot-token-12345", "987654321", 5*time.Second,
		WithBaseURL("https://api.telegram.org"),
		WithHTTPClient(mockClient),
	)

	err := notifier.Send(context.Background(), "🚨 <b>Test Alert</b>")
	if err != nil {
		t.Fatalf("Unexpected error from Send: %v", err)
	}

	if receivedChatID != "987654321" {
		t.Errorf("Expected chat_id '987654321', got '%s'", receivedChatID)
	}
	if receivedText != "🚨 <b>Test Alert</b>" {
		t.Errorf("Expected message text, got '%s'", receivedText)
	}
	if receivedParseMode != "HTML" {
		t.Errorf("Expected parse_mode 'HTML', got '%s'", receivedParseMode)
	}
}

func TestTelegramNotifierTokenRedaction(t *testing.T) {
	secretToken := "super-secret-token-abcdef"

	mockClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Body:       io.NopCloser(bytes.NewBufferString(`{"error": "invalid token super-secret-token-abcdef"}`)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	notifier := NewTelegramNotifier(secretToken, "123456", 5*time.Second,
		WithBaseURL("https://api.telegram.org"),
		WithHTTPClient(mockClient),
	)

	err := notifier.Send(context.Background(), "test")
	if err == nil {
		t.Fatal("Expected error on 401 Unauthorized")
	}

	errMsg := err.Error()
	if strings.Contains(errMsg, secretToken) {
		t.Errorf("Security violation: error message contains raw secret token: %s", errMsg)
	}
	if !strings.Contains(errMsg, "***REDACTED***") {
		t.Errorf("Expected redacted marker in error message: %s", errMsg)
	}

	// Verify String() method redaction
	safeStr := notifier.(*TelegramNotifier).String()
	if strings.Contains(safeStr, secretToken) {
		t.Errorf("Security violation: String() contains secret token: %s", safeStr)
	}
}

func TestTelegramNotifierTransportErrorRedaction(t *testing.T) {
	secretToken := "super-secret-token-12345"

	mockClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return nil, errors.New("connection failed to https://api.telegram.org/bot" + secretToken + "/sendMessage")
		}),
	}

	notifier := NewTelegramNotifier(secretToken, "123456", 5*time.Second,
		WithHTTPClient(mockClient),
	)

	err := notifier.Send(context.Background(), "test")
	if err == nil {
		t.Fatal("Expected error from failing transport")
	}

	if strings.Contains(err.Error(), secretToken) {
		t.Errorf("Security violation: raw token leaked in transport error: %v", err)
	}
}

func TestNoopNotifierWhenUnconfigured(t *testing.T) {
	n1 := NewTelegramNotifier("", "12345", 5*time.Second)
	if _, ok := n1.(*NoopNotifier); !ok {
		t.Errorf("Expected NoopNotifier when token is empty")
	}

	n2 := NewTelegramNotifier("token", "", 5*time.Second)
	if _, ok := n2.(*NoopNotifier); !ok {
		t.Errorf("Expected NoopNotifier when chatID is empty")
	}

	if err := n1.Send(context.Background(), "msg"); err != nil {
		t.Errorf("Expected nil error from NoopNotifier, got %v", err)
	}
}
