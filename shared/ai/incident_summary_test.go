package ai_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"v11-server/shared/ai"
)

func TestIncidentSummary_SuccessWithSeparation(t *testing.T) {
	mockFacts := []string{
		"บริการ Todo FM (https://todo-fm-d.vercel.app/) ล้มเหลวต่อเนื่อง",
		"ได้รับ HTTP Status 503 (คาดหวัง 200)",
		"ความล่าช้า (Latency): 145 ms",
	}
	mockHypotheses := []string{
		"Vercel serverless function อาจประสบปัญหา internal crash หรือ backend timeout",
		"ควรตรวจสอบ deployment logs และสถานะ edge function บน Vercel Dashboard",
	}

	var capturedUserPrompt string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ai.ChatCompletionRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		for _, m := range req.Messages {
			if m.Role == "user" {
				capturedUserPrompt = m.Content
			}
		}

		resp := ai.ChatCompletionResponse{
			Choices: []ai.ChatChoice{
				{
					Message: ai.ChatMessage{
						Role: "assistant",
						Content: `{
							"facts": ["` + strings.Join(mockFacts, `","`) + `"],
							"hypotheses": ["` + strings.Join(mockHypotheses, `","`) + `"]
						}`,
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := ai.NewClient("test-key", ai.WithBaseURL(server.URL))

	incident := ai.IncidentMetadata{
		Timestamp:      time.Date(2026, 11, 19, 2, 30, 0, 0, time.UTC),
		WatcherName:    "Todo FM",
		URL:            "https://todo-fm-d.vercel.app/dashboard?auth_token=super_secret_123#view",
		ExpectedStatus: 200,
		ActualStatus:   503,
		LatencyMs:      145,
		ErrorSummary:   "HTTP 503 Service Unavailable with Bearer secret-token-xyz",
	}

	result, err := ai.SummarizeIncident(context.Background(), client, incident)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Fallback {
		t.Fatal("expected fallback=false on successful summary")
	}

	// Verify Facts separation
	if len(result.Facts) != len(mockFacts) {
		t.Errorf("expected %d facts, got %d", len(mockFacts), len(result.Facts))
	}

	// Verify Hypotheses separation
	if len(result.Hypotheses) != len(mockHypotheses) {
		t.Errorf("expected %d hypotheses, got %d", len(mockHypotheses), len(result.Hypotheses))
	}

	// Verify RawSummary contains sections
	if !strings.Contains(result.RawSummary, "ข้อเท็จจริง (Facts)") {
		t.Errorf("expected RawSummary to contain Facts section header: %s", result.RawSummary)
	}
	if !strings.Contains(result.RawSummary, "ข้อสันนิษฐาน & สิ่งที่ควรตรวจต่อ (Hypotheses)") {
		t.Errorf("expected RawSummary to contain Hypotheses section header: %s", result.RawSummary)
	}

	// Verify that prompt sent to AI had URL and Error sanitized!
	if strings.Contains(capturedUserPrompt, "auth_token=super_secret_123") {
		t.Errorf("prompt leaked query secret: %s", capturedUserPrompt)
	}
	if strings.Contains(capturedUserPrompt, "secret-token-xyz") {
		t.Errorf("prompt leaked bearer token: %s", capturedUserPrompt)
	}
}

func TestIncidentSummary_SanitizeURL(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{
			"https://todo-fm-d.vercel.app/",
			"https://todo-fm-d.vercel.app/",
		},
		{
			"https://user:password@example.com:8443/api/v1/health?token=secret&env=prod#debug",
			"https://example.com:8443/api/v1/health",
		},
		{
			"http://admin:12345@192.168.1.1/setup?key=secret",
			"http://192.168.1.1/setup",
		},
	}

	for _, tc := range tests {
		got := ai.SanitizeURL(tc.input)
		if got != tc.expected {
			t.Errorf("SanitizeURL(%q) = %q; expected %q", tc.input, got, tc.expected)
		}
	}
}

func TestIncidentSummary_SanitizeErrorSummary(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"connection reset by peer", "connection reset by peer"},
		{
			"Request failed with Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.xyz",
			"Request failed with Bearer [REDACTED]",
		},
		{
			"Failed request to endpoint api_key=super_secret_abc12345",
			"Failed request to endpoint api_key=[REDACTED]",
		},
	}

	for _, tc := range tests {
		got := ai.SanitizeErrorSummary(tc.input)
		if got != tc.expected {
			t.Errorf("SanitizeErrorSummary(%q) = %q; expected %q", tc.input, got, tc.expected)
		}
	}
}

func TestIncidentSummary_AIError_Fallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream timeout", http.StatusGatewayTimeout)
	}))
	defer server.Close()

	client := ai.NewClient("test-key", ai.WithBaseURL(server.URL))

	incident := ai.IncidentMetadata{
		Timestamp:      time.Now(),
		WatcherName:    "Todo FM Production",
		URL:            "https://todo-fm-d.vercel.app/?secret=private",
		ExpectedStatus: 200,
		ActualStatus:   502,
		LatencyMs:      320,
		ErrorSummary:   "Bad Gateway",
	}

	result, err := ai.SummarizeIncident(context.Background(), client, incident)
	if err != nil {
		t.Fatalf("expected nil error on fallback, got %v", err)
	}

	if !result.Fallback {
		t.Fatal("expected fallback=true when AI call fails")
	}

	if len(result.Facts) == 0 {
		t.Fatal("expected fallback to provide deterministic facts")
	}
	if len(result.Hypotheses) == 0 {
		t.Fatal("expected fallback to provide deterministic hypotheses")
	}

	// Verify facts contain watcher name and sanitized URL
	factsText := strings.Join(result.Facts, " ")
	if !strings.Contains(factsText, "Todo FM Production") {
		t.Errorf("facts should contain watcher name: %s", factsText)
	}
	if strings.Contains(factsText, "secret=private") {
		t.Errorf("facts should have sanitized URL without query params: %s", factsText)
	}
	if !strings.Contains(factsText, "502") {
		t.Errorf("facts should contain actual status 502: %s", factsText)
	}

	// Verify hypotheses contain server error guidance
	hypText := strings.Join(result.Hypotheses, " ")
	if !strings.Contains(hypText, "5xx") {
		t.Errorf("hypotheses for 502 should mention 5xx server error: %s", hypText)
	}
}

func TestIncidentSummary_NetworkTimeout_Fallback(t *testing.T) {
	client := ai.NewClient("") // No key

	incident := ai.IncidentMetadata{
		Timestamp:      time.Now(),
		WatcherName:    "Test Socket Timeout",
		URL:            "https://example.com/health",
		ExpectedStatus: 200,
		ActualStatus:   0,
		LatencyMs:      10000,
		ErrorSummary:   "i/o timeout dialing socket",
	}

	result, err := ai.SummarizeIncident(context.Background(), client, incident)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if !result.Fallback {
		t.Fatal("expected fallback=true")
	}

	hypText := strings.Join(result.Hypotheses, " ")
	if !strings.Contains(hypText, "DNS") && !strings.Contains(hypText, "เครือข่าย") {
		t.Errorf("hypotheses for status 0 should suggest network/DNS checks: %s", hypText)
	}
}
