package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"v11-server/modules/bot"
)

type mockSender struct {
	sentText      string
	sentPhotoName string
	sentDocName   string
	sentCaption   string
	shouldError   bool
	photoFails    bool
}

func (m *mockSender) SendMessage(ctx context.Context, chatID int64, text string, replyMarkup *bot.InlineKeyboardMarkup) (*bot.Message, error) {
	if m.shouldError {
		return nil, fmt.Errorf("mock error")
	}
	m.sentText = text
	return &bot.Message{MessageID: 100}, nil
}

func (m *mockSender) SendPhoto(ctx context.Context, chatID int64, filename string, r io.Reader, caption string) (*bot.Message, error) {
	if m.shouldError {
		return nil, fmt.Errorf("mock photo error")
	}
	if m.photoFails {
		return nil, fmt.Errorf("telegram API error (400): Bad Request: PHOTO_SAVE_FILE_INVALID")
	}
	m.sentPhotoName = filename
	m.sentCaption = caption
	return &bot.Message{MessageID: 101}, nil
}

func (m *mockSender) SendDocument(ctx context.Context, chatID int64, filename string, r io.Reader, caption string) (*bot.Message, error) {
	if m.shouldError {
		return nil, fmt.Errorf("mock doc error")
	}
	m.sentDocName = filename
	m.sentCaption = caption
	return &bot.Message{MessageID: 102}, nil
}

func setupTestRelay(sender TelegramSender) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	mod := New(sender, "123456")
	mod.RegisterRoutes(r)
	return r
}

func TestRelay_SendNeedsNoPIN(t *testing.T) {
	sender := &mockSender{}
	r := setupTestRelay(sender)

	// The relay is public: no PIN header, and a stale PIN header, both succeed.
	for _, stale := range []string{"", "9999"} {
		req, _ := http.NewRequest(http.MethodPost, "/api/relay/send", strings.NewReader(`{"message":"hello public"}`))
		req.Header.Set("Content-Type", "application/json")
		if stale != "" {
			req.Header.Set("X-Relay-PIN", stale)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("pin=%q: expected 200, got %d: %s", stale, w.Code, w.Body.String())
		}
	}
	if !strings.Contains(sender.sentText, "hello public") {
		t.Errorf("expected sent text to contain 'hello public', got %s", sender.sentText)
	}
}

func TestRelay_FileUpload_PhotoAndDocument(t *testing.T) {
	sender := &mockSender{}
	r := setupTestRelay(sender)

	// 1. Upload Photo (.png)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("caption", "My cool photo")
	part, _ := writer.CreateFormFile("file", "test_screen.png")
	_, _ = part.Write([]byte("fake png binary data"))
	_ = writer.Close()

	req, _ := http.NewRequest(http.MethodPost, "/api/relay/send", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Relay-PIN", "1804")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("photo upload expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if sender.sentPhotoName != "test_screen.png" {
		t.Errorf("expected photo name 'test_screen.png', got %s", sender.sentPhotoName)
	}
	if sender.sentCaption != "My cool photo" {
		t.Errorf("expected caption 'My cool photo', got %s", sender.sentCaption)
	}

	// 2. Upload Document (.pdf)
	body.Reset()
	writer = multipart.NewWriter(&body)
	_ = writer.WriteField("caption", "Annual report")
	part, _ = writer.CreateFormFile("file", "report.pdf")
	_, _ = part.Write([]byte("%PDF dummy content"))
	_ = writer.Close()

	req, _ = http.NewRequest(http.MethodPost, "/api/relay/send", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Relay-PIN", "1804")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("doc upload expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if sender.sentDocName != "report.pdf" {
		t.Errorf("expected doc name 'report.pdf', got %s", sender.sentDocName)
	}
}

func TestRelay_Status(t *testing.T) {
	sender := &mockSender{}
	r := setupTestRelay(sender)

	req, _ := http.NewRequest(http.MethodGet, "/api/relay/status", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status expected 200, got %d", w.Code)
	}

	var res struct {
		Service    string `json:"service"`
		Configured bool   `json:"configured"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &res)
	if res.Service != "pocket-sentinel-relay" || !res.Configured {
		t.Errorf("unexpected status response: %+v", res)
	}
}

func TestRelay_PhotoFallbackToDocument(t *testing.T) {
	sender := &mockSender{photoFails: true} // Simulate Telegram rejecting SendPhoto (e.g. dimensions > 10,000 px)
	r := setupTestRelay(sender)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("caption", "High-res Poster A2")
	part, _ := writer.CreateFormFile("file", "Final_Poster_A2.png")
	_, _ = part.Write([]byte("fake large poster png bytes"))
	_ = writer.Close()

	req, _ := http.NewRequest(http.MethodPost, "/api/relay/send", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Relay-PIN", "1804")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK after fallback, got %d: %s", w.Code, w.Body.String())
	}
	// Verify that SendDocument received the file after SendPhoto failed
	if sender.sentDocName != "Final_Poster_A2.png" {
		t.Errorf("expected doc name 'Final_Poster_A2.png' after fallback, got '%s'", sender.sentDocName)
	}
	if sender.sentCaption != "High-res Poster A2" {
		t.Errorf("expected caption 'High-res Poster A2', got '%s'", sender.sentCaption)
	}
}

func TestRelay_UploadLimit(t *testing.T) {
	sender := &mockSender{}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	mod := New(sender, "123456")
	mod.maxUploadBytes = 100 // Set tiny limit: 100 bytes
	mod.RegisterRoutes(r)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("file", "large.txt")
	_, _ = part.Write(bytes.Repeat([]byte("A"), 500)) // 500 bytes > 100 bytes
	_ = writer.Close()

	req, _ := http.NewRequest(http.MethodPost, "/api/relay/send", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Relay-PIN", "1804")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 Request Entity Too Large, got %d: %s", w.Code, w.Body.String())
	}
}
