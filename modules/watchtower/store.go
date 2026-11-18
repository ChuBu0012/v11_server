package watchtower

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Store handles SQLite persistence for Personal Watchtower.
type Store struct {
	db        *sql.DB
	validator *URLValidator
}

// NewStore initializes the Store and applies required DDL migrations.
func NewStore(db *sql.DB) (*Store, error) {
	if db == nil {
		return nil, errors.New("db cannot be nil")
	}

	// Ensure foreign keys and WAL pragmas are enabled
	pragmas := []string{
		"PRAGMA foreign_keys = ON;",
		"PRAGMA busy_timeout = 5000;",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			// Non-fatal warning if pragma fails
			_ = err
		}
	}

	s := &Store{
		db:        db,
		validator: NewURLValidator(),
	}

	if err := s.InitSchema(context.Background()); err != nil {
		return nil, fmt.Errorf("init watchtower schema failed: %w", err)
	}

	return s, nil
}

// InitSchema creates tables and indexes if they do not exist.
func (s *Store) InitSchema(ctx context.Context) error {
	queries := []string{
		DDLWatchers,
		DDLWatcherChecks,
	}

	for _, q := range queries {
		if _, err := s.db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("exec schema query failed: %w", err)
		}
	}

	return nil
}

// CountWatchers returns total number of watchers currently configured.
func (s *Store) CountWatchers(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM watchers").Scan(&count)
	return count, err
}

