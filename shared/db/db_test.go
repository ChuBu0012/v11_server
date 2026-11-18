package db

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitDB(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "v11-db-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")
	db, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer db.Close()

	tables := []string{"event_logs", "health_samples", "alert_states", "watchers", "watcher_checks"}
	for _, tbl := range tables {
		var name string
		err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", tbl).Scan(&name)
		if err != nil {
			t.Errorf("Expected table %s to exist, got error: %v", tbl, err)
		}
	}

	// Test foreign keys and cascading delete on watchers -> watcher_checks
	res, err := db.Exec("INSERT INTO watchers (name, url, expected_status) VALUES ('Test Service', 'https://example.com', 200)")
	if err != nil {
		t.Fatalf("Failed to insert watcher: %v", err)
	}
	watcherID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("Failed to get watcher last insert id: %v", err)
	}

	_, err = db.Exec("INSERT INTO watcher_checks (watcher_id, success, status_code) VALUES (?, 1, 200)", watcherID)
	if err != nil {
		t.Fatalf("Failed to insert watcher check: %v", err)
	}

	var checkCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM watcher_checks WHERE watcher_id=?", watcherID).Scan(&checkCount); err != nil || checkCount != 1 {
		t.Fatalf("Expected 1 watcher check, got %d (err: %v)", checkCount, err)
	}

	// Delete watcher
	if _, err := db.Exec("DELETE FROM watchers WHERE id=?", watcherID); err != nil {
		t.Fatalf("Failed to delete watcher: %v", err)
	}

	// Cascade should have removed the check
	if err := db.QueryRow("SELECT COUNT(*) FROM watcher_checks WHERE watcher_id=?", watcherID).Scan(&checkCount); err != nil || checkCount != 0 {
		t.Errorf("Expected 0 watcher checks after cascade delete, got %d (err: %v)", checkCount, err)
	}
}
