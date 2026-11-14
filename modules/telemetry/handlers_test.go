package telemetry

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"v11-server/modules/alerts"
	"v11-server/shared/db"
)

func setupTestModule(t *testing.T) (*Module, *gin.Engine, func()) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	tempDir, err := os.MkdirTemp("", "v11-handler-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tempDir, "test.db")
	dbConn, err := db.InitDB(dbPath)
	if err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("Failed to init db: %v", err)
	}

	repo := NewRepository(dbConn)
	alertStore := alerts.NewAlertStore(dbConn)
	thresholds := alerts.Thresholds{
		BatteryPercent: 20.0,
		DiskFreeGB:     5.0,
	}

	sysP := &fakeSystemProvider{uptime: 100, alloc: 1.0, sys: 5.0, total: 100.0, free: 50.0}
	batP := &fakeBatteryProvider{}
	collector := NewCollector(repo, time.Now(), WithSystemProvider(sysP), WithBatteryProvider(batP))

	mod := NewModule(dbConn, collector, alertStore, thresholds)

	r := gin.New()
	mod.RegisterRoutes(r.Group("/api/telemetry"))

	cleanup := func() {
		dbConn.Close()
		os.RemoveAll(tempDir)
	}

	return mod, r, cleanup
}

func TestHistoryEndpointEmpty(t *testing.T) {
	_, router, cleanup := setupTestModule(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/telemetry/history", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rr.Code)
	}

	var samples []HealthSample
	if err := json.Unmarshal(rr.Body.Bytes(), &samples); err != nil {
		t.Fatalf("Failed to decode JSON: %v", err)
	}

	if len(samples) != 0 {
		t.Errorf("Expected empty slice, got %d items", len(samples))
	}
}

func TestHistoryEndpointParams(t *testing.T) {
	mod, router, cleanup := setupTestModule(t)
	defer cleanup()

	// Seed 5 samples
	for i := 1; i <= 5; i++ {
		_, _ = mod.repo.InsertSample(t.Context(), HealthSample{
			Timestamp:     time.Now().UTC().Add(-time.Duration(i) * time.Minute),
			UptimeSeconds: int64(i * 60),
			MemoryAllocMB: float64(i),
			MemorySysMB:   10.0,
			DiskTotalGB:   100.0,
			DiskFreeGB:    50.0,
		})
	}

	// 1. Query with valid limit=2
	req := httptest.NewRequest(http.MethodGet, "/api/telemetry/history?limit=2", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rr.Code)
	}
	var samples []HealthSample
	_ = json.Unmarshal(rr.Body.Bytes(), &samples)
	if len(samples) != 2 {
		t.Errorf("Expected 2 samples with limit=2, got %d", len(samples))
	}

	// 2. Query with invalid limit
	reqBad := httptest.NewRequest(http.MethodGet, "/api/telemetry/history?limit=invalid", nil)
	rrBad := httptest.NewRecorder()
	router.ServeHTTP(rrBad, reqBad)
	if rrBad.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 Bad Request for invalid limit, got %d", rrBad.Code)
	}

	// 3. Query with invalid hours
	reqBadH := httptest.NewRequest(http.MethodGet, "/api/telemetry/history?hours=bad", nil)
	rrBadH := httptest.NewRecorder()
	router.ServeHTTP(rrBadH, reqBadH)
	if rrBadH.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 Bad Request for invalid hours, got %d", rrBadH.Code)
	}
}

func TestCurrentEndpoint(t *testing.T) {
	mod, router, cleanup := setupTestModule(t)
	defer cleanup()

	// Initially with no collection
	req := httptest.NewRequest(http.MethodGet, "/api/telemetry/current", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rr.Code)
	}

	var resp struct {
		LatestSample *HealthSample      `json:"latest_sample"`
		Conditions   []alerts.Condition `json:"conditions"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}
	if resp.LatestSample != nil {
		t.Errorf("Expected nil latest sample initially, got %+v", resp.LatestSample)
	}

	// Now run collection once
	_, err := mod.collector.CollectOnce(t.Context())
	if err != nil {
		t.Fatalf("CollectOnce failed: %v", err)
	}

	rr2 := httptest.NewRecorder()
	router.ServeHTTP(rr2, req)

	if rr2.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rr2.Code)
	}

	var resp2 struct {
		LatestSample *HealthSample      `json:"latest_sample"`
		Conditions   []alerts.Condition `json:"conditions"`
	}
	if err := json.Unmarshal(rr2.Body.Bytes(), &resp2); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if resp2.LatestSample == nil {
		t.Fatal("Expected populated latest sample after collection")
	}
	if len(resp2.Conditions) == 0 {
		t.Error("Expected evaluated conditions in current response")
	}
}
