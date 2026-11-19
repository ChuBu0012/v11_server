package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"v11-server/modules/logs"
	"v11-server/modules/system"
	"v11-server/modules/telemetry"
	"v11-server/shared/config"
	"v11-server/shared/db"
)

func setupTestApp(t *testing.T) (*App, *gin.Engine, func()) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	tempDir, err := os.MkdirTemp("", "v11-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tempDir, "test.db")
	dbConn, err := db.InitDB(dbPath)
	if err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("Failed to init db: %v", err)
	}

	cfg := &config.Config{
		SampleInterval:   5 * time.Second,
		RetentionMaxAge:  24 * time.Hour,
		RetentionMaxRows: 100,
		BatteryThreshold: 20,
		DiskThresholdGB:  5.0,
		TelegramTimeout:  2 * time.Second,
		RelayPIN:         "1804",
		ReminderPIN:      "1804",
		WatchtowerPIN:    "1804",
	}

	app := setupApp(dbConn, "8080", cfg)
	router := setupRouter(app)

	cleanup := func() {
		dbConn.Close()
		os.RemoveAll(tempDir)
	}

	return app, router, cleanup
}

func TestDashboardEndpoint(t *testing.T) {
	_, router, cleanup := setupTestApp(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rr.Code)
	}

	if !strings.Contains(rr.Body.String(), "Pocket Sentinel") {
		t.Errorf("Expected response body to contain 'Pocket Sentinel'")
	}
}

func TestLocationViewerRouteIsTokenProtected(t *testing.T) {
	app, router, cleanup := setupTestApp(t)
	defer cleanup()
	_, viewerToken := app.locationStore.Tokens()

	unauthorized := httptest.NewRecorder()
	router.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/live/wrong-token", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("invalid viewer token status = %d", unauthorized.Code)
	}

	authorized := httptest.NewRecorder()
	router.ServeHTTP(authorized, httptest.NewRequest(http.MethodGet, "/live/"+viewerToken, nil))
	if authorized.Code != http.StatusOK || !strings.Contains(authorized.Body.String(), "live-map-canvas") {
		t.Fatalf("valid viewer response = %d (%d bytes)", authorized.Code, authorized.Body.Len())
	}
	if authorized.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", authorized.Header().Get("Cache-Control"))
	}
}

func TestLiveViewerRequestPathRedactsToken(t *testing.T) {
	if got := redactedRequestPath("/live/private-viewer-token"); got != "/live/[token]" {
		t.Fatalf("redacted request path = %q", got)
	}
	if got := redactedRequestPath("/api/location/live"); got != "/api/location/live" {
		t.Fatalf("unrelated request path changed: %q", got)
	}
	if got := redactedRequestPath("/api/location/report?device=me&token=private-secret"); strings.Contains(got, "private-secret") {
		t.Fatalf("location report token was not redacted: %q", got)
	}
}

