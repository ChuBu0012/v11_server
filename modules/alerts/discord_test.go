package alerts

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type discordRoundTripper func(req *http.Request) (*http.Response, error)

func (f discordRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestNoopDiscordWhenUnconfigured(t *testing.T) {
	n := NewDiscordNotifier("", 5*time.Second)
	if _, ok := n.(*NoopNotifier); !ok {
		t.Errorf("expected NoopNotifier when webhook is empty, got %T", n)
	}
}

func TestDiscordSendSuccess(t *testing.T) {
	var capturedBody string
	mockClient := &http.Client{
		Transport: discordRoundTripper(func(req *http.Request) (*http.Response, error) {
			b, _ := io.ReadAll(req.Body)
			capturedBody = string(b)
			return &http.Response{
				StatusCode: http.StatusNoContent,
				Body:       io.NopCloser(strings.NewReader("")),
				Header:     make(http.Header),
			}, nil
		}),
	}

	webhook := "https://discord.com/api/webhooks/123456/SECRET_TOKEN_XYZ"
	notifier := NewDiscordNotifier(webhook, 5*time.Second, WithDiscordHTTPClient(mockClient))

	err := notifier.Send(context.Background(), "<b>Alert:</b> <code>Low Battery</code> <i>15%</i>")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(capturedBody, "**Alert:** `Low Battery` *15%*") {
		t.Errorf("expected HTML converted to Discord markdown, got %s", capturedBody)
	}
}

func TestDiscordRedactionOnError(t *testing.T) {
	mockClient := &http.Client{
		Transport: discordRoundTripper(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Body:       io.NopCloser(strings.NewReader("Invalid webhook SECRET_TOKEN_XYZ")),
				Header:     make(http.Header),
			}, nil
		}),
	}

	webhook := "https://discord.com/api/webhooks/123456/SECRET_TOKEN_XYZ"
	notifier := NewDiscordNotifier(webhook, 5*time.Second, WithDiscordHTTPClient(mockClient))

	err := notifier.Send(context.Background(), "Test message")
	if err == nil {
		t.Fatal("expected error from 400 response")
	}

	errMsg := err.Error()
	if strings.Contains(errMsg, "SECRET_TOKEN_XYZ") {
		t.Errorf("error message leaked secret token: %s", errMsg)
	}
	if !strings.Contains(errMsg, "DISCORD_TOKEN_REDACTED") {
		t.Errorf("expected token to be redacted, got: %s", errMsg)
	}
}
