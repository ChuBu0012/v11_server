package ailimit

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Repository manages persistence for AI quota states and historical snapshots.
type Repository struct {
	db *sql.DB
}

// NewRepository initializes a Repository with the shared database connection.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// SaveStates upserts the latest quota bucket states into ai_quota_states.
func (r *Repository) SaveStates(ctx context.Context, buckets []QuotaBucket) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx failed: %w", err)
	}
	defer tx.Rollback()

	query := `
	INSERT INTO ai_quota_states (
		provider, group_name, bucket_id, display_name, window,
		used_percent, remaining_fraction, reset_time, last_checked_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(provider, bucket_id) DO UPDATE SET
		group_name = excluded.group_name,
		display_name = excluded.display_name,
		window = excluded.window,
		used_percent = excluded.used_percent,
		remaining_fraction = excluded.remaining_fraction,
		reset_time = excluded.reset_time,
		last_checked_at = excluded.last_checked_at;
	`

	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return fmt.Errorf("prepare stmt failed: %w", err)
	}
	defer stmt.Close()

	for _, b := range buckets {
		resetTimeStr := b.ResetTime.UTC().Format(time.RFC3339)
		lastCheckedStr := b.LastCheckedAt.UTC().Format(time.RFC3339)

		if _, err := stmt.ExecContext(ctx,
			b.Provider, b.GroupName, b.BucketID, b.DisplayName, b.Window,
			b.UsedPercent, b.RemainingFraction, resetTimeStr, lastCheckedStr,
		); err != nil {
			return fmt.Errorf("exec upsert failed for %s: %w", b.BucketID, err)
		}
	}

	return tx.Commit()
}

// SaveSnapshots logs snapshot entries into ai_quota_snapshots for trends/graphs.
func (r *Repository) SaveSnapshots(ctx context.Context, buckets []QuotaBucket, recordedAt time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx failed: %w", err)
	}
	defer tx.Rollback()

	query := `
	INSERT INTO ai_quota_snapshots (
		provider, group_name, bucket_id, remaining_fraction, reset_time, recorded_at
	) VALUES (?, ?, ?, ?, ?, ?);
	`

	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return fmt.Errorf("prepare stmt failed: %w", err)
	}
	defer stmt.Close()

	recordedAtStr := recordedAt.UTC().Format(time.RFC3339)
	for _, b := range buckets {
		resetTimeStr := b.ResetTime.UTC().Format(time.RFC3339)
		if _, err := stmt.ExecContext(ctx,
			b.Provider, b.GroupName, b.BucketID, b.RemainingFraction, resetTimeStr, recordedAtStr,
		); err != nil {
			return fmt.Errorf("exec insert snapshot failed for %s: %w", b.BucketID, err)
		}
	}

	return tx.Commit()
}

// GetLatestStates returns all latest quota buckets.
func (r *Repository) GetLatestStates(ctx context.Context) ([]QuotaBucket, error) {
	query := `
	SELECT provider, group_name, bucket_id, display_name, window,
	       used_percent, remaining_fraction, reset_time, last_checked_at
	FROM ai_quota_states
	ORDER BY provider, group_name, window;
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query latest states failed: %w", err)
	}
	defer rows.Close()

	var buckets []QuotaBucket
	for rows.Next() {
		var b QuotaBucket
		var resetStr, lastCheckedStr string

		if err := rows.Scan(
			&b.Provider, &b.GroupName, &b.BucketID, &b.DisplayName, &b.Window,
			&b.UsedPercent, &b.RemainingFraction, &resetStr, &lastCheckedStr,
		); err != nil {
			return nil, fmt.Errorf("scan latest states row failed: %w", err)
		}

		b.RemainingPercent = b.RemainingFraction * 100.0
		b.IsLow = b.RemainingPercent <= 15.0

		if t, err := time.Parse(time.RFC3339, resetStr); err == nil {
			b.ResetTime = t
		}
		if t, err := time.Parse(time.RFC3339, lastCheckedStr); err == nil {
			b.LastCheckedAt = t
		}

		buckets = append(buckets, b)
	}

	return buckets, rows.Err()
}

// GetHistory returns historical snapshots within the specified hours.
func (r *Repository) GetHistory(ctx context.Context, hours int) ([]QuotaSnapshot, error) {
	if hours <= 0 {
		hours = 24
	}

	cutoff := time.Now().Add(-time.Duration(hours) * time.Hour).UTC().Format(time.RFC3339)
	query := `
	SELECT id, provider, group_name, bucket_id, remaining_fraction, reset_time, recorded_at
	FROM ai_quota_snapshots
	WHERE recorded_at >= ?
	ORDER BY recorded_at ASC;
	`

	rows, err := r.db.QueryContext(ctx, query, cutoff)
	if err != nil {
		return nil, fmt.Errorf("query history failed: %w", err)
	}
	defer rows.Close()

	var snapshots []QuotaSnapshot
	for rows.Next() {
		var s QuotaSnapshot
		var resetStr, recordedStr string

		if err := rows.Scan(
			&s.ID, &s.Provider, &s.GroupName, &s.BucketID,
			&s.RemainingFraction, &resetStr, &recordedStr,
		); err != nil {
			return nil, fmt.Errorf("scan snapshot row failed: %w", err)
		}

		s.RemainingPercent = s.RemainingFraction * 100.0
		if t, err := time.Parse(time.RFC3339, resetStr); err == nil {
			s.ResetTime = t
		}
		if t, err := time.Parse(time.RFC3339, recordedStr); err == nil {
			s.RecordedAt = t
		}

		snapshots = append(snapshots, s)
	}

	return snapshots, rows.Err()
}
