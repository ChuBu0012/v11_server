package bot

import (
	"bytes"
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
)

func TestClient_SendPhoto_Success(t *testing.T) {
	var capturedContentType string
	var capturedChatID string
	var capturedCaption string
	var capturedFilename string
	var capturedPhotoBytes []byte

	mockTransport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if !strings.Contains(r.URL.Path, "sendPhoto") {
			t.Fatalf("expected path to contain sendPhoto, got %s", r.URL.Path)
		}
		capturedContentType = r.Header.Get("Content-Type")

		mediaType, params, err := mime.ParseMediaType(capturedContentType)
		if err != nil || !strings.HasPrefix(mediaType, "multipart/") {
			t.Fatalf("invalid media type: %v", err)
		}

		mr := multipart.NewReader(r.Body, params["boundary"])
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("reading part failed: %v", err)
			}
			data, _ := io.ReadAll(part)
			switch part.FormName() {
			case "chat_id":
				capturedChatID = string(data)
			case "caption":
				capturedCaption = string(data)
			case "photo":
				capturedFilename = part.FileName()
				capturedPhotoBytes = data
			}
		}

		respBody := `{"ok":true,"result":{"message_id":777,"date":1600000000,"chat":{"id":123456,"type":"private"}}}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(respBody)),
		}, nil
	})

	httpClient := &http.Client{Transport: mockTransport}
	client := NewClient("TEST_TOKEN", WithBaseURL("https://api.telegram.org"), WithHTTPClient(httpClient))

	photoContent := []byte("FAKE_IMAGE_DATA_12345")
	msg, err := client.SendPhoto(context.Background(), 123456, "screenshot.png", bytes.NewReader(photoContent), "Look at this")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if msg.MessageID != 777 {
		t.Errorf("expected message_id 777, got %d", msg.MessageID)
	}
	if capturedChatID != "123456" {
		t.Errorf("expected chat_id 123456, got %s", capturedChatID)
	}
	if capturedCaption != "Look at this" {
		t.Errorf("expected caption 'Look at this', got %s", capturedCaption)
	}
	if capturedFilename != "screenshot.png" {
		t.Errorf("expected filename 'screenshot.png', got %s", capturedFilename)
	}
	if string(capturedPhotoBytes) != string(photoContent) {
		t.Errorf("expected photo bytes %q, got %q", photoContent, capturedPhotoBytes)
	}
}

func TestClient_SendDocument_Success(t *testing.T) {
	var capturedChatID string
	var capturedCaption string
	var capturedFilename string
	var capturedDocBytes []byte

	mockTransport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if !strings.Contains(r.URL.Path, "sendDocument") {
			t.Fatalf("expected path to contain sendDocument, got %s", r.URL.Path)
		}

		_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			t.Fatalf("invalid media type: %v", err)
		}

		mr := multipart.NewReader(r.Body, params["boundary"])
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("reading part failed: %v", err)
			}
			data, _ := io.ReadAll(part)
			switch part.FormName() {
			case "chat_id":
				capturedChatID = string(data)
			case "caption":
				capturedCaption = string(data)
			case "document":
				capturedFilename = part.FileName()
				capturedDocBytes = data
			}
		}

		respBody := `{"ok":true,"result":{"message_id":888,"date":1600000000,"chat":{"id":123456,"type":"private"}}}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(respBody)),
		}, nil
	})

	httpClient := &http.Client{Transport: mockTransport}
	client := NewClient("TEST_TOKEN", WithBaseURL("https://api.telegram.org"), WithHTTPClient(httpClient))

	docContent := []byte("%PDF-1.4 sample pdf content")
	msg, err := client.SendDocument(context.Background(), 123456, "report.pdf", bytes.NewReader(docContent), "Quarterly Report")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if msg.MessageID != 888 {
		t.Errorf("expected message_id 888, got %d", msg.MessageID)
	}
	if capturedChatID != "123456" {
		t.Errorf("expected chat_id 123456, got %s", capturedChatID)
	}
	if capturedCaption != "Quarterly Report" {
		t.Errorf("expected caption 'Quarterly Report', got %s", capturedCaption)
	}
	if capturedFilename != "report.pdf" {
		t.Errorf("expected filename 'report.pdf', got %s", capturedFilename)
	}
	if string(capturedDocBytes) != string(docContent) {
		t.Errorf("expected doc bytes %q, got %q", docContent, capturedDocBytes)
	}
}
