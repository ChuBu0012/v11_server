package alerts

import (
	"database/sql"
	"time"
)

// AlertStore persists alert state in SQLite to survive process restarts.
type AlertStore struct {
	db *sql.DB
}

// NewAlertStore creates a new AlertStore backed by the given database.
func NewAlertStore(db *sql.DB) *AlertStore {
	return &AlertStore{db: db}
}

// LoadState retrieves all persisted alert states, keyed by ConditionType.
func (s *AlertStore) LoadState() (map[ConditionType]*AlertState, error) {
	rows, err := s.db.Query(`
		SELECT condition_type, active, triggered_at, last_checked_at, value_at_trigger
		FROM alert_states
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	states := make(map[ConditionType]*AlertState)
	for rows.Next() {
		var st AlertState
		var ct string
		var triggeredAt, lastCheckedAt string
		if err := rows.Scan(&ct, &st.Active, &triggeredAt, &lastCheckedAt, &st.ValueAtTrigger); err != nil {
			return nil, err
		}
		st.ConditionType = ConditionType(ct)
		st.TriggeredAt, _ = time.Parse(time.RFC3339, triggeredAt)
		st.LastCheckedAt, _ = time.Parse(time.RFC3339, lastCheckedAt)
		states[st.ConditionType] = &st
	}
	return states, rows.Err()
}

// SaveState upserts the alert state for a given condition.
func (s *AlertStore) SaveState(cond ConditionType, st AlertState) error {
	_, err := s.db.Exec(`
		INSERT INTO alert_states (condition_type, active, triggered_at, last_checked_at, value_at_trigger)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(condition_type) DO UPDATE SET
			active = excluded.active,
			triggered_at = excluded.triggered_at,
			last_checked_at = excluded.last_checked_at,
			value_at_trigger = excluded.value_at_trigger
	`, string(cond), st.Active, st.TriggeredAt.UTC().Format(time.RFC3339), st.LastCheckedAt.UTC().Format(time.RFC3339), st.ValueAtTrigger)
	return err
}

// ClearState removes the persisted state for a condition (used after recovery).
func (s *AlertStore) ClearState(cond ConditionType) error {
	_, err := s.db.Exec(`DELETE FROM alert_states WHERE condition_type = ?`, string(cond))
	return err
}
