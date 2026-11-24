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

type mockWorkspaceBotService struct {
	notes map[string]*BotNoteItem
}

func newMockWorkspaceBotService() *mockWorkspaceBotService {
	return &mockWorkspaceBotService{
		notes: make(map[string]*BotNoteItem),
	}
}

func (m *mockWorkspaceBotService) CreateNote(ctx context.Context, text string) (*BotNoteItem, error) {
	lines := strings.Split(text, "\n")
	title := strings.TrimSpace(lines[0])
	content := text
	id := "nt_mock1"

	item := &BotNoteItem{
		ID:        id,
		Title:     title,
		Content:   content,
		Tags:      []string{"todo"},
		IsPinned:  false,
		UpdatedAt: time.Now(),
	}
	m.notes[id] = item
	return item, nil
}

func (m *mockWorkspaceBotService) ListNotes(ctx context.Context, tag string, limit int) ([]BotNoteItem, error) {
	var res []BotNoteItem
	for _, n := range m.notes {
		res = append(res, *n)
	}
	return res, nil
}

func (m *mockWorkspaceBotService) GetNote(ctx context.Context, id string) (*BotNoteItem, error) {
	n, ok := m.notes[id]
	if !ok {
		return nil, ErrSecurityViolation
	}
	return n, nil
}

func (m *mockWorkspaceBotService) DeleteNote(ctx context.Context, id string) error {
	delete(m.notes, id)
	return nil
}

func (m *mockWorkspaceBotService) SetNotePinned(ctx context.Context, id string, pinned bool) error {
	if n, ok := m.notes[id]; ok {
		n.IsPinned = pinned
	}
	return nil
}

func TestRouter_NoteCommands(t *testing.T) {
	mockSvc := newMockWorkspaceBotService()

	var mu sync.Mutex
	var lastSentMessage string

	mockTransport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		mu.Lock()
		defer mu.Unlock()

		if strings.Contains(r.URL.Path, "sendMessage") {
			var payload SendMessagePayload
			_ = json.NewDecoder(r.Body).Decode(&payload)
			lastSentMessage = payload.Text

			resp := APIResponse{
				OK: true,
				Result: json.RawMessage(`{
					"message_id": 100,
					"date": 1600000000,
					"chat": {"id": 12345, "type": "private"},
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
		if strings.Contains(r.URL.Path, "answerCallbackQuery") || strings.Contains(r.URL.Path, "editMessageText") {
			resp := APIResponse{OK: true, Result: json.RawMessage(`true`)}
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
	auth := NewAuthorizer("12345", nil)

	router := NewRouter(client, auth, nil, nil, nil, WithWorkspaceService(mockSvc))
	ctx := context.Background()

	// 1. Send /note empty
	err := router.HandleUpdate(ctx, Update{
		Message: &Message{
			Text: "/note",
			Chat: &Chat{ID: 12345},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error for /note empty: %v", err)
	}
	mu.Lock()
	if !strings.Contains(lastSentMessage, "วิธีใช้คำสั่ง /note") {
		t.Errorf("expected usage instructions, got %q", lastSentMessage)
	}
	mu.Unlock()

	// 2. Send /note with text
	err = router.HandleUpdate(ctx, Update{
		Message: &Message{
			Text: "/note รายการซื้อของ: นมสด #todo",
			Chat: &Chat{ID: 12345},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error for /note: %v", err)
	}
	mu.Lock()
	if !strings.Contains(lastSentMessage, "บันทึกโน้ตสำเร็จ") {
		t.Errorf("expected success message, got: %s", lastSentMessage)
	}
	mu.Unlock()

	// 3. Send /notes
	err = router.HandleUpdate(ctx, Update{
		Message: &Message{
			Text: "/notes",
			Chat: &Chat{ID: 12345},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error for /notes: %v", err)
	}
	mu.Lock()
	if !strings.Contains(lastSentMessage, "บันทึกโน้ตล่าสุด") {
		t.Errorf("expected notes list, got: %s", lastSentMessage)
	}
	mu.Unlock()

	// 4. Test Callback Query: wn:del
	cbErr := router.HandleUpdate(ctx, Update{
		CallbackQuery: &CallbackQuery{
			ID:   "cb1",
			Data: "wn:del:nt_mock1",
			From: &User{ID: 12345},
			Message: &Message{
				MessageID: 100,
				Chat:      &Chat{ID: 12345},
			},
		},
	})
	if cbErr != nil {
		t.Fatalf("unexpected error for wn:del callback: %v", cbErr)
	}
	if len(mockSvc.notes) != 0 {
		t.Errorf("expected note to be deleted from mock store, but got %d notes", len(mockSvc.notes))
	}
}
