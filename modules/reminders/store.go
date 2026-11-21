package reminders

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) (*Store, error) {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS reminders (id INTEGER PRIMARY KEY AUTOINCREMENT, title TEXT NOT NULL, details TEXT NOT NULL DEFAULT '', starts_at TEXT NOT NULL, recurrence TEXT NOT NULL CHECK(recurrence IN ('once','weekly')), offsets TEXT NOT NULL, enabled INTEGER NOT NULL DEFAULT 1, paused INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS reminder_occurrences (id INTEGER PRIMARY KEY AUTOINCREMENT, reminder_id INTEGER NOT NULL REFERENCES reminders(id) ON DELETE CASCADE, event_at TEXT NOT NULL, state TEXT NOT NULL DEFAULT 'scheduled', completed_at TEXT, UNIQUE(reminder_id,event_at))`,
		`CREATE TABLE IF NOT EXISTS reminder_deliveries (id INTEGER PRIMARY KEY AUTOINCREMENT, occurrence_id INTEGER NOT NULL REFERENCES reminder_occurrences(id) ON DELETE CASCADE, reminder_id INTEGER NOT NULL REFERENCES reminders(id) ON DELETE CASCADE, event_at TEXT NOT NULL, due_at TEXT NOT NULL, offset_minutes INTEGER NOT NULL, kind TEXT NOT NULL DEFAULT 'notice', state TEXT NOT NULL DEFAULT 'pending', attempts INTEGER NOT NULL DEFAULT 0, retry_at TEXT, last_error TEXT NOT NULL DEFAULT '', sent_at TEXT, telegram_message_id INTEGER, UNIQUE(reminder_id,event_at,offset_minutes,kind))`,
		`CREATE INDEX IF NOT EXISTS idx_reminder_delivery_due ON reminder_deliveries(state,due_at,retry_at)`,
		`CREATE INDEX IF NOT EXISTS idx_reminders_enabled_starts ON reminders(enabled, starts_at)`,
		`CREATE INDEX IF NOT EXISTS idx_reminder_occurrences_state_completed ON reminder_occurrences(state, completed_at)`,
		`CREATE INDEX IF NOT EXISTS idx_reminder_deliveries_occurrence ON reminder_deliveries(occurrence_id)`,
		`CREATE INDEX IF NOT EXISTS idx_reminder_deliveries_reminder ON reminder_deliveries(reminder_id)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return nil, fmt.Errorf("create reminder schema: %w", err)
		}
	}
	columns, err := db.Query(`PRAGMA table_info(reminders)`)
	if err != nil {
		return nil, fmt.Errorf("inspect reminder schema: %w", err)
	}
	hasPaused := false
	for columns.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := columns.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			columns.Close()
			return nil, fmt.Errorf("read reminder schema: %w", err)
		}
		if name == "paused" {
			hasPaused = true
		}
	}
	if err := columns.Err(); err != nil {
		columns.Close()
		return nil, fmt.Errorf("read reminder schema: %w", err)
	}
	columns.Close()
	if !hasPaused {
		if _, err := db.Exec(`ALTER TABLE reminders ADD COLUMN paused INTEGER NOT NULL DEFAULT 0`); err != nil {
			return nil, fmt.Errorf("add reminder pause state: %w", err)
		}
	}
	return &Store{db: db}, nil
}

