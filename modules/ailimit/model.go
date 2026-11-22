package ailimit

import "time"

// QuotaBucket represents a specific quota window and limit.
type QuotaBucket struct {
	Provider          string    `json:"provider"`           // e.g. "antigravity"
	GroupName         string    `json:"group_name"`         // e.g. "Gemini Models", "Claude and GPT models"
	BucketID          string    `json:"bucket_id"`          // e.g. "gemini-5h", "gemini-weekly", "3p-5h", "3p-weekly"
	DisplayName       string    `json:"display_name"`       // e.g. "Five Hour Limit Remaining", "Weekly Limit Remaining"
	Window            string    `json:"window"`             // "5h" or "weekly"
	UsedPercent       float64   `json:"used_percent"`       // 0.0 - 100.0
	RemainingPercent  float64   `json:"remaining_percent"`  // 0.0 - 100.0
	RemainingFraction float64   `json:"remaining_fraction"` // 0.0 - 1.0
	ResetTime         time.Time `json:"reset_time"`
	LastCheckedAt     time.Time `json:"last_checked_at"`
	IsLow             bool      `json:"is_low"` // true if remaining <= 15%
}

// QuotaGroup groups buckets by model family.
type QuotaGroup struct {
	GroupName   string        `json:"group_name"`
	DisplayName string        `json:"display_name"`
	Buckets     []QuotaBucket `json:"buckets"`
}

// QuotaSnapshot represents historical point-in-time quota sample.
type QuotaSnapshot struct {
	ID                int64     `json:"id"`
	Provider          string    `json:"provider"`
	GroupName         string    `json:"group_name"`
	BucketID          string    `json:"bucket_id"`
	RemainingFraction float64   `json:"remaining_fraction"`
	RemainingPercent  float64   `json:"remaining_percent"`
	ResetTime         time.Time `json:"reset_time"`
	RecordedAt        time.Time `json:"recorded_at"`
}

// QuotaInsight provides actionable analysis on current quota state.
type QuotaInsight struct {
	Level          string  `json:"level"` // "normal", "warning", "critical"
	Title          string  `json:"title"`
	Summary        string  `json:"summary"`
	Recommendation string  `json:"recommendation"`
	CriticalBucket *string `json:"critical_bucket"` // bucket_id if any is low
	AlertTriggered bool    `json:"alert_triggered"`
}

// QuotaResponse is the API response payload for GET /api/limits.
type QuotaResponse struct {
	UpdatedAt time.Time       `json:"updated_at"`
	Groups    []QuotaGroup    `json:"groups"`
	Buckets   []QuotaBucket   `json:"buckets"`
	Insight   QuotaInsight    `json:"insight"`
	History   []QuotaSnapshot `json:"history,omitempty"`
}