func TestHealthEndpoint(t *testing.T) {
	_, router, cleanup := setupTestApp(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rr.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if resp["status"] != "ok" {
		t.Errorf("Expected status 'ok', got %v", resp["status"])
	}
	if resp["service"] != "v11-pocket-server" {
		t.Errorf("Expected service 'v11-pocket-server', got %v", resp["service"])
	}
}

func TestSystemEndpoint(t *testing.T) {
	_, router, cleanup := setupTestApp(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/system", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rr.Code)
	}

	var status system.SystemStatus
	if err := json.Unmarshal(rr.Body.Bytes(), &status); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if status.Service != "v11-pocket-server" {
		t.Errorf("Expected service 'v11-pocket-server', got %s", status.Service)
	}
	if status.Memory == nil {
		t.Error("Expected memory stats to be populated")
	}
	if status.Disk == nil {
		t.Error("Expected disk stats to be populated")
	}
}

func TestBatteryEndpoint(t *testing.T) {
	_, router, cleanup := setupTestApp(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/device/battery", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rr.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if _, exists := resp["available"]; !exists {
		t.Error("Expected 'available' boolean field in response")
	}
}

func TestTelemetryEndpoints(t *testing.T) {
	app, router, cleanup := setupTestApp(t)
	defer cleanup()

	// 1. Run collection sample
	sample, err := app.collector.CollectOnce(t.Context())
	if err != nil {
		t.Fatalf("CollectOnce failed: %v", err)
	}
	if sample == nil {
		t.Fatal("Expected sample not to be nil")
	}

	// 2. Query history
	reqHist := httptest.NewRequest(http.MethodGet, "/api/telemetry/history?limit=10", nil)
	rrHist := httptest.NewRecorder()
	router.ServeHTTP(rrHist, reqHist)

	if rrHist.Code != http.StatusOK {
		t.Fatalf("Expected status 200 for history, got %d", rrHist.Code)
	}

	var samples []telemetry.HealthSample
	if err := json.Unmarshal(rrHist.Body.Bytes(), &samples); err != nil {
		t.Fatalf("Failed to parse history JSON: %v", err)
	}
	if len(samples) != 1 {
		t.Errorf("Expected 1 sample in history, got %d", len(samples))
	}

	// 3. Query current
	reqCur := httptest.NewRequest(http.MethodGet, "/api/telemetry/current", nil)
	rrCur := httptest.NewRecorder()
	router.ServeHTTP(rrCur, reqCur)

	if rrCur.Code != http.StatusOK {
		t.Fatalf("Expected status 200 for current, got %d", rrCur.Code)
	}
}

func TestLogsEndpoint(t *testing.T) {
	_, router, cleanup := setupTestApp(t)
	defer cleanup()

	// 1. Post a new log
	payload := logs.LogInput{
		Level:    "warn",
		Message:  "Battery level low test",
		Metadata: "percentage=15",
	}
	body, _ := json.Marshal(payload)
	reqPost := httptest.NewRequest(http.MethodPost, "/api/logs", bytes.NewReader(body))
	reqPost.Header.Set("Content-Type", "application/json")
	rrPost := httptest.NewRecorder()

	router.ServeHTTP(rrPost, reqPost)

	if rrPost.Code != http.StatusCreated {
		t.Fatalf("Expected status 201 Created, got %d. Body: %s", rrPost.Code, rrPost.Body.String())
	}

	// 2. Query logs
	reqGet := httptest.NewRequest(http.MethodGet, "/api/logs", nil)
	rrGet := httptest.NewRecorder()

	router.ServeHTTP(rrGet, reqGet)

	if rrGet.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rrGet.Code)
	}

	var logsList []logs.EventLog
	if err := json.Unmarshal(rrGet.Body.Bytes(), &logsList); err != nil {
		t.Fatalf("Failed to parse logs JSON: %v", err)
	}

	if len(logsList) == 0 {
		t.Fatal("Expected at least 1 log entry, got 0")
	}

	if logsList[0].Message != "Battery level low test" {
		t.Errorf("Expected message 'Battery level low test', got '%s'", logsList[0].Message)
	}
	if logsList[0].Level != "warn" {
		t.Errorf("Expected level 'warn', got '%s'", logsList[0].Level)
	}
}

