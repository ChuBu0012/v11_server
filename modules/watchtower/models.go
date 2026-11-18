package watchtower

import (
	"errors"
	"time"
)

// Standard constants for Personal Watchtower.
const (
	StateUnknown = "unknown"
	StateUp      = "up"
	StateDown    = "down"
	StatePaused  = "paused"

	DefaultTargetURL        = "https://todo-fm-d.vercel.app/"
	DefaultTargetName       = "Todo FM"
	DefaultTargetLabel      = "seen from V11"
	DefaultExpectedStatus   = 200
	MaxWatchersLimit        = 25
	MaxConcurrency          = 3
	RetentionDays           = 7
	FailureStreakThreshold  = 3
	CheckTimeout            = 10 * time.Second
	MaxResponseBodyLimit    = 64 * 1024 // 64 KB
	MaxRetriesPerCheckCycle = 3
)

// Common errors.
var (
	ErrWatcherNotFound    = errors.New("watcher not found")
	ErrMaxWatchersReached = errors.New("maximum watchers limit reached (max 25)")
	ErrInvalidURL         = errors.New("invalid target url")
	ErrInvalidName        = errors.New("name must be between 1 and 80 characters")
	ErrInvalidStatus      = errors.New("expected status must be between 100 and 599")
	ErrPinNotConfigured   = errors.New("watchtower mutations unavailable: WATCHTOWER_PIN not configured")
	ErrUnauthorized       = errors.New("unauthorized: invalid watchtower pin")
)

// Watcher represents a monitored public service endpoint.
type Watcher struct {
	ID                  int64      `json:"id"`
	Name                string     `json:"name"`
	URL                 string     `json:"url"`
	ExpectedStatus      int        `json:"expected_status"`
	Enabled             bool       `json:"enabled"`
	State               string     `json:"state"` // 'unknown', 'up', 'down', 'paused'
	ConsecutiveFailures int        `json:"consecutive_failures"`
	DownSince           *time.Time `json:"down_since,omitempty"`
	LastCheckedAt       *time.Time `json:"last_checked_at,omitempty"`
	LastStatusCode      *int       `json:"last_status_code,omitempty"`
	LastLatencyMs       *int64     `json:"last_latency_ms,omitempty"`
	LastError           *string    `json:"last_error,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

// WatcherCheck represents a single check execution record.
type WatcherCheck struct {
	ID           int64     `json:"id"`
	WatcherID    int64     `json:"watcher_id"`
	CheckedAt    time.Time `json:"checked_at"`
	Success      bool      `json:"success"`
	StatusCode   int       `json:"status_code"`
	LatencyMs    int64     `json:"latency_ms"`
	Error        string    `json:"error,omitempty"`
	AttemptCount int       `json:"attempt_count"`
}

// CheckResult represents the outcome of an HTTP check probe.
type CheckResult struct {
	CheckedAt    time.Time `json:"checked_at"`
	Success      bool      `json:"success"`
	StatusCode   int       `json:"status_code"`
	LatencyMs    int64     `json:"latency_ms"`
	Error        string    `json:"error,omitempty"`
	AttemptCount int       `json:"attempt_count"`
}

// WatchtowerSummary represents the aggregated health state of all watchers.
type WatchtowerSummary struct {
	Total   int    `json:"total"`
	Up      int    `json:"up"`
	Down    int    `json:"down"`
	Paused  int    `json:"paused"`
	Status  string `json:"status"`  // 'healthy', 'degraded', 'unknown'
	Vantage string `json:"vantage"` // 'seen from V11'
}

// CreateWatcherInput represents payload to create a new watcher.
type CreateWatcherInput struct {
	Name           string `json:"name"`
	URL            string `json:"url"`
	ExpectedStatus int    `json:"expected_status"`
}

// UpdateWatcherInput represents payload to update an existing watcher.
type UpdateWatcherInput struct {
	Name           *string `json:"name,omitempty"`
	URL            *string `json:"url,omitempty"`
	ExpectedStatus *int    `json:"expected_status,omitempty"`
	Enabled        *bool   `json:"enabled,omitempty"`
}

// DDL queries for SQLite table and index creation.
const (
	DDLWatchers = `
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
`

	DDLWatcherChecks = `
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
)
