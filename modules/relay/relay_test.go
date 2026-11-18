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

func setupTestRelay(sender TelegramSender, pin string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	mod := New(sender, "123456", pin)
	mod.RegisterRoutes(r)
	return r
}

func TestRelay_PIN_Auth(t *testing.T) {
	sender := &mockSender{}
	r := setupTestRelay(sender, "1804")

	// 1. Missing PIN -> 401
	req, _ := http.NewRequest(http.MethodPost, "/api/relay/send", strings.NewReader(`{"message":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d", w.Code)
	}

	// 2. Wrong PIN -> 401
	req, _ = http.NewRequest(http.MethodPost, "/api/relay/send", strings.NewReader(`{"message":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Relay-PIN", "9999")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for wrong PIN, got %d", w.Code)
	}

	// 3. Valid PIN via Header -> 200
	req, _ = http.NewRequest(http.MethodPost, "/api/relay/send", strings.NewReader(`{"message":"hello from header"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Relay-PIN", "1804")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for valid header PIN, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(sender.sentText, "hello from header") {
		t.Errorf("expected sent text to contain 'hello from header', got %s", sender.sentText)
	}

	// 4. Valid PIN via Query param -> 200
	req, _ = http.NewRequest(http.MethodPost, "/api/relay/send?pin=1804", strings.NewReader(`{"message":"hello from query"}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for valid query PIN, got %d", w.Code)
	}

	// 5. Valid PIN via Bearer -> 200
	req, _ = http.NewRequest(http.MethodPost, "/api/relay/send", strings.NewReader(`{"message":"hello from bearer"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer 1804")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for valid Bearer PIN, got %d", w.Code)
	}
}

func TestRelay_FileUpload_PhotoAndDocument(t *testing.T) {
	sender := &mockSender{}
	r := setupTestRelay(sender, "1804")

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
	r := setupTestRelay(sender, "1804")

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
	r := setupTestRelay(sender, "1804")

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
	mod := New(sender, "123456", "1804")
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