func TestTelegramBotWiring(t *testing.T) {
	app, _, cleanup := setupTestApp(t)
	defer cleanup()

	// Default test config has empty token, so botPoller should be nil
	if app.botPoller != nil {
		t.Errorf("Expected botPoller to be nil when Telegram is unconfigured")
	}

	// Mock Telegram API server for wiring lifecycle test
	mockTelegramServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true,"result":[]}`))
	}))
	defer mockTelegramServer.Close()
	t.Setenv("TELEGRAM_API_URL", mockTelegramServer.URL)

	// Now configure Telegram and verify botPoller is created
	cfgWithTelegram := &config.Config{
		SampleInterval:   5 * time.Second,
		RetentionMaxAge:  24 * time.Hour,
		RetentionMaxRows: 100,
		BatteryThreshold: 20,
		DiskThresholdGB:  5.0,
		TelegramBotToken: "TEST_TOKEN_123",
		TelegramChatID:   "999888777",
		TelegramTimeout:  2 * time.Second,
	}

	appWithTelegram := setupApp(app.db, "8080", cfgWithTelegram)
	if appWithTelegram.botPoller == nil {
		t.Fatal("Expected botPoller to be initialized when Telegram is configured")
	}

	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		appWithTelegram.botPoller.Start(ctx)
		close(stopped)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-stopped:
		// Clean stop
	case <-time.After(2 * time.Second):
		t.Fatal("botPoller did not stop on context cancellation")
	}
}

func TestWatchtowerEndpoints_Integration(t *testing.T) {
	_, router, cleanup := setupTestApp(t)
	defer cleanup()

	// 1. GET /api/watchtower/summary
	reqSum := httptest.NewRequest(http.MethodGet, "/api/watchtower/summary", nil)
	rrSum := httptest.NewRecorder()
	router.ServeHTTP(rrSum, reqSum)

	if rrSum.Code != http.StatusOK {
		t.Fatalf("Expected 200 for summary, got %d", rrSum.Code)
	}

	var sum map[string]interface{}
	if err := json.Unmarshal(rrSum.Body.Bytes(), &sum); err != nil {
		t.Fatalf("Failed to parse summary JSON: %v", err)
	}
	if sum["status"] != "healthy" {
		t.Errorf("Expected status 'healthy', got %v", sum["status"])
	}

	// 2. POST /api/watchtower/watchers without PIN -> 401
	payload := `{"name":"Todo FM App","url":"https://todo-fm-d.vercel.app/","expected_status":200}`
	reqNoPin := httptest.NewRequest(http.MethodPost, "/api/watchtower/watchers", strings.NewReader(payload))
	reqNoPin.Header.Set("Content-Type", "application/json")
	rrNoPin := httptest.NewRecorder()
	router.ServeHTTP(rrNoPin, reqNoPin)

	if rrNoPin.Code != http.StatusUnauthorized {
		t.Fatalf("Expected 401 Unauthorized for missing PIN, got %d", rrNoPin.Code)
	}

	// 3. POST /api/watchtower/watchers with PIN -> 201
	reqWithPin := httptest.NewRequest(http.MethodPost, "/api/watchtower/watchers", strings.NewReader(payload))
	reqWithPin.Header.Set("Content-Type", "application/json")
	reqWithPin.Header.Set("X-Watchtower-PIN", "1804")
	rrWithPin := httptest.NewRecorder()
	router.ServeHTTP(rrWithPin, reqWithPin)

	if rrWithPin.Code != http.StatusCreated {
		t.Fatalf("Expected 201 Created with valid PIN, got %d (%s)", rrWithPin.Code, rrWithPin.Body.String())
	}
}

func TestScheduleNLParse_Integration(t *testing.T) {
	_, router, cleanup := setupTestApp(t)
	defer cleanup()

	// 1. Without PIN -> 401
	reqNoPin := httptest.NewRequest(http.MethodPost, "/api/schedule/parse", strings.NewReader(`{"prompt":"ประชุมทีม"}`))
	reqNoPin.Header.Set("Content-Type", "application/json")
	rrNoPin := httptest.NewRecorder()
	router.ServeHTTP(rrNoPin, reqNoPin)

	if rrNoPin.Code != http.StatusUnauthorized {
		t.Fatalf("Expected 401 without PIN, got %d", rrNoPin.Code)
	}

	// 2. Empty prompt -> 400
	reqEmpty := httptest.NewRequest(http.MethodPost, "/api/schedule/parse", strings.NewReader(`{"prompt":""}`))
	reqEmpty.Header.Set("Content-Type", "application/json")
	reqEmpty.Header.Set("X-Reminder-PIN", "1804")
	rrEmpty := httptest.NewRecorder()
	router.ServeHTTP(rrEmpty, reqEmpty)

	if rrEmpty.Code != http.StatusBadRequest {
		t.Fatalf("Expected 400 for empty prompt, got %d", rrEmpty.Code)
	}

	// 3. Ambiguous prompt -> 200 with needs_clarification=true
	reqAmbiguous := httptest.NewRequest(http.MethodPost, "/api/schedule/parse", strings.NewReader(`{"prompt":"เตือนกินยาวันพรุ่งนี้ตอนสายๆ"}`))
	reqAmbiguous.Header.Set("Content-Type", "application/json")
	reqAmbiguous.Header.Set("X-Reminder-PIN", "1804")
	rrAmbiguous := httptest.NewRecorder()
	router.ServeHTTP(rrAmbiguous, reqAmbiguous)

	if rrAmbiguous.Code != http.StatusOK {
		t.Fatalf("Expected 200 for ambiguous prompt, got %d", rrAmbiguous.Code)
	}

	var ambResp map[string]interface{}
	if err := json.Unmarshal(rrAmbiguous.Body.Bytes(), &ambResp); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}
	if ambResp["needs_clarification"] != true {
		t.Errorf("Expected needs_clarification=true, got %v", ambResp["needs_clarification"])
	}

	// 4. Clear prompt without API key -> 200 with deterministic fallback preview
	reqClear := httptest.NewRequest(http.MethodPost, "/api/schedule/parse", strings.NewReader(`{"prompt":"จ่ายค่าไฟวันพรุ่งนี้ 09:00"}`))
	reqClear.Header.Set("Content-Type", "application/json")
	reqClear.Header.Set("X-Reminder-PIN", "1804")
	rrClear := httptest.NewRecorder()
	router.ServeHTTP(rrClear, reqClear)

	if rrClear.Code != http.StatusOK {
		t.Fatalf("Expected 200 for clear prompt, got %d", rrClear.Code)
	}

	var clearResp map[string]interface{}
	if err := json.Unmarshal(rrClear.Body.Bytes(), &clearResp); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}
	if clearResp["fallback"] != true {
		t.Errorf("Expected fallback=true when AI key not set, got %v", clearResp["fallback"])
	}
	if clearResp["preview"] == nil {
		t.Error("Expected preview object to be populated in fallback")
	}
}

func TestRelayProofread_Integration(t *testing.T) {
	_, router, cleanup := setupTestApp(t)
	defer cleanup()

	// 1. Without PIN -> 401
	reqNoPin := httptest.NewRequest(http.MethodPost, "/api/relay/proofread", strings.NewReader(`{"text":"สวัสดีครับ"}`))
	reqNoPin.Header.Set("Content-Type", "application/json")
	rrNoPin := httptest.NewRecorder()
	router.ServeHTTP(rrNoPin, reqNoPin)

	if rrNoPin.Code != http.StatusUnauthorized {
		t.Fatalf("Expected 401 without PIN, got %d", rrNoPin.Code)
	}

	// 2. Empty text -> 400
	reqEmpty := httptest.NewRequest(http.MethodPost, "/api/relay/proofread", strings.NewReader(`{"text":""}`))
	reqEmpty.Header.Set("Content-Type", "application/json")
	reqEmpty.Header.Set("X-Relay-PIN", "1804")
	rrEmpty := httptest.NewRecorder()
	router.ServeHTTP(rrEmpty, reqEmpty)

	if rrEmpty.Code != http.StatusBadRequest {
		t.Fatalf("Expected 400 for empty text, got %d", rrEmpty.Code)
	}

	// 3. Valid text (no API key configured -> deterministic fallback) -> 200
	reqValid := httptest.NewRequest(http.MethodPost, "/api/relay/proofread", strings.NewReader(`{"text":"เซิร์ฟเวอร์ออนไลน์พร้อมใช้งาน https://example.com"}`))
	reqValid.Header.Set("Content-Type", "application/json")
	reqValid.Header.Set("X-Relay-PIN", "1804")
	rrValid := httptest.NewRecorder()
	router.ServeHTTP(rrValid, reqValid)

	if rrValid.Code != http.StatusOK {
		t.Fatalf("Expected 200 for proofread, got %d", rrValid.Code)
	}

	var proofResp map[string]interface{}
	if err := json.Unmarshal(rrValid.Body.Bytes(), &proofResp); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}
	if !strings.Contains(proofResp["original"].(string), "เซิร์ฟเวอร์ออนไลน์พร้อมใช้งาน") {
		t.Errorf("Expected original text preserved, got %v", proofResp["original"])
	}
}

func TestWatchtowerScheduler_Lifecycle(t *testing.T) {
	app, _, cleanup := setupTestApp(t)
	defer cleanup()

	if app.watchtowerService == nil {
		t.Fatal("Expected watchtowerService to be initialized in setupApp")
	}

	ctx, cancel := context.WithCancel(context.Background())
	app.watchtowerService.Start(ctx)

	time.Sleep(20 * time.Millisecond)
	cancel()
	app.watchtowerService.Stop()
}
