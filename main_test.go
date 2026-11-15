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
