package watchtower

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	// Open in-memory sqlite instance
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open in-memory sqlite: %v", err)
	}

	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys = ON;"); err != nil {
		t.Fatalf("Failed to set foreign_keys pragma: %v", err)
	}

	return db
}

func TestStore_WatcherCRUD(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	store, err := NewStore(db)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	ctx := context.Background()

	// 1. Create Watcher
	input := CreateWatcherInput{
		Name:           "Todo FM",
		URL:            "https://todo-fm-d.vercel.app/",
		ExpectedStatus: 200,
	}
	w, err := store.CreateWatcher(ctx, input)
	if err != nil {
		t.Fatalf("CreateWatcher failed: %v", err)
	}

	if w.ID <= 0 {
		t.Fatalf("Expected positive ID, got %d", w.ID)
	}
	if w.Name != "Todo FM" || w.ExpectedStatus != 200 || !w.Enabled || w.State != StateUnknown {
		t.Fatalf("Unexpected watcher fields: %+v", w)
	}

	// 2. Get Watcher
	fetched, err := store.GetWatcher(ctx, w.ID)
	if err != nil {
		t.Fatalf("GetWatcher failed: %v", err)
	}
	if fetched.ID != w.ID || fetched.URL != w.URL {
		t.Fatalf("Fetched watcher mismatch: %+v vs %+v", fetched, w)
	}

	// 3. Update Watcher
	newName := "Todo FM Updated"
	newStatus := 204
	updated, err := store.UpdateWatcher(ctx, w.ID, UpdateWatcherInput{
		Name:           &newName,
		ExpectedStatus: &newStatus,
	})
	if err != nil {
		t.Fatalf("UpdateWatcher failed: %v", err)
	}
	if updated.Name != newName || updated.ExpectedStatus != newStatus {
		t.Fatalf("Updated watcher fields mismatch: %+v", updated)
	}

	// 4. Update Watcher State
	now := time.Now().UTC()
	err = store.UpdateWatcherState(ctx, w.ID, StateDown, 3, &now)
	if err != nil {
		t.Fatalf("UpdateWatcherState failed: %v", err)
	}

	stateCheck, err := store.GetWatcher(ctx, w.ID)
	if err != nil || stateCheck.State != StateDown || stateCheck.ConsecutiveFailures != 3 || stateCheck.DownSince == nil {
		t.Fatalf("State check failed: %+v (err: %v)", stateCheck, err)
	}

	// 5. Reset Baseline
	err = store.ResetWatcherBaseline(ctx, w.ID)
	if err != nil {
		t.Fatalf("ResetWatcherBaseline failed: %v", err)
	}
	baselineCheck, err := store.GetWatcher(ctx, w.ID)
	if err != nil || baselineCheck.State != StateUnknown || baselineCheck.ConsecutiveFailures != 0 || baselineCheck.DownSince != nil {
		t.Fatalf("Baseline reset failed: %+v", baselineCheck)
	}

	// 6. Pause and Resume
	if err := store.PauseWatcher(ctx, w.ID); err != nil {
		t.Fatalf("PauseWatcher failed: %v", err)
	}
	pausedCheck, _ := store.GetWatcher(ctx, w.ID)
	if pausedCheck.Enabled || pausedCheck.State != StatePaused {
		t.Fatalf("Pause check failed: %+v", pausedCheck)
	}

	if err := store.ResumeWatcher(ctx, w.ID); err != nil {
		t.Fatalf("ResumeWatcher failed: %v", err)
	}
	resumedCheck, _ := store.GetWatcher(ctx, w.ID)
	if !resumedCheck.Enabled || resumedCheck.State != StateUnknown {
		t.Fatalf("Resume check failed: %+v", resumedCheck)
	}
}

func TestStore_MaxWatchersQuota(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	store, err := NewStore(db)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	ctx := context.Background()

	// Insert 25 watchers (allowed maximum)
	for i := 1; i <= MaxWatchersLimit; i++ {
		_, err := store.CreateWatcher(ctx, CreateWatcherInput{
			Name:           fmt.Sprintf("Watcher %d", i),
			URL:            fmt.Sprintf("http://93.184.216.34/watcher/%d", i),
			ExpectedStatus: 200,
		})
		if err != nil {
			t.Fatalf("Failed creating watcher #%d: %v", i, err)
		}
	}

	// 26th watcher must fail with ErrMaxWatchersReached
	_, err = store.CreateWatcher(ctx, CreateWatcherInput{
		Name:           "Watcher 26",
		URL:            "http://93.184.216.34/watcher/26",
		ExpectedStatus: 200,
	})
	if err != ErrMaxWatchersReached {
		t.Fatalf("Expected ErrMaxWatchersReached on 26th watcher, got %v", err)
	}
}

