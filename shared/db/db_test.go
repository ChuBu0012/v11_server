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

	tables := []string{
		"event_logs", "health_samples", "alert_states", "watchers", "watcher_checks",
		"location_points", "location_battery_states", "location_geofences", "location_geofence_states",
		"location_colocation_state", "location_settings",
	}
	for _, tbl := range tables {
		var name string
		err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", tbl).Scan(&name)
		if err != nil {
			t.Errorf("Expected table %s to exist, got error: %v", tbl, err)
		}
	}

	var secretToken, viewerToken string
	if err := db.QueryRow("SELECT secret_token, viewer_token FROM location_settings WHERE id=1").Scan(&secretToken, &viewerToken); err != nil {
		t.Fatalf("read initial location settings: %v", err)
	}
	if secretToken != "" || viewerToken != "" {
		t.Fatalf("fresh database should not contain predictable location tokens: secret=%q viewer=%q", secretToken, viewerToken)
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

	// Verify all pool connections inherit busy_timeout from DSN
	for i := 0; i < 5; i++ {
		var timeout int
		if err := db.QueryRow("PRAGMA busy_timeout").Scan(&timeout); err != nil {
			t.Fatalf("Failed to read busy_timeout on connection %d: %v", i, err)
		}
		if timeout != 10000 {
			t.Errorf("Expected busy_timeout 10000, got %d on iteration %d", timeout, i)
		}
	}
}
