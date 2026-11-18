package reminders

import "time"

const timezone = "Asia/Bangkok"

func location() *time.Location {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return time.FixedZone(timezone, 7*60*60)
	}
	return loc
}

type Reminder struct {
	ID                   int64       `json:"id"`
	Title                string      `json:"title"`
	Details              string      `json:"details,omitempty"`
	StartsAt             time.Time   `json:"starts_at"`
	Recurrence           string      `json:"recurrence"`
	Offsets              []int       `json:"offsets"`
	Enabled              bool        `json:"enabled"`
	Paused               bool        `json:"paused"`
	CreatedAt            time.Time   `json:"created_at"`
	CompletedOccurrences []time.Time `json:"completed_occurrences,omitempty"`
}

type CompletedOccurrence struct {
	ReminderID  int64     `json:"reminder_id"`
	Title       string    `json:"title"`
	Details     string    `json:"details,omitempty"`
	EventAt     time.Time `json:"event_at"`
	CompletedAt time.Time `json:"completed_at"`
	Recurrence  string    `json:"recurrence"`
}

type Delivery struct {
	ID         int64     `json:"id"`
	ReminderID int64     `json:"reminder_id"`
	Title      string    `json:"title"`
	Details    string    `json:"details,omitempty"`
	EventAt    time.Time `json:"event_at"`
	DueAt      time.Time `json:"due_at"`
	Offset     int       `json:"offset_minutes"`
	Kind       string    `json:"kind"`
	State      string    `json:"state"`
	LastError  string    `json:"last_error,omitempty"`
}

type Input struct {
	Title      string `json:"title"`
	Details    string `json:"details"`
	StartsAt   string `json:"starts_at"`
	Recurrence string `json:"recurrence"`
	Offsets    []int  `json:"offsets"`
}
