package ai_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"v11-server/shared/ai"
)

func TestScheduleParser_ClearPrompt_Thai(t *testing.T) {
	mockResponse := ai.ScheduleAIResponse{
		NeedsClarification: false,
		Preview: &ai.SchedulePreview{
			Title:      "ประชุมทีม",
			Details:    "ประชุมทีมประจำสัปดาห์",
			StartsAt:   "2026-11-20T09:00:00+07:00",
			Recurrence: "weekly",
			Offsets:    []int{0, 5},
		},
	}
	respBytes, _ := json.Marshal(mockResponse)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chatResp := ai.ChatCompletionResponse{
			Choices: []ai.ChatChoice{
				{
					Message: ai.ChatMessage{
						Role:    "assistant",
						Content: string(respBytes),
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(chatResp)
	}))
	defer server.Close()

	client := ai.NewClient("test-key", ai.WithBaseURL(server.URL))

	result, err := ai.ParseSchedulePrompt(context.Background(), client, "พรุ่งนี้ 09:00 น. ประชุมทีม", "Asia/Bangkok")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.NeedsClarification {
		t.Fatal("expected needs_clarification=false for clear time")
	}
	if result.Fallback {
		t.Fatal("expected fallback=false for successful parse")
	}
	if result.Preview == nil {
		t.Fatal("expected preview object to be non-nil")
	}
	if result.Preview.Title != "ประชุมทีม" {
		t.Errorf("expected title 'ประชุมทีม', got %q", result.Preview.Title)
	}
	if result.Preview.Recurrence != "weekly" {
		t.Errorf("expected recurrence 'weekly', got %q", result.Preview.Recurrence)
	}
	if !reflect.DeepEqual(result.Preview.Offsets, []int{0, 5}) {
		t.Errorf("expected offsets [0, 5], got %v", result.Preview.Offsets)
	}

	// Verify Bangkok timezone
	parsed, err := time.Parse(time.RFC3339, result.Preview.StartsAt)
	if err != nil {
		t.Fatalf("invalid starts_at RFC3339: %v", err)
	}
	_, offset := parsed.Zone()
	if offset != 7*60*60 {
		t.Errorf("expected starts_at timezone offset +07:00 (25200s), got %d", offset)
	}
}

func TestScheduleParser_ClearPrompt_English(t *testing.T) {
	mockResponse := ai.ScheduleAIResponse{
		NeedsClarification: false,
		Preview: &ai.SchedulePreview{
			Title:      "Dentist Appointment",
			Details:    "Routine dental checkup",
			StartsAt:   "2026-11-20T14:30:00+07:00",
			Recurrence: "once",
			Offsets:    []int{0, 10, 30},
		},
	}
	respBytes, _ := json.Marshal(mockResponse)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chatResp := ai.ChatCompletionResponse{
			Choices: []ai.ChatChoice{
				{
					Message: ai.ChatMessage{
						Role:    "assistant",
						Content: "```json\n" + string(respBytes) + "\n```", // wrapped in markdown code fence
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(chatResp)
	}))
	defer server.Close()

	client := ai.NewClient("test-key", ai.WithBaseURL(server.URL))

	result, err := ai.ParseSchedulePrompt(context.Background(), client, "Tomorrow at 14:30 Dentist Appointment", "Asia/Bangkok")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.NeedsClarification {
		t.Fatal("expected needs_clarification=false")
	}
	if result.Preview.Title != "Dentist Appointment" {
		t.Errorf("expected title 'Dentist Appointment', got %q", result.Preview.Title)
	}
	if !reflect.DeepEqual(result.Preview.Offsets, []int{0, 10, 30}) {
		t.Errorf("expected offsets [0, 10, 30], got %v", result.Preview.Offsets)
	}
}

func TestScheduleParser_AmbiguousPrompts(t *testing.T) {
	ambiguousCases := []struct {
		name   string
		prompt string
	}{
		{"Thai vague morning", "พรุ่งนี้ตอนสายๆ ปลุกด้วย"},
		{"Thai vague afternoon", "บ่ายๆ โทรหาหมอ"},
		{"Thai vague evening", "ตอนเย็นๆ แวะซื้อของ"},
		{"Thai no date or time", "เตือนกินยาด้วย"},
		{"Thai vague future", "สัปดาห์หน้าว่างๆ นัดเจอกัน"},
		{"English vague future", "meeting sometime next week"},
		{"English no time", "remember to buy groceries"},
	}

	client := ai.NewClient("test-key")

	for _, tc := range ambiguousCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ai.ParseSchedulePrompt(context.Background(), client, tc.prompt, "Asia/Bangkok")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !result.NeedsClarification {
				t.Errorf("prompt %q should require clarification (NeedsClarification=true)", tc.prompt)
			}
			if result.Question == "" {
				t.Errorf("prompt %q should provide a clarification question", tc.prompt)
			}
			if result.Preview != nil {
				t.Errorf("prompt %q should NOT guess or hallucinate a Preview timestamp", tc.prompt)
			}
		})
	}
}

