package db

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

func InitDB(dbPath string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil && filepath.Dir(dbPath) != "." {
		return nil, fmt.Errorf("failed to create db dir: %w", err)
	}

	dsn := dbPath
	if !strings.Contains(dsn, "_pragma=") {
		sep := "?"
		if strings.Contains(dsn, "?") {
			sep = "&"
		}
		dsn = fmt.Sprintf("%s%s_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)&_pragma=cache_size(-8000)&_pragma=temp_store(MEMORY)&_pragma=mmap_size(268435456)", dsn, sep)
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite: %w", err)
	}

	// Keep idle connections alive to reuse WAL readers on a single-core ARM64 device.
	db.SetMaxOpenConns(3)
	db.SetMaxIdleConns(3)
	db.SetConnMaxLifetime(time.Hour)

	pragmas := []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA busy_timeout=10000;",
		"PRAGMA synchronous=NORMAL;",
		"PRAGMA foreign_keys=ON;",
		// 8 MB page cache — reduces I/O on the low-RAM ARM device.
		"PRAGMA cache_size=-8000;",
		// Memory-mapped I/O for read-heavy workloads (256 MB ceiling).
		"PRAGMA mmap_size=268435456;",
		// Use memory for temporary tables to avoid disk I/O.
		"PRAGMA temp_store=MEMORY;",
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

	CREATE TABLE IF NOT EXISTS location_points (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		device_id TEXT NOT NULL,
		latitude REAL NOT NULL,
		longitude REAL NOT NULL,
		accuracy REAL NOT NULL DEFAULT 0,
		speed REAL NOT NULL DEFAULT 0,
		altitude REAL NOT NULL DEFAULT 0,
		course REAL NOT NULL DEFAULT 0,
		battery_level INTEGER NOT NULL DEFAULT 0,
		battery_status INTEGER NOT NULL DEFAULT 0,
		recorded_at DATETIME NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_location_points_device_recorded ON location_points(device_id, recorded_at DESC);
	CREATE INDEX IF NOT EXISTS idx_location_points_recorded_at ON location_points(recorded_at DESC);

	CREATE TABLE IF NOT EXISTS location_battery_states (
		device_id TEXT PRIMARY KEY,
		is_critical BOOLEAN NOT NULL DEFAULT 0,
		last_alert_at DATETIME NOT NULL DEFAULT '1970-01-01 00:00:00',
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS location_geofences (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		latitude REAL NOT NULL,
		longitude REAL NOT NULL,
		radius_meters REAL NOT NULL DEFAULT 150.0,
		notify_enter BOOLEAN NOT NULL DEFAULT 1,
		notify_exit BOOLEAN NOT NULL DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS location_geofence_states (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		device_id TEXT NOT NULL,
		geofence_id INTEGER NOT NULL REFERENCES location_geofences(id) ON DELETE CASCADE,
		is_inside BOOLEAN NOT NULL DEFAULT 0,
		last_transition_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(device_id, geofence_id)
	);

	CREATE TABLE IF NOT EXISTS location_colocation_state (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		is_colocated BOOLEAN NOT NULL DEFAULT 0,
		last_distance_meters REAL NOT NULL DEFAULT 0,
		last_transition_at DATETIME NOT NULL DEFAULT '1970-01-01 00:00:00',
		last_notified_at DATETIME NOT NULL DEFAULT '1970-01-01 00:00:00',
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	INSERT OR IGNORE INTO location_colocation_state (id, is_colocated, last_distance_meters, last_transition_at, last_notified_at)
	VALUES (1, 0, 0.0, '1970-01-01 00:00:00', '1970-01-01 00:00:00');

	CREATE TABLE IF NOT EXISTS location_settings (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		secret_token TEXT NOT NULL DEFAULT '',
		viewer_token TEXT NOT NULL DEFAULT '',
		notify_colocation BOOLEAN NOT NULL DEFAULT 1,
		notify_geofence BOOLEAN NOT NULL DEFAULT 1,
		notify_battery BOOLEAN NOT NULL DEFAULT 1,
		colocation_enter_meters REAL NOT NULL DEFAULT 80.0,
		colocation_exit_meters REAL NOT NULL DEFAULT 250.0,
		cooldown_seconds INTEGER NOT NULL DEFAULT 300,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	INSERT OR IGNORE INTO location_settings (id) VALUES (1);
	UPDATE location_settings SET secret_token = '' WHERE secret_token = 'sentinel-loc-secret';
	UPDATE location_settings SET viewer_token = '' WHERE viewer_token = 'live-shared-token';

	CREATE TABLE IF NOT EXISTS ai_quota_states (
		provider TEXT NOT NULL,
		group_name TEXT NOT NULL,
		bucket_id TEXT NOT NULL,
		display_name TEXT NOT NULL,
		window TEXT NOT NULL,
		used_percent REAL NOT NULL,
		remaining_fraction REAL NOT NULL,
		reset_time DATETIME NOT NULL,
		last_checked_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (provider, bucket_id)
	);

	CREATE TABLE IF NOT EXISTS ai_quota_snapshots (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		provider TEXT NOT NULL,
		group_name TEXT NOT NULL,
		bucket_id TEXT NOT NULL,
		remaining_fraction REAL NOT NULL,
		reset_time DATETIME NOT NULL,
		recorded_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_ai_quota_snapshots_bucket_time 
	ON ai_quota_snapshots(bucket_id, recorded_at DESC);
	CREATE INDEX IF NOT EXISTS idx_ai_quota_snapshots_recorded_at
	ON ai_quota_snapshots(recorded_at DESC);

	CREATE TABLE IF NOT EXISTS ask_cache (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		prompt_hash TEXT NOT NULL UNIQUE,
		prompt_text TEXT NOT NULL,
		model_name TEXT NOT NULL,
		response_text TEXT NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		expires_at DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_ask_cache_hash ON ask_cache(prompt_hash);
	CREATE INDEX IF NOT EXISTS idx_ask_cache_expires ON ask_cache(expires_at);

	CREATE TABLE IF NOT EXISTS notes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		title TEXT NOT NULL DEFAULT '',
		content TEXT NOT NULL DEFAULT '',
		kind TEXT NOT NULL DEFAULT 'note',
		items_json TEXT NOT NULL DEFAULT '[]',
		tags TEXT NOT NULL DEFAULT '',
		pinned INTEGER NOT NULL DEFAULT 0,
		archived INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_notes_pinned_updated ON notes(archived, pinned DESC, updated_at DESC);
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
