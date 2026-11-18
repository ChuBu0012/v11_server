package db

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

func InitDB(dbPath string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil && filepath.Dir(dbPath) != "." {
		return nil, fmt.Errorf("failed to create db dir: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite: %w", err)
	}

	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(time.Hour)

	pragmas := []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA busy_timeout=5000;",
		"PRAGMA synchronous=NORMAL;",
		"PRAGMA foreign_keys=ON;",
	}
	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			log.Printf("Warning setting pragma %q: %v", pragma, err)
		}
	}

	createTableQuery := `
	CREATE TABLE IF NOT EXISTS event_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		level TEXT NOT NULL,
		message TEXT NOT NULL,
		metadata TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_event_logs_created_at ON event_logs(created_at DESC);

	CREATE TABLE IF NOT EXISTS health_samples (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME NOT NULL,
		uptime_seconds INTEGER NOT NULL,
		memory_alloc_mb REAL NOT NULL,
		memory_sys_mb REAL NOT NULL,
		disk_total_gb REAL NOT NULL,
		disk_free_gb REAL NOT NULL,
		battery_level REAL,
		battery_status TEXT,
		host_memory_total_mb REAL,
		host_memory_used_mb REAL,
		host_memory_free_mb REAL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_health_samples_timestamp ON health_samples(timestamp DESC);

	CREATE TABLE IF NOT EXISTS alert_states (
		condition_type TEXT PRIMARY KEY,
		active BOOLEAN NOT NULL,
		triggered_at DATETIME NOT NULL,
		last_checked_at DATETIME NOT NULL,
		value_at_trigger REAL NOT NULL
	);

	CREATE TABLE IF NOT EXISTS watchers (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		url TEXT NOT NULL,
		expected_status INTEGER NOT NULL DEFAULT 200,
		enabled INTEGER NOT NULL DEFAULT 1,
		state TEXT NOT NULL DEFAULT 'unknown',
		consecutive_failures INTEGER NOT NULL DEFAULT 0,
		down_since DATETIME,
		last_checked_at DATETIME,
		last_status_code INTEGER,
		last_latency_ms INTEGER,
		last_error TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_watchers_enabled ON watchers(enabled);

	CREATE TABLE IF NOT EXISTS watcher_checks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		watcher_id INTEGER NOT NULL REFERENCES watchers(id) ON DELETE CASCADE,
		checked_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		success BOOLEAN NOT NULL,
		status_code INTEGER,
		latency_ms INTEGER,
		error TEXT,
		attempt_count INTEGER NOT NULL DEFAULT 1
	);
	CREATE INDEX IF NOT EXISTS idx_watcher_checks_watcher_id_checked_at ON watcher_checks(watcher_id, checked_at DESC);
	CREATE INDEX IF NOT EXISTS idx_watcher_checks_checked_at ON watcher_checks(checked_at DESC);
	`
	if _, err := db.Exec(createTableQuery); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create tables: %w", err)
	}

	// Apply migrations for columns that may not exist in pre-existing databases
	migrations := []string{
		"ALTER TABLE health_samples ADD COLUMN host_memory_total_mb REAL;",
		"ALTER TABLE health_samples ADD COLUMN host_memory_used_mb REAL;",
		"ALTER TABLE health_samples ADD COLUMN host_memory_free_mb REAL;",
	}
	for _, migration := range migrations {
		_, _ = db.Exec(migration)
	}

	return db, nil
}
