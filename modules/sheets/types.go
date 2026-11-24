package sheets

import "time"

// Sheet represents a spreadsheet document.
type Sheet struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	DataJSON  string    `json:"data_json"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CreateSheetRequest payload for creating a new sheet.
type CreateSheetRequest struct {
	Title    string `json:"title"`
	DataJSON string `json:"data_json"`
}

// UpdateSheetRequest payload for updating an existing sheet.
type UpdateSheetRequest struct {
	Title    string `json:"title"`
	DataJSON string `json:"data_json"`
}

// AIRequest represents an AI assist query on a sheet.
type AIRequest struct {
	Action string `json:"action"` // "generate", "analyze", "formula"
	Prompt string `json:"prompt"`
}

// AIResponse represents the AI response.
type AIResponse struct {
	Action     string `json:"action"`
	Result     string `json:"result"`
	Provider   string `json:"provider"`
	DurationMS int64  `json:"duration_ms"`
}