func TestStore_CascadingDelete(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	store, err := NewStore(db)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	ctx := context.Background()

	w, err := store.CreateWatcher(ctx, CreateWatcherInput{
		Name: "Cascade Target",
		URL:  "https://todo-fm-d.vercel.app/",
	})
	if err != nil {
		t.Fatalf("CreateWatcher failed: %v", err)
	}

	// Insert checks for this watcher
	for i := 0; i < 5; i++ {
		res := CheckResult{
			CheckedAt:    time.Now().UTC(),
			Success:      true,
			StatusCode:   200,
			LatencyMs:    15,
			AttemptCount: 1,
		}
		if err := store.RecordCheck(ctx, w.ID, res); err != nil {
			t.Fatalf("RecordCheck failed: %v", err)
		}
	}

	checks, err := store.ListChecks(ctx, w.ID, 10)
	if err != nil || len(checks) != 5 {
		t.Fatalf("Expected 5 checks, got %d (err: %v)", len(checks), err)
	}

	// Delete watcher
	if err := store.DeleteWatcher(ctx, w.ID); err != nil {
		t.Fatalf("DeleteWatcher failed: %v", err)
	}

	// Checks must be cascaded
	checksAfter, err := store.ListChecks(ctx, w.ID, 10)
	if err != nil || len(checksAfter) != 0 {
		t.Fatalf("Expected 0 checks after deletion, got %d (err: %v)", len(checksAfter), err)
	}
}

func TestStore_PurgeOldChecks(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	store, err := NewStore(db)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	ctx := context.Background()

	w, err := store.CreateWatcher(ctx, CreateWatcherInput{
		Name: "Purge Target",
		URL:  "https://todo-fm-d.vercel.app/",
	})
	if err != nil {
		t.Fatalf("CreateWatcher failed: %v", err)
	}

	// Insert old check (10 days ago) directly into SQLite
	oldTime := time.Now().UTC().Add(-10 * 24 * time.Hour)
	_, err = db.Exec(
		"INSERT INTO watcher_checks (watcher_id, checked_at, success, status_code, latency_ms, attempt_count) VALUES (?, ?, 1, 200, 20, 1)",
		w.ID, oldTime,
	)
	if err != nil {
		t.Fatalf("Failed to insert old check: %v", err)
	}

	// Insert recent check (1 day ago)
	recentTime := time.Now().UTC().Add(-1 * 24 * time.Hour)
	_, err = db.Exec(
		"INSERT INTO watcher_checks (watcher_id, checked_at, success, status_code, latency_ms, attempt_count) VALUES (?, ?, 1, 200, 20, 1)",
		w.ID, recentTime,
	)
	if err != nil {
		t.Fatalf("Failed to insert recent check: %v", err)
	}

	// Purge checks older than 7 days
	purged, err := store.PurgeOldChecks(ctx, 7)
	if err != nil {
		t.Fatalf("PurgeOldChecks failed: %v", err)
	}
	if purged != 1 {
		t.Fatalf("Expected exactly 1 check purged, got %d", purged)
	}

	remaining, err := store.ListChecks(ctx, w.ID, 10)
	if err != nil || len(remaining) != 1 {
		t.Fatalf("Expected 1 check remaining, got %d (err: %v)", len(remaining), err)
	}
}

func TestStore_GetSummary(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	store, err := NewStore(db)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	ctx := context.Background()

	// Initial summary with 0 watchers
	sum, err := store.GetSummary(ctx)
	if err != nil {
		t.Fatalf("GetSummary failed: %v", err)
	}
	if sum.Total != 0 || sum.Status != "healthy" || sum.Vantage != DefaultTargetLabel {
		t.Fatalf("Unexpected empty summary: %+v", sum)
	}

	// Add 1 up, 1 down, 1 paused
	w1, _ := store.CreateWatcher(ctx, CreateWatcherInput{Name: "W1", URL: "http://93.184.216.34/w1"})
	_ = store.UpdateWatcherState(ctx, w1.ID, StateUp, 0, nil)

	w2, _ := store.CreateWatcher(ctx, CreateWatcherInput{Name: "W2", URL: "http://93.184.216.34/w2"})
	_ = store.UpdateWatcherState(ctx, w2.ID, StateDown, 3, nil)

	w3, _ := store.CreateWatcher(ctx, CreateWatcherInput{Name: "W3", URL: "http://93.184.216.34/w3"})
	_ = store.PauseWatcher(ctx, w3.ID)

	sumAfter, err := store.GetSummary(ctx)
	if err != nil {
		t.Fatalf("GetSummary failed: %v", err)
	}

	if sumAfter.Total != 3 || sumAfter.Up != 1 || sumAfter.Down != 1 || sumAfter.Paused != 1 {
		t.Fatalf("Summary counts mismatch: %+v", sumAfter)
	}
	if sumAfter.Status != "degraded" {
		t.Fatalf("Expected status 'degraded' when 1 watcher is down, got %s", sumAfter.Status)
	}
}
