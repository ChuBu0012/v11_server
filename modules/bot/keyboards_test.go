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
)

func TestKeyboards(t *testing.T) {
	// 1. Check layout and buttons
	kb := MakeStatusKeyboard()
	if len(kb.InlineKeyboard) != 2 {
		t.Fatalf("Expected 2 rows of inline buttons, got %d", len(kb.InlineKeyboard))
	}
	if len(kb.InlineKeyboard[0]) != 1 || kb.InlineKeyboard[0][0].CallbackData != "status_refresh" {
		t.Errorf("Row 0 expected [status_refresh], got %+v", kb.InlineKeyboard[0])
	}
	if len(kb.InlineKeyboard[1]) != 2 {
		t.Fatalf("Row 1 expected 2 buttons, got %d", len(kb.InlineKeyboard[1]))
	}
	if kb.InlineKeyboard[1][0].CallbackData != "torch_toggle" || kb.InlineKeyboard[1][1].CallbackData != "ping_check" {
		t.Errorf("Row 1 expected [torch_toggle, ping_check], got %+v", kb.InlineKeyboard[1])
	}

	// 2. Test CallbackQuery handling
	var mu sync.Mutex
	var lastAnswerQueryText string
	var lastEditedText string
	var editCount int

	mockTransport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		mu.Lock()
		defer mu.Unlock()

		if strings.Contains(r.URL.Path, "answerCallbackQuery") {
			var payload AnswerCallbackQueryPayload
			_ = json.NewDecoder(r.Body).Decode(&payload)
			lastAnswerQueryText = payload.Text
			resp := APIResponse{OK: true, Result: json.RawMessage("true")}
			bodyBytes, _ := json.Marshal(resp)
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
			}, nil
		}
		if strings.Contains(r.URL.Path, "editMessageText") {
			var payload EditMessageTextPayload
			_ = json.NewDecoder(r.Body).Decode(&payload)
			lastEditedText = payload.Text
			editCount++
			resp := APIResponse{OK: true, Result: json.RawMessage("true")}
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
	client := NewClient("TEST_TOKEN", WithBaseURL("https://api.telegram.org"), WithHTTPClient(httpClient))
	statusProv := &mockStatusProvider{
		statusCard: "📊 Updated Status Card",
		uptimeStr:  "1h 15m",
	}
	mockRunner := &mockTorchRunner{}
	torch := NewTorchController(WithCommandRunner(mockRunner.Run))

	kbMod := NewKeyboardsModule(client, statusProv, torch)
	ctx := context.Background()

	// Case 1: status_refresh
	refreshQuery := &CallbackQuery{
		ID:   "query_1",
		From: &User{ID: 123456, Username: "owner"},
		Message: &Message{
			MessageID: 55,
			Chat:      &Chat{ID: 123456},
		},
		Data: "status_refresh",
	}
	err := kbMod.HandleCallbackQuery(ctx, refreshQuery)
	if err != nil {
		t.Fatalf("Handle status_refresh failed: %v", err)
	}
	mu.Lock()
	if !strings.Contains(lastAnswerQueryText, "Updating status") {
		t.Errorf("Expected answer query 'Updating status', got: %s", lastAnswerQueryText)
	}
	if !strings.Contains(lastEditedText, "Updated Status Card") {
		t.Errorf("Expected edited message text, got: %s", lastEditedText)
	}
	mu.Unlock()

	// Case 2: torch_toggle
	toggleQuery := &CallbackQuery{
		ID:   "query_2",
		From: &User{ID: 123456, Username: "owner"},
		Message: &Message{
			MessageID: 55,
			Chat:      &Chat{ID: 123456},
		},
		Data: "torch_toggle",
	}
	err = kbMod.HandleCallbackQuery(ctx, toggleQuery)
	if err != nil {
		t.Fatalf("Handle torch_toggle failed: %v", err)
	}
	if !torch.IsOn() {
		t.Error("Expected torch to be ON after toggle")
	}
	mu.Lock()
	if !strings.Contains(lastAnswerQueryText, "Torch turned ON") {
		t.Errorf("Expected toast 'Torch turned ON', got: %s", lastAnswerQueryText)
	}
	mu.Unlock()

	// Case 3: ping_check
	pingQuery := &CallbackQuery{
		ID:   "query_3",
		From: &User{ID: 123456, Username: "owner"},
		Message: &Message{
			MessageID: 55,
			Chat:      &Chat{ID: 123456},
		},
		Data: "ping_check",
	}
	err = kbMod.HandleCallbackQuery(ctx, pingQuery)
	if err != nil {
		t.Fatalf("Handle ping_check failed: %v", err)
	}
	mu.Lock()
	if !strings.Contains(lastAnswerQueryText, "Pong!") || !strings.Contains(lastAnswerQueryText, "1h 15m") {
		t.Errorf("Expected ping answer with uptime, got: %s", lastAnswerQueryText)
	}
	mu.Unlock()
}
