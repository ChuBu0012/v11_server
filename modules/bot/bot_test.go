package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestPoller(t *testing.T) {
	var pollCount int64
	fakeUpdates := []Update{
		{
			UpdateID: 100,
			Message: &Message{
				MessageID: 1,
				Chat:      &Chat{ID: 123456},
				Text:      "/ping",
			},
		},
		{
			UpdateID: 101,
			Message: &Message{
				MessageID: 2,
				Chat:      &Chat{ID: 123456},
				Text:      "/status",
			},
		},
	}

	mockTransport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		currentCount := atomic.AddInt64(&pollCount, 1)

		if r.URL.Path == "/botTEST_SECRET_TOKEN/getUpdates" {
			offset := r.URL.Query().Get("offset")

			if currentCount == 1 {
				if offset != "" {
					t.Errorf("First poll expected empty offset, got %s", offset)
				}
				resBytes, _ := json.Marshal(fakeUpdates)
				resp := APIResponse{
					OK:     true,
					Result: resBytes,
				}
				bodyBytes, _ := json.Marshal(resp)
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
				}, nil
			}

			if offset != "102" {
				t.Errorf("Second poll expected offset 102, got %s", offset)
			}

			resp := APIResponse{
				OK:     true,
				Result: json.RawMessage("[]"),
			}
			bodyBytes, _ := json.Marshal(resp)
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
			}, nil
		}

		return &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(strings.NewReader("404 not found")),
		}, nil
	})

	httpClient := &http.Client{Transport: mockTransport}
	client := NewClient("TEST_SECRET_TOKEN", WithBaseURL("https://api.telegram.org"), WithHTTPClient(httpClient))

	var handledCount int64
	handler := HandlerFunc(func(ctx context.Context, u Update) error {
		atomic.AddInt64(&handledCount, 1)
		return nil
	})

	poller := NewPoller(client, handler)
	poller.SetTimeout(1)

	ctx, cancel := context.WithCancel(context.Background())

	// Run first poll
	n, err := poller.PollOnce(ctx)
	if err != nil {
		t.Fatalf("PollOnce failed: %v", err)
	}
	if n != 2 {
		t.Errorf("Expected 2 updates, got %d", n)
	}
	if poller.GetOffset() != 102 {
		t.Errorf("Expected offset 102, got %d", poller.GetOffset())
	}
	if atomic.LoadInt64(&handledCount) != 2 {
		t.Errorf("Expected 2 handled updates, got %d", handledCount)
	}

	// Run second poll (empty)
	n2, err := poller.PollOnce(ctx)
	if err != nil {
		t.Fatalf("PollOnce empty failed: %v", err)
	}
	if n2 != 0 {
		t.Errorf("Expected 0 updates, got %d", n2)
	}
	if poller.GetOffset() != 102 {
		t.Errorf("Expected offset to stay 102, got %d", poller.GetOffset())
	}

	// Test Poller Start and clean cancellation
	stopped := make(chan struct{})
	go func() {
		poller.Start(ctx)
		close(stopped)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-stopped:
		// Clean stop!
	case <-time.After(2 * time.Second):
		t.Fatal("Poller did not stop promptly on context cancellation")
	}
}

func TestClientTokenRedaction(t *testing.T) {
	client := NewClient("SUPER_SECRET_TOKEN_12345", WithBaseURL("http://invalid.localhost:99999"))
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := client.GetUpdates(ctx, 0, 10, 1)
	if err == nil {
		t.Fatal("Expected error on invalid URL, got nil")
	}

	errStr := err.Error()
	if containsSubstring(errStr, "SUPER_SECRET_TOKEN_12345") {
		t.Errorf("Security leak: error contains unredacted token: %s", errStr)
	}
	if !containsSubstring(errStr, "SUPE****2345") && !containsSubstring(errStr, "****") {
		t.Errorf("Expected redacted token in error, got: %s", errStr)
	}
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || (len(s) > 0 && len(sub) > 0 && stringContains(s, sub)))
}

