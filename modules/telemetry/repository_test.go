package telemetry

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"v11-server/shared/db"
)

func setupTestDB(t *testing.T) (*Repository, func()) {
	t.Helper()
	tempDir, err := os.MkdirTemp("", "v11-telemetry-test-*")
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
	cleanup := func() {
		dbConn.Close()
		os.RemoveAll(tempDir)
	}

	return repo, cleanup
}

func TestInsertAndGetRecentSamples(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// 1. Initially empty
	latest, err := repo.GetLatestSample(ctx)
	if err != nil {
		t.Fatalf("Unexpected error querying latest: %v", err)
	}
	if latest != nil {
		t.Errorf("Expected nil latest sample on empty table, got %+v", latest)
	}

	// 2. Insert sample with battery
	batLevel := 75.0
	batStatus := "DISCHARGING"
	s1 := HealthSample{
		Timestamp:     time.Now().UTC().Add(-2 * time.Minute),
		UptimeSeconds: 120,
		MemoryAllocMB: 1.5,
		MemorySysMB:   8.0,
		DiskTotalGB:   100.0,
		DiskFreeGB:    60.0,
		BatteryLevel:  &batLevel,
		BatteryStatus: &batStatus,
	}

	id1, err := repo.InsertSample(ctx, s1)
	if err != nil {
		t.Fatalf("Failed to insert sample 1: %v", err)
	}
	if id1 <= 0 {
		t.Errorf("Expected positive id, got %d", id1)
	}

	// 3. Insert sample without battery (unavailable)
	s2 := HealthSample{
		Timestamp:     time.Now().UTC(),
		UptimeSeconds: 180,
		MemoryAllocMB: 1.8,
		MemorySysMB:   8.5,
		DiskTotalGB:   100.0,
		DiskFreeGB:    59.8,
		BatteryLevel:  nil,
		BatteryStatus: nil,
	}

	id2, err := repo.InsertSample(ctx, s2)
	if err != nil {
		t.Fatalf("Failed to insert sample 2: %v", err)
	}
	if id2 <= id1 {
		t.Errorf("Expected id2 > id1, got id1=%d, id2=%d", id1, id2)
	}

	// 4. Query recent samples
	samples, err := repo.GetRecentSamples(ctx, 10, 0)
	if err != nil {
		t.Fatalf("Failed to get recent samples: %v", err)
	}
	if len(samples) != 2 {
		t.Fatalf("Expected 2 samples, got %d", len(samples))
	}

	// Ensure ordering is DESC (most recent first)
	if samples[0].ID != id2 {
		t.Errorf("Expected first sample to be latest id=%d, got id=%d", id2, samples[0].ID)
	}
	if samples[0].BatteryLevel != nil {
		t.Errorf("Expected nil battery level on sample 2, got %v", *samples[0].BatteryLevel)
	}
	if samples[1].BatteryLevel == nil || *samples[1].BatteryLevel != 75.0 {
		t.Errorf("Expected battery level 75.0 on sample 1, got %v", samples[1].BatteryLevel)
	}

	// 5. Test bounds: limit clamping
	samplesBound, err := repo.GetRecentSamples(ctx, 1, 0)
	if err != nil || len(samplesBound) != 1 {
		t.Errorf("Expected 1 sample with limit=1, got %d (err: %v)", len(samplesBound), err)
	}
}

func TestPruneSamples(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Insert 10 samples with timestamps 1 hour apart
	now := time.Now().UTC()
	for i := 10; i >= 1; i-- {
		s := HealthSample{
			Timestamp:     now.Add(-time.Duration(i) * time.Hour),
			UptimeSeconds: int64(1000 - i*100),
			MemoryAllocMB: 2.0,
			MemorySysMB:   10.0,
			DiskTotalGB:   100.0,
			DiskFreeGB:    50.0,
		}
		if _, err := repo.InsertSample(ctx, s); err != nil {
			t.Fatalf("Failed to insert sample %d: %v", i, err)
		}
	}

	// Initial count should be 10
	samples, _ := repo.GetRecentSamples(ctx, 50, 0)
	if len(samples) != 10 {
		t.Fatalf("Expected 10 samples, got %d", len(samples))
	}

	// Prune older than 5.5 hours -> samples 6, 7, 8, 9, 10 should be pruned (5 samples)
	deleted, err := repo.PruneSamples(ctx, 5*time.Hour+30*time.Minute, 0)
	if err != nil {
		t.Fatalf("PruneSamples failed: %v", err)
	}
	if deleted != 5 {
		t.Errorf("Expected 5 deleted samples by age, got %d", deleted)
	}

	samplesAfterAge, _ := repo.GetRecentSamples(ctx, 50, 0)
	if len(samplesAfterAge) != 5 {
		t.Errorf("Expected 5 remaining samples, got %d", len(samplesAfterAge))
	}

	// Prune by maxRows: keep only 3
	deletedRows, err := repo.PruneSamples(ctx, 0, 3)
	if err != nil {
		t.Fatalf("PruneSamples by maxRows failed: %v", err)
	}
	if deletedRows != 2 {
		t.Errorf("Expected 2 deleted samples by maxRows, got %d", deletedRows)
	}

	samplesAfterRows, _ := repo.GetRecentSamples(ctx, 50, 0)
	if len(samplesAfterRows) != 3 {
		t.Errorf("Expected 3 remaining samples, got %d", len(samplesAfterRows))
	}
}
