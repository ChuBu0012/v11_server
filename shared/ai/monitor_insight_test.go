package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSummarizeMonitorInsight_NormalJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ChatCompletionResponse{
			Choices: []ChatChoice{
				{
					Message: ChatMessage{
						Content: `{"summary":"ระบบทำงานปกติ","observations":["RAM ปกติ","ดิสก์เหลือพอ"],"next_checks":["ตรวจอุณหภูมิรอบถัดไป"]}`,
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	client := NewClient("fake-key", WithBaseURL(ts.URL))
	evidence := MonitorEvidence{
		HostRAMTotalMB: 4000,
		HostRAMUsedMB:  1500,
		DiskTotalGB:    64,
		DiskFreeGB:     30,
	}

	insight := SummarizeMonitorInsight(context.Background(), client, evidence)
	if insight == nil {
		t.Fatal("expected non-nil insight")
	}
	if insight.Summary != "ระบบทำงานปกติ" {
		t.Errorf("expected 'ระบบทำงานปกติ', got: %s", insight.Summary)
	}
	if len(insight.Observations) != 2 {
		t.Errorf("expected 2 observations, got: %d", len(insight.Observations))
	}
	if len(insight.NextChecks) != 1 {
		t.Errorf("expected 1 next check, got: %d", len(insight.NextChecks))
	}
}

func TestSummarizeMonitorInsight_CodeFenceJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ChatCompletionResponse{
			Choices: []ChatChoice{
				{
					Message: ChatMessage{
						Content: "```json\n{\"summary\":\"สรุปสถานะ\",\"observations\":[\"ข้อสังเกต 1\"],\"next_checks\":[\"เช็ค 1\"]}\n```",
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	client := NewClient("fake-key", WithBaseURL(ts.URL))
	evidence := MonitorEvidence{
		HostRAMTotalMB: 4000,
		HostRAMUsedMB:  1500,
	}

	insight := SummarizeMonitorInsight(context.Background(), client, evidence)
	if insight == nil {
		t.Fatal("expected non-nil insight")
	}
	if insight.Summary != "สรุปสถานะ" {
		t.Errorf("expected 'สรุปสถานะ', got: %s", insight.Summary)
	}
}

func TestSummarizeMonitorInsight_NilClientFallback(t *testing.T) {
	evidence := MonitorEvidence{
		HostRAMTotalMB:   4000,
		HostRAMUsedMB:    2000,
		DiskTotalGB:      64,
		DiskFreeGB:       32,
		BatteryAvailable: true,
		BatteryPct:       85,
	}

	insight := SummarizeMonitorInsight(context.Background(), nil, evidence)
	if insight == nil {
		t.Fatal("expected fallback insight")
	}
	if insight.FallbackMessage == "" {
		t.Errorf("expected FallbackMessage on nil client")
	}
	if len(insight.Observations) != 3 {
		t.Errorf("expected 3 fallback observations, got: %d", len(insight.Observations))
	}
}
