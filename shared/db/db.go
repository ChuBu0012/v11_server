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
	`
	if _, err := db.Exec(createTableQuery); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create tables: %w", err)
	}

	return db, nil
}
