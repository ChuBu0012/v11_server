package workspace

import "time"

// Note represents a personal markdown note.
type Note struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	Content    string    `json:"content"`
	Tags       []string  `json:"tags"`
	IsPinned   bool      `json:"is_pinned"`
	SyncPublic bool      `json:"sync_public"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// CreateNoteRequest defines payload to create a note.
type CreateNoteRequest struct {
	Title      string   `json:"title"`
	Content    string   `json:"content"`
	Tags       []string `json:"tags"`
	IsPinned   bool     `json:"is_pinned"`
	SyncPublic bool     `json:"sync_public"`
}

// UpdateNoteRequest defines payload to partially update a note.
type UpdateNoteRequest struct {
	Title      *string   `json:"title"`
	Content    *string   `json:"content"`
	Tags       *[]string `json:"tags"`
	IsPinned   *bool     `json:"is_pinned"`
	SyncPublic *bool     `json:"sync_public"`
}

// NoteFilter contains criteria for searching and listing notes.
type NoteFilter struct {
	Tag    string
	Search string
	Limit  int
	Offset int
}

// NoteAIRequest represents AI assist request for notes.
type NoteAIRequest struct {
	Action      string `json:"action"` // summarize | action_items | polish
	Instruction string `json:"instruction,omitempty"`
}

// NoteAIResponse represents AI assist response for notes.
type NoteAIResponse struct {
	Action     string `json:"action"`
	Result     string `json:"result"`
	Provider   string `json:"provider"`
	DurationMs int64  `json:"duration_ms"`
}

// SheetCellStyle holds visual styling for a single cell.
type SheetCellStyle struct {
	Bold   bool   `json:"b,omitempty"`
	Italic bool   `json:"i,omitempty"`
	Align  string `json:"align,omitempty"` // "left" | "center" | "right"
	Bg     string `json:"bg,omitempty"`
	Color  string `json:"color,omitempty"`
}

// SheetCell represents data and style of a single cell.
type SheetCell struct {
	Raw    string          `json:"raw"`
	Value  any             `json:"v"`
	Format string          `json:"fmt,omitempty"`
	Style  *SheetCellStyle `json:"style,omitempty"`
}

// SheetMerge represents merged cell boundaries.
type SheetMerge struct {
	Start string `json:"start"` // e.g. "A1"
	End   string `json:"end"`   // e.g. "B2"
}

// SheetData represents parsed sheet data structure.
type SheetData struct {
	Cells     map[string]SheetCell `json:"cells"`
	Merges    []SheetMerge         `json:"merges,omitempty"`
	ColWidths map[string]int       `json:"colWidths,omitempty"`
}

// Sheet represents a lightweight spreadsheet record.
type Sheet struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	RowCount   int       `json:"row_count"`
	ColCount   int       `json:"col_count"`
	DataJSON   string    `json:"data_json"`
	SyncPublic bool      `json:"sync_public"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// SheetHeader represents a lightweight summary of a sheet without data_json.
type SheetHeader struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	RowCount   int       `json:"row_count"`
	ColCount   int       `json:"col_count"`
	SyncPublic bool      `json:"sync_public"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// CreateSheetRequest defines payload to create a new sheet.
type CreateSheetRequest struct {
	Title      string `json:"title"`
	RowCount   int    `json:"row_count"`
	ColCount   int    `json:"col_count"`
	DataJSON   string `json:"data_json"`
	SyncPublic bool   `json:"sync_public"`
}

// UpdateSheetRequest defines payload to update a sheet.
type UpdateSheetRequest struct {
	Title      *string `json:"title"`
	RowCount   *int    `json:"row_count"`
	ColCount   *int    `json:"col_count"`
	DataJSON   *string `json:"data_json"`
	SyncPublic *bool   `json:"sync_public"`
}

// SheetAIRequest represents AI assist request for sheets.
type SheetAIRequest struct {
	Action    string `json:"action"` // explain | formula_suggest
	Prompt    string `json:"prompt"`
	Selection string `json:"selection,omitempty"`
}

// SheetAIResponse represents AI assist response for sheets.
type SheetAIResponse struct {
	Action      string `json:"action"`
	Result      string `json:"result"`
	Explanation string `json:"explanation,omitempty"`
	Provider    string `json:"provider"`
	DurationMs  int64  `json:"duration_ms"`
}
