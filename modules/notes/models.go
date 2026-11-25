package notes

import "time"

// ChecklistItem represents a single checkable row inside a checklist note.
type ChecklistItem struct {
	Text string `json:"text"`
	Done bool   `json:"done"`
}

// Note represents a minimalist personal note or checklist stored in SQLite.
type Note struct {
	ID        int64           `json:"id"`
	Title     string          `json:"title"`
	Content   string          `json:"content"`
	Kind      string          `json:"kind"` // "note" or "checklist"
	Items     []ChecklistItem `json:"items"`
	Tags      string          `json:"tags"`
	Pinned    bool            `json:"pinned"`
	Archived  bool            `json:"archived"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// CreateInput defines the payload for creating a new note.
type CreateInput struct {
	Title   string          `json:"title"`
	Content string          `json:"content"`
	Kind    string          `json:"kind"`
	Items   []ChecklistItem `json:"items"`
	Tags    string          `json:"tags"`
	Pinned  bool            `json:"pinned"`
}

// UpdateInput defines the payload for updating an existing note.
type UpdateInput struct {
	Title    *string          `json:"title"`
	Content  *string          `json:"content"`
	Kind     *string          `json:"kind"`
	Items    *[]ChecklistItem `json:"items"`
	Tags     *string          `json:"tags"`
	Pinned   *bool            `json:"pinned"`
	Archived *bool            `json:"archived"`
}