func (s *Store) Create(ctx context.Context, input Input) (Reminder, error) {
	starts, err := time.Parse(time.RFC3339, input.StartsAt)
	if err != nil {
		return Reminder{}, fmt.Errorf("invalid date and time")
	}
	loc := location()
	starts = starts.In(loc)
	if !starts.After(time.Now()) {
		return Reminder{}, fmt.Errorf("choose a future date and time")
	}
	input.Title = strings.TrimSpace(trimRunes(input.Title, 120))
	input.Details = trimRunes(input.Details, 1500)
	if input.Title == "" {
		return Reminder{}, fmt.Errorf("title is required")
	}
	if input.Recurrence == "" {
		input.Recurrence = "once"
	}
	if input.Recurrence != "weekly" {
		input.Recurrence = "once"
	}
	input.Offsets = normalizeOffsets(input.Offsets)
	encoded, _ := json.Marshal(input.Offsets)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.ExecContext(ctx, `INSERT INTO reminders(title,details,starts_at,recurrence,offsets,enabled,created_at,updated_at) VALUES(?,?,?,?,?,1,?,?)`, input.Title, input.Details, starts.UTC().Format(time.RFC3339Nano), input.Recurrence, string(encoded), now, now)
	if err != nil {
		return Reminder{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Reminder{}, err
	}
	if err := s.ensureOccurrences(ctx, time.Now()); err != nil {
		return Reminder{}, err
	}
	return Reminder{ID: id, Title: input.Title, Details: input.Details, StartsAt: starts, Recurrence: input.Recurrence, Offsets: input.Offsets, Enabled: true}, nil
}

func (s *Store) List(ctx context.Context) ([]Reminder, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,title,details,starts_at,recurrence,offsets,enabled,paused,created_at FROM reminders WHERE enabled=1 ORDER BY starts_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Reminder{}
	for rows.Next() {
		var x Reminder
		var start, created, raw string
		var enabled, paused int
		if err := rows.Scan(&x.ID, &x.Title, &x.Details, &start, &x.Recurrence, &raw, &enabled, &paused, &created); err != nil {
			return nil, err
		}
		x.StartsAt, _ = time.Parse(time.RFC3339Nano, start)
		x.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		_ = json.Unmarshal([]byte(raw), &x.Offsets)
		x.Enabled = enabled == 1
		x.Paused = paused == 1
		items = append(items, x)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close() // Release connection before fetching related occurrences

	cutoff := time.Now().Add(-35 * 24 * time.Hour).UTC().Format(time.RFC3339Nano)
	for i := range items {
		completed, queryErr := s.db.QueryContext(ctx, `SELECT event_at FROM reminder_occurrences WHERE reminder_id=? AND state='done' AND completed_at>=?`, items[i].ID, cutoff)
		if queryErr != nil {
			return nil, queryErr
		}
		for completed.Next() {
			var event string
			if err := completed.Scan(&event); err != nil {
				completed.Close()
				return nil, err
			}
			parsed, _ := time.Parse(time.RFC3339Nano, event)
			items[i].CompletedOccurrences = append(items[i].CompletedOccurrences, parsed)
		}
		if err := completed.Err(); err != nil {
			completed.Close()
			return nil, err
		}
		completed.Close()
	}
	return items, nil
}

func (s *Store) Delete(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `UPDATE reminders SET enabled=0,updated_at=? WHERE id=?`, time.Now().UTC().Format(time.RFC3339Nano), id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE reminder_deliveries SET state='canceled' WHERE reminder_id=? AND state IN ('pending','retry')`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) SetPaused(ctx context.Context, id int64, paused bool) error {
	value := 0
	if paused {
		value = 1
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE reminders SET paused=?,updated_at=? WHERE id=? AND enabled=1 AND recurrence='weekly'`, value, time.Now().UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return sql.ErrNoRows
	}
	if paused {
		if _, err = tx.ExecContext(ctx, `UPDATE reminder_deliveries SET state='canceled' WHERE reminder_id=? AND state IN ('pending','retry')`, id); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM reminder_deliveries WHERE reminder_id=? AND state='canceled'`, id); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	if !paused {
		return s.ensureOccurrences(ctx, time.Now())
	}
	return nil
}

func (s *Store) Update(ctx context.Context, id int64, input Input) error {
	starts, err := time.Parse(time.RFC3339, input.StartsAt)
	if err != nil {
		return fmt.Errorf("invalid date and time")
	}
	if input.Recurrence != "weekly" && !starts.After(time.Now()) {
		return fmt.Errorf("choose a future date and time")
	}
	input.Title = strings.TrimSpace(input.Title)
	if input.Title == "" {
		return fmt.Errorf("title is required")
	}
	if input.Recurrence != "weekly" {
		input.Recurrence = "once"
	}
	input.Offsets = normalizeOffsets(input.Offsets)
	encoded, _ := json.Marshal(input.Offsets)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE reminders SET title=?,details=?,starts_at=?,recurrence=?,offsets=?,updated_at=? WHERE id=? AND enabled=1`, trimRunes(input.Title, 120), trimRunes(input.Details, 1500), starts.UTC().Format(time.RFC3339Nano), input.Recurrence, string(encoded), now, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	if _, err = tx.ExecContext(ctx, `UPDATE reminder_deliveries SET state='canceled' WHERE reminder_id=? AND state IN ('pending','retry')`, id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM reminder_deliveries WHERE reminder_id=? AND state='canceled'`, id); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return s.ensureOccurrences(ctx, time.Now())
}

func (s *Store) ensureOccurrences(ctx context.Context, now time.Time) error {
	reminders, err := s.List(ctx)
	if err != nil {
		return err
	}
	loc := location()
	for _, r := range reminders {
		if r.Paused {
			continue
		}
		first := nextEvent(r, now.In(loc), loc)
		count := 1
		if r.Recurrence == "weekly" {
			count = 3
		}
		for i := 0; i < count; i++ {
			event := first.AddDate(0, 0, 7*i)
			if r.Recurrence != "weekly" && i > 0 {
				break
			}
			if _, err = s.db.ExecContext(ctx, `INSERT OR IGNORE INTO reminder_occurrences(reminder_id,event_at) VALUES(?,?)`, r.ID, event.UTC().Format(time.RFC3339Nano)); err != nil {
				return err
			}
			var occID int64
			if err = s.db.QueryRowContext(ctx, `SELECT id FROM reminder_occurrences WHERE reminder_id=? AND event_at=?`, r.ID, event.UTC().Format(time.RFC3339Nano)).Scan(&occID); err != nil {
				return err
			}
			var state string
			_ = s.db.QueryRowContext(ctx, `SELECT state FROM reminder_occurrences WHERE id=?`, occID).Scan(&state)
			if state == "done" {
				continue
			}
			for _, offset := range r.Offsets {
				due := event.Add(-time.Duration(offset) * time.Minute)
				if due.Before(now) {
					continue
				}
				_, err = s.db.ExecContext(ctx, `INSERT OR IGNORE INTO reminder_deliveries(occurrence_id,reminder_id,event_at,due_at,offset_minutes,kind) VALUES(?,?,?,?,?,'notice')`, occID, r.ID, event.UTC().Format(time.RFC3339Nano), due.UTC().Format(time.RFC3339Nano), offset)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func nextEvent(r Reminder, now time.Time, loc *time.Location) time.Time {
	start := r.StartsAt.In(loc)
	if r.Recurrence != "weekly" || !start.Before(now) {
		return start
	}
	days := int(now.Sub(start).Hours() / 24 / 7)
	candidate := start.AddDate(0, 0, days*7)
	for candidate.Before(now) {
		candidate = candidate.AddDate(0, 0, 7)
	}
	return candidate
}

func (s *Store) Due(ctx context.Context, now time.Time) ([]Delivery, error) {
	if err := s.ensureOccurrences(ctx, now); err != nil {
		return nil, err
	}
	cutoff := now.Add(-10 * time.Minute).UTC().Format(time.RFC3339Nano)
	_, _ = s.db.ExecContext(ctx, `UPDATE reminder_deliveries SET state='missed' WHERE state='pending' AND due_at<? AND occurrence_id IN (SELECT id FROM reminder_occurrences WHERE state='scheduled')`, cutoff)
	rows, err := s.db.QueryContext(ctx, `SELECT d.id,d.reminder_id,r.title,r.details,d.event_at,d.due_at,d.offset_minutes,d.kind,d.state,d.last_error FROM reminder_deliveries d JOIN reminders r ON r.id=d.reminder_id JOIN reminder_occurrences o ON o.id=d.occurrence_id WHERE r.enabled=1 AND r.paused=0 AND o.state='scheduled' AND ((d.state='pending' AND d.due_at<=?) OR (d.state='retry' AND d.retry_at<=?)) ORDER BY d.due_at LIMIT 10`, now.UTC().Format(time.RFC3339Nano), now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Delivery{}
	for rows.Next() {
		var d Delivery
		var event, due string
		if err := rows.Scan(&d.ID, &d.ReminderID, &d.Title, &d.Details, &event, &due, &d.Offset, &d.Kind, &d.State, &d.LastError); err != nil {
			return nil, err
		}
		d.EventAt, _ = time.Parse(time.RFC3339Nano, event)
		d.DueAt, _ = time.Parse(time.RFC3339Nano, due)
		items = append(items, d)
	}
	return items, rows.Err()
}

func (s *Store) MarkSent(ctx context.Context, id, msgID int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE reminder_deliveries SET state='sent',sent_at=?,telegram_message_id=?,last_error='' WHERE id=? AND state IN ('pending','retry')`, time.Now().UTC().Format(time.RFC3339Nano), msgID, id)
	return err
}
func (s *Store) MarkFailed(ctx context.Context, d Delivery, sendErr error, now time.Time) error {
	attempt := 1
	_ = s.db.QueryRowContext(ctx, `SELECT attempts+1 FROM reminder_deliveries WHERE id=?`, d.ID).Scan(&attempt)
	state := "retry"
	retry := now.Add(time.Minute).UTC().Format(time.RFC3339Nano)
	if now.Sub(d.DueAt) >= 10*time.Minute {
		state = "failed"
		retry = ""
	}
	_, err := s.db.ExecContext(ctx, `UPDATE reminder_deliveries SET state=?,attempts=?,retry_at=?,last_error=? WHERE id=? AND state IN ('pending','retry')`, state, attempt, retry, trimRunes(sendErr.Error(), 300), d.ID)
	return err
}

func (s *Store) Upcoming(ctx context.Context, now time.Time) ([]Delivery, error) {
	if err := s.ensureOccurrences(ctx, now); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT d.id,d.reminder_id,r.title,r.details,d.event_at,d.due_at,d.offset_minutes,d.kind,d.state,d.last_error FROM reminder_deliveries d JOIN reminders r ON r.id=d.reminder_id JOIN reminder_occurrences o ON o.id=d.occurrence_id WHERE r.enabled=1 AND r.paused=0 AND o.state='scheduled' AND d.state IN ('pending','retry','failed') ORDER BY CASE d.state WHEN 'failed' THEN 0 ELSE 1 END,d.due_at LIMIT 40`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Delivery{}
	for rows.Next() {
		var d Delivery
		var e, u string
		if err := rows.Scan(&d.ID, &d.ReminderID, &d.Title, &d.Details, &e, &u, &d.Offset, &d.Kind, &d.State, &d.LastError); err != nil {
			return nil, err
		}
		d.EventAt, _ = time.Parse(time.RFC3339Nano, e)
		d.DueAt, _ = time.Parse(time.RFC3339Nano, u)
		items = append(items, d)
	}
	return items, rows.Err()
}

func (s *Store) Complete(ctx context.Context, id int64, eventAt time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	event := eventAt.UTC().Format(time.RFC3339Nano)
	result, err := tx.ExecContext(ctx, `UPDATE reminder_occurrences SET state='done',completed_at=? WHERE reminder_id=? AND event_at=?`, time.Now().UTC().Format(time.RFC3339Nano), id, event)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return sql.ErrNoRows
	}
	if _, err = tx.ExecContext(ctx, `UPDATE reminder_deliveries SET state='canceled' WHERE reminder_id=? AND event_at=? AND state IN ('pending','retry')`, id, event); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE reminders SET enabled=0,updated_at=? WHERE id=? AND recurrence='once'`, time.Now().UTC().Format(time.RFC3339Nano), id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) History(ctx context.Context) ([]CompletedOccurrence, error) {
	cutoff := time.Now().Add(-30 * 24 * time.Hour).UTC().Format(time.RFC3339Nano)
	_, _ = s.db.ExecContext(ctx, `DELETE FROM reminder_deliveries WHERE occurrence_id IN (SELECT id FROM reminder_occurrences WHERE event_at<?)`, cutoff)
	_, _ = s.db.ExecContext(ctx, `DELETE FROM reminder_occurrences WHERE event_at<?`, cutoff)
	_, _ = s.db.ExecContext(ctx, `DELETE FROM reminder_deliveries WHERE occurrence_id IN (SELECT id FROM reminder_occurrences WHERE state='done' AND completed_at<?)`, cutoff)
	_, _ = s.db.ExecContext(ctx, `DELETE FROM reminder_occurrences WHERE state='done' AND completed_at<?`, cutoff)
	_, _ = s.db.ExecContext(ctx, `DELETE FROM reminder_deliveries WHERE reminder_id IN (SELECT id FROM reminders WHERE enabled=0 AND updated_at<?)`, cutoff)
	_, _ = s.db.ExecContext(ctx, `DELETE FROM reminder_occurrences WHERE reminder_id IN (SELECT id FROM reminders WHERE enabled=0 AND updated_at<?)`, cutoff)
	_, _ = s.db.ExecContext(ctx, `DELETE FROM reminders WHERE enabled=0 AND updated_at<?`, cutoff)
	rows, err := s.db.QueryContext(ctx, `SELECT o.reminder_id,r.title,r.details,o.event_at,o.completed_at,r.recurrence FROM reminder_occurrences o JOIN reminders r ON r.id=o.reminder_id WHERE o.state='done' AND o.completed_at>=? ORDER BY o.completed_at DESC LIMIT 30`, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []CompletedOccurrence{}
	for rows.Next() {
		var item CompletedOccurrence
		var event, completed string
		if err := rows.Scan(&item.ReminderID, &item.Title, &item.Details, &event, &completed, &item.Recurrence); err != nil {
			return nil, err
		}
		item.EventAt, _ = time.Parse(time.RFC3339Nano, event)
		item.CompletedAt, _ = time.Parse(time.RFC3339Nano, completed)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) Snooze(ctx context.Context, id int64, minutes int) error {
	if minutes != 5 && minutes != 10 && minutes != 30 {
		return fmt.Errorf("unsupported snooze")
	}
	var reminderID int64
	var event string
	var occID int64
	var deliveryState, occurrenceState string
	err := s.db.QueryRowContext(ctx, `SELECT d.reminder_id,d.event_at,d.occurrence_id,d.state,o.state FROM reminder_deliveries d JOIN reminder_occurrences o ON o.id=d.occurrence_id JOIN reminders r ON r.id=d.reminder_id WHERE d.id=? AND r.enabled=1`, id).Scan(&reminderID, &event, &occID, &deliveryState, &occurrenceState)
	if err != nil {
		return err
	}
	if deliveryState != "sent" || occurrenceState == "done" {
		return fmt.Errorf("reminder is no longer active")
	}
	due := time.Now().Add(time.Duration(minutes) * time.Minute).UTC().Format(time.RFC3339Nano)
	_, err = s.db.ExecContext(ctx, `INSERT OR IGNORE INTO reminder_deliveries(occurrence_id,reminder_id,event_at,due_at,offset_minutes,kind) VALUES(?,?,?,?,?,'snooze')`, occID, reminderID, event, due, -minutes)
	return err
}

func (s *Store) GetDelivery(ctx context.Context, id int64) (Delivery, error) {
	var d Delivery
	var e, u string
	err := s.db.QueryRowContext(ctx, `SELECT d.id,d.reminder_id,r.title,r.details,d.event_at,d.due_at,d.offset_minutes,d.kind,d.state,d.last_error FROM reminder_deliveries d JOIN reminders r ON r.id=d.reminder_id WHERE d.id=?`, id).Scan(&d.ID, &d.ReminderID, &d.Title, &d.Details, &e, &u, &d.Offset, &d.Kind, &d.State, &d.LastError)
	d.EventAt, _ = time.Parse(time.RFC3339Nano, e)
	d.DueAt, _ = time.Parse(time.RFC3339Nano, u)
	return d, err
}

func normalizeOffsets(values []int) []int {
	allowed := map[int]bool{0: true, 5: true, 10: true, 30: true}
	out := []int{}
	seen := map[int]bool{}
	for _, v := range values {
		if allowed[v] && !seen[v] {
			out = append(out, v)
			seen[v] = true
		}
	}
	if len(out) == 0 {
		out = []int{0}
	}
	sort.Ints(out)
	return out
}
func trimRunes(value string, limit int) string {
	r := []rune(value)
	if len(r) > limit {
		return string(r[:limit])
	}
	return value
}
