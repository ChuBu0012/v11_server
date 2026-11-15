package digest

import (
	"time"
)

// NewsItem represents an individual news or insight item parsed from RSS feeds.
type NewsItem struct {
	Category    string    `json:"category"` // "AI", "Politics", "Trivia"
	Source      string    `json:"source"`
	Title       string    `json:"title"`
	Link        string    `json:"link"`
	Description string    `json:"description"`
	PublishedAt time.Time `json:"published_at"`
}

// Settings stores user configuration for the automated digest.
type Settings struct {
	Enabled   bool      `json:"enabled"`
	Schedule  string    `json:"schedule"` // e.g. "13:00"
	UpdatedAt time.Time `json:"updated_at"`
}

// RunRecord logs execution status and outputs of a digest run.
type RunRecord struct {
	ID           int64     `json:"id"`
	StartedAt    time.Time `json:"started_at"`
	FinishedAt   time.Time `json:"finished_at"`
	Status       string    `json:"status"` // "running", "success", "failed", "skipped"
	SummaryMD    string    `json:"summary_md"`
	ErrorMessage string    `json:"error_message,omitempty"`
	ItemsCount   int       `json:"items_count"`
}
