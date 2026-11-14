package telemetry

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// HealthSample represents a snapshot of the device and service health.
type HealthSample struct {
	ID            int64     `json:"id"`
	Timestamp     time.Time `json:"timestamp"`
	UptimeSeconds int64     `json:"uptime_seconds"`
	MemoryAllocMB float64   `json:"memory_alloc_mb"`
	MemorySysMB   float64   `json:"memory_sys_mb"`
	DiskTotalGB   float64   `json:"disk_total_gb"`
	DiskFreeGB    float64   `json:"disk_free_gb"`
	BatteryLevel  *float64  `json:"battery_level,omitempty"`
	BatteryStatus *string   `json:"battery_status,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// Repository handles persistence and querying of health samples in SQLite.
type Repository struct {
	db *sql.DB
}

// NewRepository creates a new telemetry repository.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// InsertSample persists a new health sample.
func (r *Repository) InsertSample(ctx context.Context, s HealthSample) (int64, error) {
	ts := s.Timestamp.UTC()
	if ts.IsZero() {
		ts = time.Now().UTC()
	}

	query := `
		INSERT INTO health_samples (
			timestamp, uptime_seconds, memory_alloc_mb, memory_sys_mb,
			disk_total_gb, disk_free_gb, battery_level, battery_status, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	res, err := r.db.ExecContext(ctx, query,
		ts.Format(time.RFC3339),
		s.UptimeSeconds,
		s.MemoryAllocMB,
		s.MemorySysMB,
		s.DiskTotalGB,
		s.DiskFreeGB,
		s.BatteryLevel,
		s.BatteryStatus,
		time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return 0, fmt.Errorf("telemetry: failed to insert sample: %w", err)
	}

	return res.LastInsertId()
}

// GetRecentSamples retrieves bounded samples ordered descending or ascending by timestamp.
// default limit: 60, max limit: 5000. The HTTP handler applies a smaller
// response bound after it has read enough samples to cover the requested window.
func (r *Repository) GetRecentSamples(ctx context.Context, limit int, maxAge time.Duration) ([]HealthSample, error) {
	if limit <= 0 {
		limit = 60
	}
	if limit > 5000 {
		limit = 5000
	}

	var query string
	var args []interface{}

	if maxAge > 0 {
		cutoff := time.Now().UTC().Add(-maxAge).Format(time.RFC3339)
		query = `
			SELECT id, timestamp, uptime_seconds, memory_alloc_mb, memory_sys_mb,
			       disk_total_gb, disk_free_gb, battery_level, battery_status, created_at
			FROM health_samples
			WHERE timestamp >= ?
			ORDER BY id DESC
			LIMIT ?
		`
		args = []interface{}{cutoff, limit}
	} else {
		query = `
			SELECT id, timestamp, uptime_seconds, memory_alloc_mb, memory_sys_mb,
			       disk_total_gb, disk_free_gb, battery_level, battery_status, created_at
			FROM health_samples
			ORDER BY id DESC
			LIMIT ?
		`
		args = []interface{}{limit}
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("telemetry: query samples failed: %w", err)
	}
	defer rows.Close()

	samples := make([]HealthSample, 0, limit)
	for rows.Next() {
		var s HealthSample
		var tsStr, caStr string
		err := rows.Scan(
			&s.ID,
			&tsStr,
			&s.UptimeSeconds,
			&s.MemoryAllocMB,
			&s.MemorySysMB,
			&s.DiskTotalGB,
			&s.DiskFreeGB,
			&s.BatteryLevel,
			&s.BatteryStatus,
			&caStr,
		)
		if err != nil {
			return nil, fmt.Errorf("telemetry: scan sample failed: %w", err)
		}
		s.Timestamp, _ = time.Parse(time.RFC3339, tsStr)
		s.CreatedAt, _ = time.Parse(time.RFC3339, caStr)
		samples = append(samples, s)
	}

	return samples, rows.Err()
}

// GetLatestSample returns the most recent sample, or nil if no samples exist.
func (r *Repository) GetLatestSample(ctx context.Context) (*HealthSample, error) {
	samples, err := r.GetRecentSamples(ctx, 1, 0)
	if err != nil {
		return nil, err
	}
	if len(samples) == 0 {
		return nil, nil
	}
	return &samples[0], nil
}

// PruneSamples removes samples older than maxAge or beyond maxRows.
// Returns the total number of deleted rows.
func (r *Repository) PruneSamples(ctx context.Context, maxAge time.Duration, maxRows int) (int64, error) {
	var totalDeleted int64

	// 1. Prune by maxAge if specified
	if maxAge > 0 {
		cutoff := time.Now().UTC().Add(-maxAge).Format(time.RFC3339)
		res, err := r.db.ExecContext(ctx, "DELETE FROM health_samples WHERE timestamp < ?", cutoff)
		if err != nil {
			return totalDeleted, fmt.Errorf("telemetry: prune by age failed: %w", err)
		}
		if rows, err := res.RowsAffected(); err == nil {
			totalDeleted += rows
		}
	}

	// 2. Prune by maxRows if specified
	if maxRows > 0 {
		query := `
			DELETE FROM health_samples
			WHERE id NOT IN (
				SELECT id FROM health_samples
				ORDER BY id DESC
				LIMIT ?
			)
		`
		res, err := r.db.ExecContext(ctx, query, maxRows)
		if err != nil {
			return totalDeleted, fmt.Errorf("telemetry: prune by max rows failed: %w", err)
		}
		if rows, err := res.RowsAffected(); err == nil {
			totalDeleted += rows
		}
	}

	return totalDeleted, nil
}
