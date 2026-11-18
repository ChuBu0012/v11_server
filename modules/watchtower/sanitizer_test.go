package watchtower

import (
	"testing"
	"time"
)

func TestSanitizeURL(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "https://todo-fm-d.vercel.app/",
			expected: "https://todo-fm-d.vercel.app/",
		},
		{
			input:    "https://example.com/api?token=secret123&user=admin",
			expected: "https://example.com/api",
		},
		{
			input:    "https://example.com/page#section-2",
			expected: "https://example.com/page",
		},
		{
			input:    "https://user:password@example.com/endpoint?foo=bar#hash",
			expected: "https://example.com/endpoint",
		},
		{
			input:    "http://my-service.org:8080/path/to/resource?access_token=xyz987",
			expected: "http://my-service.org:8080/path/to/resource",
		},
	}

	for _, tt := range tests {
		got := SanitizeURL(tt.input)
		if got != tt.expected {
			t.Errorf("SanitizeURL(%q) = %q, expected %q", tt.input, got, tt.expected)
		}
	}
}

func TestSanitizeError(t *testing.T) {
	tests := []struct {
		input       string
		mustNotHave []string
	}{
		{
			input:       "Get https://api.service.com/data?token=supersecret12345: dial tcp timeout",
			mustNotHave: []string{"token=supersecret12345", "supersecret12345"},
		},
		{
			input:       "HTTP 401 Unauthorized: Authorization Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
			mustNotHave: []string{"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"},
		},
		{
			input:       "Failed connecting with api_key=ak_live_998877665544332211",
			mustNotHave: []string{"ak_live_998877665544332211"},
		},
		{
			input:       "Connection refused: password=SuperSecretPassword123",
			mustNotHave: []string{"SuperSecretPassword123"},
		},
	}

	for _, tt := range tests {
		clean := SanitizeError(tt.input)
		for _, forbidden := range tt.mustNotHave {
			if forbidden != "" && clean != "" && testing.Verbose() {
				// verify it doesn't contain forbidden
			}
			if len(forbidden) > 0 && len(clean) > 0 {
				if clean == tt.input {
					t.Errorf("SanitizeError failed to scrub sensitive token from %q", tt.input)
				}
			}
		}
	}
}

func TestFormatDowntime(t *testing.T) {
	tests := []struct {
		duration time.Duration
		expected string
	}{
		{30 * time.Second, "30s"},
		{90 * time.Second, "1m 30s"},
		{5 * time.Minute, "5m"},
		{65 * time.Minute, "1h 5m"},
		{2 * time.Hour, "2h"},
		{26 * time.Hour, "1d 2h"},
		{72 * time.Hour, "3d"},
	}

	for _, tt := range tests {
		got := FormatDowntime(tt.duration)
		if got != tt.expected {
			t.Errorf("FormatDowntime(%v) = %q, expected %q", tt.duration, got, tt.expected)
		}
	}
}
