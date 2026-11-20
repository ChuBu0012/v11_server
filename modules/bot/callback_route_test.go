package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRouter_CallbackRoutePrefix(t *testing.T) {
	mockTransport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"ok":true,"result":{}}`))),
		}, nil
	})
	httpClient := &http.Client{Transport: mockTransport}
	client := NewClient("TEST_TOKEN", WithBaseURL("https://api.telegram.org"), WithHTTPClient(httpClient))
	auth := NewAuthorizer("123456", nil)

	var dgCalled string
	var qsCalled string

	router := NewRouter(
		client,
		auth,
		nil,
		nil,
		&mockStatusProvider{},
		WithCallbackRoute("dg:", func(ctx context.Context, q *CallbackQuery) error {
			dgCalled = q.Data
			return nil
		}),
		WithCallbackHandler(func(ctx context.Context, q *CallbackQuery) error {
			qsCalled = q.Data
			return nil
		}),
	)

	ctx := context.Background()

	// 1. "dg:" prefix callback
	err := router.HandleUpdate(ctx, Update{
		UpdateID: 1,
		CallbackQuery: &CallbackQuery{
			ID:   "cb1",
			From: &User{ID: 123456},
			Data: "dg:42:gold",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dgCalled != "dg:42:gold" {
		t.Fatalf("dgCalled = %q, want %q", dgCalled, "dg:42:gold")
	}
	if qsCalled != "" {
		t.Fatalf("qsCalled should be empty, got %q", qsCalled)
	}

	// 2. "qs:" prefix callback
	err = router.HandleUpdate(ctx, Update{
		UpdateID: 2,
		CallbackQuery: &CallbackQuery{
			ID:   "cb2",
			From: &User{ID: 123456},
			Data: "qs:delete:99",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if qsCalled != "qs:delete:99" {
		t.Fatalf("qsCalled = %q, want %q", qsCalled, "qs:delete:99")
	}
}

func TestRouter_DigestNoDuplicate(t *testing.T) {
	var sentMessages []string
	var mu sync.Mutex

	mockTransport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		mu.Lock()
		defer mu.Unlock()

		if strings.Contains(r.URL.Path, "sendMessage") {
			var payload SendMessagePayload
			_ = json.NewDecoder(r.Body).Decode(&payload)
			sentMessages = append(sentMessages, payload.Text)

			resp := APIResponse{
				OK: true,
				Result: json.RawMessage(`{
					"message_id": 99,
					"date": 1600000000,
					"chat": {"id": 123456, "type": "private"},
					"text": "sent"
				}`),
			}
			bodyBytes, _ := json.Marshal(resp)
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
		}, nil
	})

	httpClient := &http.Client{Transport: mockTransport}
	client := NewClient("TEST_TOKEN", WithBaseURL("https://api.telegram.org"), WithHTTPClient(httpClient))
	auth := NewAuthorizer("123456", nil)

	// When digestFunc returns "", router should NOT send a second message
	router := NewRouter(
		client,
		auth,
		nil,
		nil,
		&mockStatusProvider{},
		WithDigestFunc(func(ctx context.Context) (string, error) {
			return "", nil
		}),
	)

	ctx := context.Background()
	err := router.HandleUpdate(ctx, Update{
		UpdateID: 1,
		Message: &Message{
			MessageID: 10,
			From:      &User{ID: 123456},
			Chat:      &Chat{ID: 123456},
			Text:      "/digest",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	// Only 1 message sent: the ACK message
	if len(sentMessages) != 1 {
		t.Fatalf("sent %d messages, want exactly 1: %v", len(sentMessages), sentMessages)
	}
	if !strings.Contains(sentMessages[0], "กำลังดึงข่าวสด") {
		t.Fatalf("expected ACK message, got: %s", sentMessages[0])
	}
}