// CreateWatcher validates input, enforces the max 25 quota, and persists a new watcher.
func (s *Store) CreateWatcher(ctx context.Context, input CreateWatcherInput) (*Watcher, error) {
	name := strings.TrimSpace(input.Name)
	if len(name) < 1 || len(name) > 80 {
		return nil, ErrInvalidName
	}

	expectedStatus := input.ExpectedStatus
	if expectedStatus == 0 {
		expectedStatus = DefaultExpectedStatus
	}
	if expectedStatus < 100 || expectedStatus > 599 {
		return nil, ErrInvalidStatus
	}

	// Validate public URL syntax and SSRF safety
	if _, err := s.validator.ValidateURL(input.URL); err != nil {
		return nil, err
	}

	// Enforce quota limit (max 25 watchers)
	count, err := s.CountWatchers(ctx)
	if err != nil {
		return nil, fmt.Errorf("check watcher count failed: %w", err)
	}
	if count >= MaxWatchersLimit {
		return nil, ErrMaxWatchersReached
	}

	now := time.Now().UTC()
	query := `
		INSERT INTO watchers (
			name, url, expected_status, enabled, state,
			consecutive_failures, created_at, updated_at
		) VALUES (?, ?, ?, 1, ?, 0, ?, ?)
	`
	res, err := s.db.ExecContext(ctx, query, name, input.URL, expectedStatus, StateUnknown, now, now)
	if err != nil {
		return nil, fmt.Errorf("insert watcher failed: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}

	return s.GetWatcher(ctx, id)
}

// GetWatcher retrieves a single watcher by ID.
func (s *Store) GetWatcher(ctx context.Context, id int64) (*Watcher, error) {
	query := `
		SELECT
			id, name, url, expected_status, enabled, state,
			consecutive_failures, down_since, last_checked_at,
			last_status_code, last_latency_ms, last_error,
			created_at, updated_at
		FROM watchers
		WHERE id = ?
	`
	row := s.db.QueryRowContext(ctx, query, id)

	var w Watcher
	var enabledInt int
	var downSince, lastCheckedAt sql.NullTime
	var lastStatusCode sql.NullInt64
	var lastLatencyMs sql.NullInt64
	var lastError sql.NullString

	err := row.Scan(
		&w.ID, &w.Name, &w.URL, &w.ExpectedStatus, &enabledInt, &w.State,
		&w.ConsecutiveFailures, &downSince, &lastCheckedAt,
		&lastStatusCode, &lastLatencyMs, &lastError,
		&w.CreatedAt, &w.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrWatcherNotFound
		}
		return nil, err
	}

	w.Enabled = enabledInt == 1
	if downSince.Valid {
		t := downSince.Time.UTC()
		w.DownSince = &t
	}
	if lastCheckedAt.Valid {
		t := lastCheckedAt.Time.UTC()
		w.LastCheckedAt = &t
	}
	if lastStatusCode.Valid {
		c := int(lastStatusCode.Int64)
		w.LastStatusCode = &c
	}
	if lastLatencyMs.Valid {
		l := lastLatencyMs.Int64
		w.LastLatencyMs = &l
	}
	if lastError.Valid {
		e := lastError.String
		w.LastError = &e
	}

	return &w, nil
}

// ListWatchers retrieves all watchers sorted by ID ascending.
func (s *Store) ListWatchers(ctx context.Context) ([]Watcher, error) {
	query := `
		SELECT
			id, name, url, expected_status, enabled, state,
			consecutive_failures, down_since, last_checked_at,
			last_status_code, last_latency_ms, last_error,
			created_at, updated_at
		FROM watchers
		ORDER BY id ASC
	`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Watcher
	for rows.Next() {
		var w Watcher
		var enabledInt int
		var downSince, lastCheckedAt sql.NullTime
		var lastStatusCode sql.NullInt64
		var lastLatencyMs sql.NullInt64
		var lastError sql.NullString

		if err := rows.Scan(
			&w.ID, &w.Name, &w.URL, &w.ExpectedStatus, &enabledInt, &w.State,
			&w.ConsecutiveFailures, &downSince, &lastCheckedAt,
			&lastStatusCode, &lastLatencyMs, &lastError,
			&w.CreatedAt, &w.UpdatedAt,
		); err != nil {
			return nil, err
		}

		w.Enabled = enabledInt == 1
		if downSince.Valid {
			t := downSince.Time.UTC()
			w.DownSince = &t
		}
		if lastCheckedAt.Valid {
			t := lastCheckedAt.Time.UTC()
			w.LastCheckedAt = &t
		}
		if lastStatusCode.Valid {
			c := int(lastStatusCode.Int64)
			w.LastStatusCode = &c
		}
		if lastLatencyMs.Valid {
			l := lastLatencyMs.Int64
			w.LastLatencyMs = &l
		}
		if lastError.Valid {
			e := lastError.String
			w.LastError = &e
		}

		list = append(list, w)
	}

	return list, rows.Err()
}

// UpdateWatcher updates mutable fields of a watcher.
// If URL or ExpectedStatus changes, caller should reset baseline.
func (s *Store) UpdateWatcher(ctx context.Context, id int64, input UpdateWatcherInput) (*Watcher, error) {
	w, err := s.GetWatcher(ctx, id)
	if err != nil {
		return nil, err
	}

	if input.Name != nil {
		trimmed := strings.TrimSpace(*input.Name)
		if len(trimmed) < 1 || len(trimmed) > 80 {
			return nil, ErrInvalidName
		}
		w.Name = trimmed
	}

	if input.ExpectedStatus != nil {
		if *input.ExpectedStatus < 100 || *input.ExpectedStatus > 599 {
			return nil, ErrInvalidStatus
		}
		w.ExpectedStatus = *input.ExpectedStatus
	}

	if input.URL != nil {
		if _, err := s.validator.ValidateURL(*input.URL); err != nil {
			return nil, err
		}
		w.URL = *input.URL
	}

	if input.Enabled != nil {
		w.Enabled = *input.Enabled
		if !w.Enabled {
			w.State = StatePaused
		}
	}

	now := time.Now().UTC()
	enabledInt := 0
	if w.Enabled {
		enabledInt = 1
	}

	query := `
		UPDATE watchers SET
			name = ?,
			url = ?,
			expected_status = ?,
			enabled = ?,
			state = ?,
			updated_at = ?
		WHERE id = ?
	`
	_, err = s.db.ExecContext(ctx, query, w.Name, w.URL, w.ExpectedStatus, enabledInt, w.State, now, id)
	if err != nil {
		return nil, err
	}

	return s.GetWatcher(ctx, id)
}

// ResetWatcherBaseline resets the watcher state to unknown baseline and clears outage data.
// Used when editing URL or expected status. Never triggers recovery notifications.
func (s *Store) ResetWatcherBaseline(ctx context.Context, id int64) error {
	now := time.Now().UTC()
	query := `
		UPDATE watchers SET
			state = ?,
			consecutive_failures = 0,
			down_since = NULL,
			last_error = NULL,
			updated_at = ?
		WHERE id = ?
	`
	_, err := s.db.ExecContext(ctx, query, StateUnknown, now, id)
	return err
}

// UpdateWatcherState updates deterministic health state, failure streaks, and downtime.
func (s *Store) UpdateWatcherState(ctx context.Context, id int64, state string, consecutiveFailures int, downSince *time.Time) error {
	now := time.Now().UTC()
	var downSinceVal interface{}
	if downSince != nil {
		downSinceVal = downSince.UTC()
	}

	query := `
		UPDATE watchers SET
			state = ?,
			consecutive_failures = ?,
			down_since = ?,
			updated_at = ?
		WHERE id = ?
	`
	_, err := s.db.ExecContext(ctx, query, state, consecutiveFailures, downSinceVal, now, id)
	return err
}

// UpdateWatcherProbeMetrics updates the latest check telemetry without perturbing failure streaks.
func (s *Store) UpdateWatcherProbeMetrics(ctx context.Context, id int64, res CheckResult) error {
	now := time.Now().UTC()
	var errVal interface{}
	if res.Error != "" {
		errVal = SanitizeError(res.Error)
	}

	query := `
		UPDATE watchers SET
			last_checked_at = ?,
			last_status_code = ?,
			last_latency_ms = ?,
			last_error = ?,
			updated_at = ?
		WHERE id = ?
	`
	_, err := s.db.ExecContext(ctx, query, res.CheckedAt.UTC(), res.StatusCode, res.LatencyMs, errVal, now, id)
	return err
}

// PauseWatcher sets enabled=0 and state='paused'.
func (s *Store) PauseWatcher(ctx context.Context, id int64) error {
	now := time.Now().UTC()
	query := `
		UPDATE watchers SET
			enabled = 0,
			state = ?,
			updated_at = ?
		WHERE id = ?
	`
	res, err := s.db.ExecContext(ctx, query, StatePaused, now, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrWatcherNotFound
	}
	return nil
}

// ResumeWatcher sets enabled=1 and resets state to unknown baseline.
func (s *Store) ResumeWatcher(ctx context.Context, id int64) error {
	now := time.Now().UTC()
	query := `
		UPDATE watchers SET
			enabled = 1,
			state = ?,
			consecutive_failures = 0,
			down_since = NULL,
			updated_at = ?
		WHERE id = ?
	`
	res, err := s.db.ExecContext(ctx, query, StateUnknown, now, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrWatcherNotFound
	}
	return nil
}

// DeleteWatcher removes a watcher and cascades removal of check history.
func (s *Store) DeleteWatcher(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Explicitly remove checks to guarantee cleanup across all SQLite pragmas
	if _, err := tx.ExecContext(ctx, "DELETE FROM watcher_checks WHERE watcher_id = ?", id); err != nil {
		return err
	}

	res, err := tx.ExecContext(ctx, "DELETE FROM watchers WHERE id = ?", id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrWatcherNotFound
	}

	return tx.Commit()
}

// RecordCheck saves a probe result into watcher_checks.
func (s *Store) RecordCheck(ctx context.Context, watcherID int64, res CheckResult) error {
	var errVal interface{}
	if res.Error != "" {
		errVal = SanitizeError(res.Error)
	}

	query := `
		INSERT INTO watcher_checks (
			watcher_id, checked_at, success, status_code, latency_ms, error, attempt_count
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.ExecContext(
		ctx, query,
		watcherID, res.CheckedAt.UTC(), res.Success, res.StatusCode, res.LatencyMs, errVal, res.AttemptCount,
	)
	return err
}

// ListChecks retrieves up to limit checks for a given watcher, sorted newest first.
func (s *Store) ListChecks(ctx context.Context, watcherID int64, limit int) ([]WatcherCheck, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	query := `
		SELECT id, watcher_id, checked_at, success, status_code, latency_ms, error, attempt_count
		FROM watcher_checks
		WHERE watcher_id = ?
		ORDER BY checked_at DESC
		LIMIT ?
	`
	rows, err := s.db.QueryContext(ctx, query, watcherID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var checks []WatcherCheck
	for rows.Next() {
		var c WatcherCheck
		var successInt int
		var errStr sql.NullString

		if err := rows.Scan(
			&c.ID, &c.WatcherID, &c.CheckedAt, &successInt, &c.StatusCode, &c.LatencyMs, &errStr, &c.AttemptCount,
		); err != nil {
			return nil, err
		}

		c.Success = successInt == 1
		c.CheckedAt = c.CheckedAt.UTC()
		if errStr.Valid {
			c.Error = errStr.String
		}
		checks = append(checks, c)
	}

	return checks, rows.Err()
}

// PurgeOldChecks purges check records older than retentionDays (7 days).
func (s *Store) PurgeOldChecks(ctx context.Context, retentionDays int) (int64, error) {
	if retentionDays <= 0 {
		retentionDays = RetentionDays
	}

	query := `
		DELETE FROM watcher_checks
		WHERE checked_at < datetime('now', printf('-%d days', ?))
	`
	res, err := s.db.ExecContext(ctx, query, retentionDays)
	if err != nil {
		return 0, fmt.Errorf("purge old checks failed: %w", err)
	}

	return res.RowsAffected()
}

// GetSummary calculates aggregate metrics across all watchers.
func (s *Store) GetSummary(ctx context.Context) (*WatchtowerSummary, error) {
	query := `
		SELECT
			COUNT(*) as total,
			COALESCE(SUM(CASE WHEN state = 'up' THEN 1 ELSE 0 END), 0) as up_count,
			COALESCE(SUM(CASE WHEN state = 'down' THEN 1 ELSE 0 END), 0) as down_count,
			COALESCE(SUM(CASE WHEN state = 'paused' THEN 1 ELSE 0 END), 0) as paused_count
		FROM watchers
	`
	var total, up, down, paused int
	err := s.db.QueryRowContext(ctx, query).Scan(&total, &up, &down, &paused)
	if err != nil {
		return nil, err
	}

	status := "healthy"
	if down > 0 {
		status = "degraded"
	} else if total == 0 {
		status = "healthy"
	}

	return &WatchtowerSummary{
		Total:   total,
		Up:      up,
		Down:    down,
		Paused:  paused,
		Status:  status,
		Vantage: DefaultTargetLabel,
	}, nil
}
