package alerts

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"v11-server/shared/db"
)

func setupTestAlertStore(t *testing.T) (*AlertStore, func()) {
	t.Helper()
	tempDir, err := os.MkdirTemp("", "v11-alert-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tempDir, "test.db")
	dbConn, err := db.InitDB(dbPath)
	if err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("Failed to init db: %v", err)
	}

	store := NewAlertStore(dbConn)
	cleanup := func() {
		dbConn.Close()
		os.RemoveAll(tempDir)
	}

	return store, cleanup
}

func TestAlertStorePersistenceAndRestart(t *testing.T) {
	store, cleanup := setupTestAlertStore(t)
	defer cleanup()

	now := time.Now().UTC().Truncate(time.Second)

	// 1. Initially empty
	states, err := store.LoadState()
	if err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}
	if len(states) != 0 {
		t.Errorf("Expected 0 states initially, got %d", len(states))
	}

	// 2. Save active LowBattery alert
	st := AlertState{
		ConditionType:  LowBattery,
		Active:         true,
		TriggeredAt:    now,
		LastCheckedAt:  now,
		ValueAtTrigger: 15.0,
	}
	if err := store.SaveState(LowBattery, st); err != nil {
		t.Fatalf("SaveState failed: %v", err)
	}

	// 3. Simulate process restart by reloading from DB
	loaded, err := store.LoadState()
	if err != nil {
		t.Fatalf("LoadState after restart failed: %v", err)
	}
	batState, exists := loaded[LowBattery]
	if !exists {
		t.Fatalf("Expected LowBattery state to exist in DB")
	}
	if !batState.Active {
		t.Errorf("Expected active=true, got %v", batState.Active)
	}
	if batState.ValueAtTrigger != 15.0 {
		t.Errorf("Expected ValueAtTrigger=15.0, got %f", batState.ValueAtTrigger)
	}

	// Verify that evaluating the same condition with the reloaded state suppresses duplicates
	cond := Condition{Type: LowBattery, Active: true, Value: 14.0, Threshold: 20.0}
	kind, _ := Transition(batState, cond, now.Add(time.Minute))
	if kind != NoTransition {
		t.Errorf("Expected duplicate alert to be suppressed after restart, got %v", kind)
	}

	// 4. Test ClearState upon recovery
	if err := store.ClearState(LowBattery); err != nil {
		t.Fatalf("ClearState failed: %v", err)
	}

	afterClear, _ := store.LoadState()
	if _, exists := afterClear[LowBattery]; exists {
		t.Errorf("Expected LowBattery to be cleared from DB")
	}
}
