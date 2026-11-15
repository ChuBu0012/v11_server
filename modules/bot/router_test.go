package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type mockStatusProvider struct {
	statusCard string
	uptimeStr  string
}

func (m *mockStatusProvider) GetStatusCard(ctx context.Context) (string, error) {
	return m.statusCard, nil
}

func (m *mockStatusProvider) GetUptimeString() string {
	return m.uptimeStr
}

func TestRouter(t *testing.T) {
	var lastSentMessage string
	var lastChatID int64
	var mu sync.Mutex

	mockTransport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		mu.Lock()
		defer mu.Unlock()

		if strings.Contains(r.URL.Path, "sendMessage") {
			var payload SendMessagePayload
			_ = json.NewDecoder(r.Body).Decode(&payload)
			lastSentMessage = payload.Text
			lastChatID = payload.ChatID

			resp := APIResponse{
				OK: true,
				Result: json.RawMessage(`{
					"message_id": 99,
					"date": 1600000000,
					"chat": {"id": 123456, "type": "private"},
					"text": "sent"
				}`),
			}
			bodyBytes, _ := json.Marshal(resp)
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(strings.NewReader("404 not found")),
		}, nil
	})

	httpClient := &http.Client{Transport: mockTransport}
	client := NewClient("TEST_TOKEN", WithBaseURL("https://api.telegram.org"), WithHTTPClient(httpClient))
	auth := NewAuthorizer("123456", nil)

	mockTorchRunner := &mockTorchRunner{}
	torch := NewTorchController(WithCommandRunner(mockTorchRunner.Run))

	mockExecRunner := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return []byte("Filesystem 107G 32G 74G /data"), nil
	}
	executor := NewSafeExecutor(WithExecRunner(mockExecRunner))

	statusProv := &mockStatusProvider{
		statusCard: "📊 <b>Pocket Sentinel — System Status</b>\n🔋 Battery: 85%\n🧠 Memory: 15MB",
		uptimeStr:  "2h 30m",
	}

	var rebootTriggered int32
	rebootCallback := func() {
		atomic.StoreInt32(&rebootTriggered, 1)
	}

	router := NewRouter(
		client,
		auth,
		torch,
		executor,
		statusProv,
		WithRebootFunc(rebootCallback),
	)

	ctx := context.Background()

	makeUpdate := func(cmd string) Update {
		return Update{
			UpdateID: 1,
			Message: &Message{
				MessageID: 1,
				Chat:      &Chat{ID: 123456},
				Text:      cmd,
			},
		}
	}

	// 1. /start or /help
	err := router.HandleUpdate(ctx, makeUpdate("/start"))
	if err != nil {
		t.Fatalf("/start failed: %v", err)
	}
	mu.Lock()
	if lastChatID != 123456 {
		t.Errorf("Expected chat ID 123456, got %d", lastChatID)
	}
	if !strings.Contains(lastSentMessage, "Pocket Sentinel 2.0") || !strings.Contains(lastSentMessage, "/status") {
		t.Errorf("Expected /start help text, got: %s", lastSentMessage)
	}
	mu.Unlock()

	// 2. /ping
	err = router.HandleUpdate(ctx, makeUpdate("/ping"))
	if err != nil {
		t.Fatalf("/ping failed: %v", err)
	}
	mu.Lock()
	if !strings.Contains(lastSentMessage, "Pong!") || !strings.Contains(lastSentMessage, "2h 30m") {
		t.Errorf("Expected pong with uptime, got: %s", lastSentMessage)
	}
	mu.Unlock()

	// 3. /status
	err = router.HandleUpdate(ctx, makeUpdate("/status"))
	if err != nil {
		t.Fatalf("/status failed: %v", err)
	}
	mu.Lock()
	if !strings.Contains(lastSentMessage, "System Status") || !strings.Contains(lastSentMessage, "85%") {
		t.Errorf("Expected status card, got: %s", lastSentMessage)
	}
	mu.Unlock()

	// 4. /torch on, off, status
	err = router.HandleUpdate(ctx, makeUpdate("/torch on"))
	if err != nil {
		t.Fatalf("/torch on failed: %v", err)
	}
	mu.Lock()
	if !strings.Contains(lastSentMessage, "Torch turned ON") {
		t.Errorf("Expected torch on message, got: %s", lastSentMessage)
	}
	mu.Unlock()

	err = router.HandleUpdate(ctx, makeUpdate("/torch status"))
	if err != nil {
		t.Fatalf("/torch status failed: %v", err)
	}
	mu.Lock()
	if !strings.Contains(lastSentMessage, "Torch is currently") {
		t.Errorf("Expected torch status message, got: %s", lastSentMessage)
	}
	mu.Unlock()

	err = router.HandleUpdate(ctx, makeUpdate("/torch off"))
	if err != nil {
		t.Fatalf("/torch off failed: %v", err)
	}
	mu.Lock()
	if !strings.Contains(lastSentMessage, "Torch turned OFF") {
		t.Errorf("Expected torch off message, got: %s", lastSentMessage)
	}
	mu.Unlock()

	// 5. /exec
	err = router.HandleUpdate(ctx, makeUpdate("/exec df -h"))
	if err != nil {
		t.Fatalf("/exec df failed: %v", err)
	}
	mu.Lock()
	if !strings.Contains(lastSentMessage, "Filesystem 107G") {
		t.Errorf("Expected exec df output, got: %s", lastSentMessage)
	}
	mu.Unlock()

	// 6. /reboot
	err = router.HandleUpdate(ctx, makeUpdate("/reboot"))
	if err != nil {
		t.Fatalf("/reboot failed: %v", err)
	}
	mu.Lock()
	if !strings.Contains(lastSentMessage, "Restarting Pocket Sentinel via PM2") {
		t.Errorf("Expected reboot message, got: %s", lastSentMessage)
	}
	mu.Unlock()

	time.Sleep(600 * time.Millisecond)
	if atomic.LoadInt32(&rebootTriggered) != 1 {
		t.Error("Expected reboot callback to be called after 500ms delay")
	}

	// 7. Unknown command
	err = router.HandleUpdate(ctx, makeUpdate("/foo_bar"))
	if err != nil {
		t.Fatalf("Unknown command failed: %v", err)
	}
	mu.Lock()
	if !strings.Contains(lastSentMessage, "Unknown command") {
		t.Errorf("Expected unknown command warning, got: %s", lastSentMessage)
	}
	mu.Unlock()

	// 8. Command with bot username suffix (e.g. /ping@pocket_sentinel_bot)
	err = router.HandleUpdate(ctx, makeUpdate("/ping@pocket_sentinel_bot"))
	if err != nil {
		t.Fatalf("Ping with suffix failed: %v", err)
	}
	mu.Lock()
	if !strings.Contains(lastSentMessage, "Pong!") {
		t.Errorf("Expected pong for suffixed command, got: %s", lastSentMessage)
	}
	mu.Unlock()

	// 9. /exec with bot username suffix (e.g. /exec@pocket_sentinel_bot df -h)
	err = router.HandleUpdate(ctx, makeUpdate("/exec@pocket_sentinel_bot df -h"))
	if err != nil {
		t.Fatalf("Exec with suffix failed: %v", err)
	}
	mu.Lock()
	if !strings.Contains(lastSentMessage, "Filesystem 107G") {
		t.Errorf("Expected exec df output with bot suffix, got: %s", lastSentMessage)
	}
	mu.Unlock()

	// 10. /EXEC with uppercase and arguments
	err = router.HandleUpdate(ctx, makeUpdate("/EXEC df -h"))
	if err != nil {
		t.Fatalf("Exec uppercase failed: %v", err)
	}
	mu.Lock()
	if !strings.Contains(lastSentMessage, "Filesystem 107G") {
		t.Errorf("Expected exec df output with uppercase command, got: %s", lastSentMessage)
	}
	mu.Unlock()
}

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
