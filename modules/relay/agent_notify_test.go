package relay

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFormatAgentTelegramCard(t *testing.T) {
	tests := []struct {
		name     string
		req      *AgentNotificationRequest
		contains []string
	}{
		{
			name: "success status with full metadata",
			req: &AgentNotificationRequest{
				Project:  "my-app",
				Status:   "success",
				Title:    "Build & Tests Completed",
				Summary:  "- Added OAuth authentication\n- Fixed 3 unit tests",
				Files:    []string{"auth.go", "auth_test.go"},
				Branch:   "feat/auth",
				Commit:   "a1b2c3d4e5f",
				Device:   "MacBook Air M4",
				Duration: "1m 30s",
			},
			contains: []string{
				"🤖 <b>Agent Notification</b> [<code>my-app</code>]",
				"สถานะ: 🟢 <b>สำเร็จ (Success)</b>",
				"📌 <b>หัวข้อ:</b> Build &amp; Tests Completed",
				"📝 <b>สรุปผล:</b>",
				"• Added OAuth authentication",
				"• Fixed 3 unit tests",
				"📂 <b>ไฟล์:</b> <code>auth.go</code>, <code>auth_test.go</code>",
				"🌿 <code>feat/auth</code>",
				"🏷️ <code>a1b2c3d</code>",
				"💻 MacBook Air M4",
				"⏱️ 1m 30s",
				"⏰ <b>เวลา:</b>",
			},
		},
		{
			name: "failed status with string files and html characters",
			req: &AgentNotificationRequest{
				Project: "backend <service>",
				Status:  "failed",
				Title:   "Integration Test Failed",
				Summary: "Error: unexpected nil pointer at main.go:42 & db.go",
				Files:   "server.go, client.go",
				Branch:  "main",
			},
			contains: []string{
				"[<code>backend &lt;service&gt;</code>]",
				"สถานะ: 🔴 <b>ล้มเหลว (Failed)</b>",
				"📌 <b>หัวข้อ:</b> Integration Test Failed",
				"Error: unexpected nil pointer at main.go:42 &amp; db.go",
				"<code>server.go</code>, <code>client.go</code>",
			},
		},
		{
			name: "warning status with json array string files",
			req: &AgentNotificationRequest{
				Project: "cli-tool",
				Status:  "warning",
				Title:   "Linter Warnings",
				Files:   `["pkg/a.go", "pkg/b.go"]`,
			},
			contains: []string{
				"สถานะ: 🟡 <b>แจ้งเตือน / ต้องตรวจทาน (Warning)</b>",
				"<code>pkg/a.go</code>, <code>pkg/b.go</code>",
			},
		},
		{
			name: "minimal payload defaults to General",
			req:  &AgentNotificationRequest{},
			contains: []string{
				"[<code>General</code>]",
				"สถานะ: 🔵 <b>แจ้งเตือน (Info)</b>",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatAgentTelegramCard(tt.req)
			for _, exp := range tt.contains {
				if !strings.Contains(got, exp) {
					t.Errorf("FormatAgentTelegramCard() missing expected substring %q in output:\n%s", exp, got)
				}
			}
		})
	}
}

func TestHandleAgentNotify_JSON(t *testing.T) {
	mock := &mockSender{}
	router := setupTestRelay(mock)

	payload := AgentNotificationRequest{
		Project: "v11_server",
		Status:  "success",
		Title:   "Task Done",
		Summary: "Everything is green",
	}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest(http.MethodPost, "/api/agent/notify", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	if !strings.Contains(mock.sentText, "v11_server") {
		t.Errorf("Expected sentText to contain project, got %q", mock.sentText)
	}
	if !strings.Contains(mock.sentText, "Task Done") {
		t.Errorf("Expected sentText to contain title, got %q", mock.sentText)
	}
}

func TestHandleAgentNotify_MultipartWithAttachment(t *testing.T) {
	mock := &mockSender{}
	router := setupTestRelay(mock)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("project", "v11_server")
	_ = writer.WriteField("status", "success")
	_ = writer.WriteField("title", "Screenshot captured")

	part, err := writer.CreateFormFile("photo", "chart.png")
	if err != nil {
		t.Fatalf("Failed to create form file: %v", err)
	}
	part.Write([]byte("fake-png-content"))
	_ = writer.Close()

	req, _ := http.NewRequest(http.MethodPost, "/api/agent/notify", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	if mock.sentPhotoName != "chart.png" {
		t.Errorf("Expected sentPhotoName 'chart.png', got %q", mock.sentPhotoName)
	}
	if !strings.Contains(mock.sentCaption, "Screenshot captured") {
		t.Errorf("Expected sentCaption to contain title, got %q", mock.sentCaption)
	}
}