func stringContains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestSendMessage_ParseErrorFallback(t *testing.T) {
	var attempts int
	var receivedParseModes []string

	mockTransport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		attempts++
		var p SendMessagePayload
		_ = json.NewDecoder(req.Body).Decode(&p)
		receivedParseModes = append(receivedParseModes, p.ParseMode)

		if attempts == 1 {
			// First attempt fails with 400 can't parse entities
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"ok":false,"error_code":400,"description":"Bad Request: can't parse entities: Can't find end of the entity"}`)),
			}, nil
		}

		// Retry attempt succeeds
		msgJSON := `{"ok":true,"result":{"message_id":77,"chat":{"id":123456},"text":"test"}}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(msgJSON)),
		}, nil
	})

	httpClient := &http.Client{Transport: mockTransport}
	client := NewClient("TEST_TOKEN", WithBaseURL("https://api.telegram.org"), WithHTTPClient(httpClient))

	msg, err := client.SendMessage(context.Background(), 123456, "<b>unclosed tag", nil)
	if err != nil {
		t.Fatalf("expected fallback to succeed, got: %v", err)
	}
	if msg == nil || msg.MessageID != 77 {
		t.Fatalf("expected message id 77, got: %v", msg)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
	if receivedParseModes[0] != "HTML" {
		t.Errorf("expected first attempt HTML, got %q", receivedParseModes[0])
	}
	if receivedParseModes[1] != "" {
		t.Errorf("expected retry attempt empty parse_mode, got %q", receivedParseModes[1])
	}
}

func TestEditMessageText_ParseErrorFallback(t *testing.T) {
	var attempts int
	var receivedParseModes []string

	mockTransport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		attempts++
		var p EditMessageTextPayload
		_ = json.NewDecoder(req.Body).Decode(&p)
		receivedParseModes = append(receivedParseModes, p.ParseMode)

		if attempts == 1 {
			// First attempt fails with 400 can't parse entities
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"ok":false,"error_code":400,"description":"Bad Request: can't parse entities: Can't find end of the entity"}`)),
			}, nil
		}

		// Retry attempt succeeds
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"ok":true,"result":true}`)),
		}, nil
	})

	httpClient := &http.Client{Transport: mockTransport}
	client := NewClient("TEST_TOKEN", WithBaseURL("https://api.telegram.org"), WithHTTPClient(httpClient))

	err := client.EditMessageText(context.Background(), 123456, 77, "<b>unclosed tag", nil)
	if err != nil {
		t.Fatalf("expected fallback to succeed, got: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
	if receivedParseModes[0] != "HTML" {
		t.Errorf("expected first attempt HTML, got %q", receivedParseModes[0])
	}
	if receivedParseModes[1] != "" {
		t.Errorf("expected retry attempt empty parse_mode, got %q", receivedParseModes[1])
	}
}

func TestCleanReplyMarkup(t *testing.T) {
	if cleanReplyMarkup(nil) != nil {
		t.Error("expected untyped nil to remain nil")
	}

	var ikm *InlineKeyboardMarkup = nil
	if cleanReplyMarkup(ikm) != nil {
		t.Error("expected typed nil *InlineKeyboardMarkup to become untyped nil")
	}

	var rkm *ReplyKeyboardMarkup = nil
	if cleanReplyMarkup(rkm) != nil {
		t.Error("expected typed nil *ReplyKeyboardMarkup to become untyped nil")
	}

	var rkr *ReplyKeyboardRemove = nil
	if cleanReplyMarkup(rkr) != nil {
		t.Error("expected typed nil *ReplyKeyboardRemove to become untyped nil")
	}

	validIKM := &InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{}}
	if cleanReplyMarkup(validIKM) != validIKM {
		t.Error("expected valid *InlineKeyboardMarkup to remain untouched")
	}
}

func TestSendMessage_OmitsNilReplyMarkup(t *testing.T) {
	var capturedBody string
	mockTransport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(req.Body)
		capturedBody = string(body)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"ok":true,"result":{"message_id":1,"chat":{"id":123456}}}`)),
		}, nil
	})

	httpClient := &http.Client{Transport: mockTransport}
	client := NewClient("TEST_TOKEN", WithBaseURL("https://api.telegram.org"), WithHTTPClient(httpClient))

	// Pass nil reply markup
	_, err := client.SendMessage(context.Background(), 123456, "Hello", nil)
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	if strings.Contains(capturedBody, `"reply_markup":null`) || strings.Contains(capturedBody, `"reply_markup"`) {
		t.Errorf("Expected reply_markup to be omitted completely when nil, got: %s", capturedBody)
	}
}
