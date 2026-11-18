package bot

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestDefaultStatusProvider(t *testing.T) {
	startTime := time.Now().Add(-2 * time.Hour).Add(-15 * time.Minute)

	mockExec := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return []byte(`{"percentage": 82.0, "status": "DISCHARGING", "temperature": 32.5}`), nil
	}

	mockHTTPTransport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "/api/tunnels") {
			body := `{"tunnels":[{"public_url":"https://test-subdomain.ngrok-free.dev","proto":"https"}]}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(strings.NewReader("404")),
		}, nil
	})

	httpClient := &http.Client{Transport: mockHTTPTransport}

	prov := NewDefaultStatusProvider(
		startTime,
		WithStatusExecRunner(mockExec),
		WithStatusHTTPClient(httpClient),
	)

	// Test Uptime String
	uptime := prov.GetUptimeString()
	if !strings.Contains(uptime, "2h 15m") {
		t.Errorf("Expected uptime to contain '2h 15m', got: %s", uptime)
	}

	// Test Status Card
	ctx := context.Background()
	card, err := prov.GetStatusCard(ctx)
	if err != nil {
		t.Fatalf("GetStatusCard failed: %v", err)
	}
	if !strings.Contains(card, "Pocket Sentinel — System Status") {
		t.Errorf("Expected title in status card, got: %s", card)
	}
	if !strings.Contains(card, "82% (DISCHARGING, 32.5°C)") {
		t.Errorf("Expected battery stats in status card, got: %s", card)
	}
	if !strings.Contains(card, "https://test-subdomain.ngrok-free.dev") {
		t.Errorf("Expected ngrok tunnel url in status card, got: %s", card)
	}
	if !strings.Contains(card, "Memory:") || !strings.Contains(card, "Disk") {
		t.Errorf("Expected memory and disk in status card, got: %s", card)
	}
}
