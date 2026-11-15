package digest

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Store handles persistence for digest settings and execution history.
type Store struct {
	db *sql.DB
}

// NewStore creates a Store and ensures required tables exist.
func NewStore(db *sql.DB) (*Store, error) {
	s := &Store{db: db}
	if err := s.initTables(); err != nil {
		return nil, fmt.Errorf("digest store init failed: %w", err)
	}
	return s, nil
}

func (s *Store) initTables() error {
	schema := `
	CREATE TABLE IF NOT EXISTS digest_settings (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		enabled BOOLEAN NOT NULL DEFAULT 1,
		schedule TEXT NOT NULL DEFAULT '13:00',
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	INSERT OR IGNORE INTO digest_settings (id, enabled, schedule) VALUES (1, 1, '13:00');

	CREATE TABLE IF NOT EXISTS digest_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		started_at DATETIME NOT NULL,
		finished_at DATETIME,
		status TEXT NOT NULL,
		summary_md TEXT,
		error_message TEXT,
		items_count INTEGER DEFAULT 0
	);

	CREATE INDEX IF NOT EXISTS idx_digest_history_started ON digest_history(started_at DESC);
	`
	_, err := s.db.Exec(schema)
	return err
}

func parseSQLiteTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	formats := []string{
		"2006-01-02 15:04:05",
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// GetSettings retrieves the current automated digest settings.
func (s *Store) GetSettings(ctx context.Context) (*Settings, error) {
	row := s.db.QueryRowContext(ctx, "SELECT enabled, schedule, updated_at FROM digest_settings WHERE id = 1")
	var st Settings
	var enabledInt int
	var updatedStr string
	if err := row.Scan(&enabledInt, &st.Schedule, &updatedStr); err != nil {
		return nil, err
	}
	st.Enabled = (enabledInt == 1)
	st.UpdatedAt = parseSQLiteTime(updatedStr)
	return &st, nil
}

// SetEnabled updates the on/off state of the digest.
func (s *Store) SetEnabled(ctx context.Context, enabled bool) error {
	enabledInt := 0
	if enabled {
		enabledInt = 1
	}
	_, err := s.db.ExecContext(ctx, "UPDATE digest_settings SET enabled = ?, updated_at = CURRENT_TIMESTAMP WHERE id = 1", enabledInt)
	return err
}

// SetSchedule updates the daily run schedule string (e.g. "13:00").
func (s *Store) SetSchedule(ctx context.Context, schedule string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE digest_settings SET schedule = ?, updated_at = CURRENT_TIMESTAMP WHERE id = 1", schedule)
	return err
}

// RecordStart creates an entry marking the beginning of a digest generation.
func (s *Store) RecordStart(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx, "INSERT INTO digest_history (started_at, status) VALUES (CURRENT_TIMESTAMP, 'running')")
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// RecordSuccess updates a digest run as successfully completed.
func (s *Store) RecordSuccess(ctx context.Context, id int64, summaryMD string, itemsCount int) error {
	_, err := s.db.ExecContext(ctx,
		"UPDATE digest_history SET finished_at = CURRENT_TIMESTAMP, status = 'success', summary_md = ?, items_count = ? WHERE id = ?",
		summaryMD, itemsCount, id)
	return err
}

// RecordFailure updates a digest run as failed with error details.
func (s *Store) RecordFailure(ctx context.Context, id int64, errMsg string) error {
	_, err := s.db.ExecContext(ctx,
		"UPDATE digest_history SET finished_at = CURRENT_TIMESTAMP, status = 'failed', error_message = ? WHERE id = ?",
		errMsg, id)
	return err
}

// RecordSkipped marks that a scheduled run was skipped because digest is disabled.
func (s *Store) RecordSkipped(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO digest_history (started_at, finished_at, status, summary_md) VALUES (CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 'skipped', 'Digest paused by user')")
	return err
}

// GetLatestRun returns the most recent digest run.
func (s *Store) GetLatestRun(ctx context.Context) (*RunRecord, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, started_at, COALESCE(finished_at, started_at), status, COALESCE(summary_md, ''), COALESCE(error_message, ''), items_count
		FROM digest_history ORDER BY id DESC LIMIT 1`)

	var r RunRecord
	var startedStr, finishedStr string
	if err := row.Scan(&r.ID, &startedStr, &finishedStr, &r.Status, &r.SummaryMD, &r.ErrorMessage, &r.ItemsCount); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	r.StartedAt = parseSQLiteTime(startedStr)
	r.FinishedAt = parseSQLiteTime(finishedStr)
	return &r, nil
}

// GetHistory returns recent digest runs.
func (s *Store) GetHistory(ctx context.Context, limit int) ([]RunRecord, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, started_at, COALESCE(finished_at, started_at), status, COALESCE(summary_md, ''), COALESCE(error_message, ''), items_count
		FROM digest_history ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []RunRecord
	for rows.Next() {
		var r RunRecord
		var startedStr, finishedStr string
		if err := rows.Scan(&r.ID, &startedStr, &finishedStr, &r.Status, &r.SummaryMD, &r.ErrorMessage, &r.ItemsCount); err == nil {
			r.StartedAt = parseSQLiteTime(startedStr)
			r.FinishedAt = parseSQLiteTime(finishedStr)
			list = append(list, r)
		}
	}
	return list, nil
}