func TestScheduleParser_AIError_Fallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer server.Close()

	client := ai.NewClient("test-key", ai.WithBaseURL(server.URL))

	prompt := "พรุ่งนี้ 10:00 น. ซ่อมเซิร์ฟเวอร์ด่วน"
	result, err := ai.ParseSchedulePrompt(context.Background(), client, prompt, "Asia/Bangkok")
	if err != nil {
		t.Fatalf("expected nil error on fallback, got: %v", err)
	}

	if !result.Fallback {
		t.Fatal("expected fallback=true when AI call fails")
	}
	if result.NeedsClarification {
		t.Fatal("expected needs_clarification=false on fallback")
	}
	if result.Preview == nil {
		t.Fatal("expected fallback preview to be populated")
	}
	if result.Preview.Title != prompt {
		t.Errorf("expected fallback title to preserve prompt, got %q", result.Preview.Title)
	}
	if result.Preview.Details != prompt {
		t.Errorf("expected fallback details to preserve prompt, got %q", result.Preview.Details)
	}
	if result.Preview.Recurrence != "once" {
		t.Errorf("expected fallback recurrence 'once', got %q", result.Preview.Recurrence)
	}
	if !reflect.DeepEqual(result.Preview.Offsets, []int{0}) {
		t.Errorf("expected fallback offsets [0], got %v", result.Preview.Offsets)
	}
}

func TestScheduleParser_NoAPIKey_Fallback(t *testing.T) {
	client := ai.NewClient("") // No API key configured

	prompt := "พรุ่งนี้ 10:00 น. ทดสอบระบบ"
	result, err := ai.ParseSchedulePrompt(context.Background(), client, prompt, "Asia/Bangkok")
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}

	if !result.Fallback {
		t.Fatal("expected fallback=true when no API key")
	}
	if result.Preview == nil || result.Preview.Title != prompt {
		t.Errorf("expected fallback preview to preserve prompt")
	}
}

func TestScheduleParser_NormalizeOffsets(t *testing.T) {
	tests := []struct {
		input    []int
		expected []int
	}{
		{nil, []int{0}},
		{[]int{}, []int{0}},
		{[]int{0}, []int{0}},
		{[]int{30, 5, 0, 10}, []int{0, 5, 10, 30}},
		{[]int{5, 5, 10, 10, 0}, []int{0, 5, 10}},
		{[]int{99, 100, -5}, []int{0}}, // non-standard offsets discarded, defaults to [0]
		{[]int{30, 99}, []int{30}},
	}

	for _, tc := range tests {
		got := ai.NormalizeOffsets(tc.input)
		if !reflect.DeepEqual(got, tc.expected) {
			t.Errorf("NormalizeOffsets(%v) = %v; expected %v", tc.input, got, tc.expected)
		}
	}
}

func TestScheduleParser_TrimRunes(t *testing.T) {
	thaiText := "สวัสดีครับทุกท่าน วันนี้มาทดสอบฟังก์ชันการตัดข้อความตามจำนวนตัวอักษร"
	runes := []rune(thaiText)

	trimmed := ai.TrimRunes(thaiText, 10)
	if len([]rune(trimmed)) != 10 {
		t.Errorf("expected 10 runes, got %d", len([]rune(trimmed)))
	}
	if trimmed != string(runes[:10]) {
		t.Errorf("rune trimming mismatch")
	}
}
