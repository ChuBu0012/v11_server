package ai_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"v11-server/shared/ai"
)

func TestProofreader_SuccessWithChanges(t *testing.T) {
	orig := "สวัสดีคับ วันนี้เซิฟเวอร์มีปันหา นิดหน่อย กำลังตรวจสอบ"
	suggested := "สวัสดีครับ วันนี้เซิร์ฟเวอร์มีปัญหานิดหน่อย กำลังตรวจสอบ"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ai.ChatCompletionResponse{
			Choices: []ai.ChatChoice{
				{
					Message: ai.ChatMessage{
						Role:    "assistant",
						Content: `{"suggested":"` + suggested + `","has_changes":true,"explanation":"แก้ไขคำผิด: คับ -> ครับ, เซิฟเวอร์ -> เซิร์ฟเวอร์, ปันหา -> ปัญหา"}`,
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := ai.NewClient("test-key", ai.WithBaseURL(server.URL))

	result, err := ai.ProofreadText(context.Background(), client, orig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.HasChanges {
		t.Fatal("expected HasChanges=true")
	}
	if result.Suggested != suggested {
		t.Errorf("expected suggested %q, got %q", suggested, result.Suggested)
	}
	if result.Original != orig {
		t.Errorf("expected original preserved, got %q", result.Original)
	}
	if result.Fallback {
		t.Fatal("expected fallback=false")
	}
	if !strings.Contains(result.Explanation, "แก้ไขคำผิด") {
		t.Errorf("expected explanation to mention corrections, got %q", result.Explanation)
	}
}

func TestProofreader_StrictPreservation_URLsAndCode(t *testing.T) {
	orig := "ตรวจสถานะที่ https://todo-fm-d.vercel.app/ ด่วน โดยใช้คำสั่ง `pm2 logs go-server --nostream` ครับ"
	// Model fixes spelling but preserves URL and code block character-for-character
	suggested := "ตรวจสอบสถานะที่ https://todo-fm-d.vercel.app/ ด่วน โดยใช้คำสั่ง `pm2 logs go-server --nostream` ครับ"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ai.ChatCompletionResponse{
			Choices: []ai.ChatChoice{
				{
					Message: ai.ChatMessage{
						Role:    "assistant",
						Content: `{"suggested":"` + suggested + `","has_changes":true,"explanation":"ปรับภาษาให้เป็นทางการ"}`,
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := ai.NewClient("test-key", ai.WithBaseURL(server.URL))

	result, err := ai.ProofreadText(context.Background(), client, orig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.HasChanges {
		t.Fatal("expected HasChanges=true")
	}
	if !strings.Contains(result.Suggested, "https://todo-fm-d.vercel.app/") {
		t.Errorf("URL was not preserved in suggested text: %s", result.Suggested)
	}
	if !strings.Contains(result.Suggested, "`pm2 logs go-server --nostream`") {
		t.Errorf("Code block was not preserved in suggested text: %s", result.Suggested)
	}
}

func TestProofreader_DefenseAgainstMangledURL(t *testing.T) {
	orig := "รายงานผลที่ https://spindle-said-hemstitch.ngrok-free.dev/health เรียบร้อย"
	// Malicious/bad AI response removes or alters URL
	badSuggested := "รายงานผลที่ลิงก์ปลายทางเรียบร้อย"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ai.ChatCompletionResponse{
			Choices: []ai.ChatChoice{
				{
					Message: ai.ChatMessage{
						Role:    "assistant",
						Content: `{"suggested":"` + badSuggested + `","has_changes":true,"explanation":"ย่อความ"}`,
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := ai.NewClient("test-key", ai.WithBaseURL(server.URL))

	result, err := ai.ProofreadText(context.Background(), client, orig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should trigger preservation defense fallback to original text!
	if !result.Fallback {
		t.Fatal("expected fallback=true when AI mangles URL")
	}
	if result.Suggested != orig {
		t.Errorf("expected suggested to fallback to original text, got %q", result.Suggested)
	}
	if result.HasChanges {
		t.Fatal("expected HasChanges=false on fallback")
	}
}

func TestProofreader_NoChangesNeeded(t *testing.T) {
	cleanText := "สวัสดีครับ ระบบทำงานได้ตามปกติ"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ai.ChatCompletionResponse{
			Choices: []ai.ChatChoice{
				{
					Message: ai.ChatMessage{
						Role:    "assistant",
						Content: `{"suggested":"` + cleanText + `","has_changes":false,"explanation":"ข้อความถูกต้องแล้ว"}`,
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := ai.NewClient("test-key", ai.WithBaseURL(server.URL))

	result, err := ai.ProofreadText(context.Background(), client, cleanText)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.HasChanges {
		t.Fatal("expected HasChanges=false for clean text")
	}
	if result.Suggested != cleanText {
		t.Errorf("expected suggested to match original, got %q", result.Suggested)
	}
}

func TestProofreader_EmptyText(t *testing.T) {
	client := ai.NewClient("test-key")

	result, err := ai.ProofreadText(context.Background(), client, "   ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.HasChanges {
		t.Fatal("expected HasChanges=false for empty text")
	}
}

func TestProofreader_AIError_Fallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := ai.NewClient("test-key", ai.WithBaseURL(server.URL))

	orig := "ข้อความสำคัญที่จะส่งผ่าน Telegram"
	result, err := ai.ProofreadText(context.Background(), client, orig)
	if err != nil {
		t.Fatalf("expected nil error on fallback, got %v", err)
	}

	if !result.Fallback {
		t.Fatal("expected fallback=true on rate limit error")
	}
	if result.Suggested != orig {
		t.Errorf("expected fallback to preserve original text, got %q", result.Suggested)
	}
	if result.HasChanges {
		t.Fatal("expected HasChanges=false on fallback")
	}
}
