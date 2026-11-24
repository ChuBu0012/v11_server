package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const (
	MaxNoteContentBytes = 1024 * 1024 // 1 MB
	MaxSheetRows        = 100
	MaxSheetCols        = 26 // A to Z
	DefaultSheetRows    = 30
	DefaultSheetCols    = 10
)

var (
	ErrNoteTooLarge          = errors.New("note content exceeds 1 MB limit")
	ErrSheetDimensionsExceed = fmt.Errorf("sheet dimensions exceed max allowed (%dx%d)", MaxSheetRows, MaxSheetCols)
)

// Service encapsulates workspace business rules and validation.
type Service struct {
	store *Store
	ai    *AIAssistant
}

// NewService constructs a new Workspace Service.
func NewService(store *Store, ai *AIAssistant) *Service {
	return &Service{
		store: store,
		ai:    ai,
	}
}

// ExtractHashtags scans text for hashtag tokens (e.g. #todo, #finance, #ช้อปปิ้ง).
// It ignores Markdown headings where '#' is followed by whitespace.
func ExtractHashtags(text string) []string {
	if text == "" {
		return []string{}
	}

	// Regex matches '#' followed immediately by alphanumeric / unicode / thai characters
	// Preceded by start of string, whitespace, or punctuation
	re := regexp.MustCompile(`(?:\s|^)#([^\s#.,!?:;()[\]{}<>"']+)`)
	matches := re.FindAllStringSubmatch(text, -1)

	tagMap := make(map[string]bool)
	var tags []string

	for _, m := range matches {
		if len(m) > 1 {
			rawTag := strings.TrimSpace(m[1])
			if rawTag == "" {
				continue
			}
			lower := strings.ToLower(rawTag)
			if !tagMap[lower] {
				tagMap[lower] = true
				tags = append(tags, lower)
			}
		}
	}
	return tags
}

// mergeTags combines tags list with extracted tags, removing duplicates and '#' prefixes.
func mergeTags(tags []string, extracted []string) []string {
	seen := make(map[string]bool)
	var merged []string

	for _, t := range tags {
		clean := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(t, "#")))
		if clean != "" && !seen[clean] {
			seen[clean] = true
			merged = append(merged, clean)
		}
	}

	for _, t := range extracted {
		clean := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(t, "#")))
		if clean != "" && !seen[clean] {
			seen[clean] = true
			merged = append(merged, clean)
		}
	}

	return merged
}

// CreateNote validates input, extracts hashtags, and creates a note.
func (s *Service) CreateNote(ctx context.Context, req *CreateNoteRequest) (*Note, error) {
	if len(req.Content) > MaxNoteContentBytes {
		return nil, ErrNoteTooLarge
	}

	// If title is empty, derive from first non-empty line of content
	title := strings.TrimSpace(req.Title)
	if title == "" {
		lines := strings.Split(req.Content, "\n")
		for _, l := range lines {
			t := strings.TrimSpace(l)
			if t != "" {
				// Strip markdown leading symbols if any (#, -, *, >, [ ], [x], etc.)
				t = strings.TrimLeft(t, "#-* >")
				t = strings.TrimSpace(t)
				if strings.HasPrefix(t, "[ ]") || strings.HasPrefix(t, "[x]") || strings.HasPrefix(t, "[X]") {
					t = strings.TrimSpace(t[3:])
				}
				// Remove trailing or inline hashtags from derived title
				t = strings.TrimSpace(regexp.MustCompile(`(?:\s|^)#[^\s]+`).ReplaceAllString(t, ""))
				if len([]rune(t)) > 60 {
					t = string([]rune(t)[:57]) + "..."
				}
				title = t
				break
			}
		}
		if title == "" {
			title = "บันทึกไม่มีชื่อ"
		}
	}

	extractedTags := ExtractHashtags(req.Content)
	finalTags := mergeTags(req.Tags, extractedTags)

	noteReq := &CreateNoteRequest{
		Title:      title,
		Content:    req.Content,
		Tags:       finalTags,
		IsPinned:   req.IsPinned,
		SyncPublic: req.SyncPublic,
	}

	return s.store.CreateNote(ctx, noteReq)
}

// GetNote retrieves a note by ID.
func (s *Service) GetNote(ctx context.Context, id string) (*Note, error) {
	return s.store.GetNote(ctx, id)
}

// UpdateNote updates a note and recalculates tags if content was changed.
func (s *Service) UpdateNote(ctx context.Context, id string, req *UpdateNoteRequest) (*Note, error) {
	if req.Content != nil && len(*req.Content) > MaxNoteContentBytes {
		return nil, ErrNoteTooLarge
	}

	if req.Content != nil {
		extracted := ExtractHashtags(*req.Content)
		var baseTags []string
		if req.Tags != nil {
			baseTags = *req.Tags
		} else {
			existing, err := s.store.GetNote(ctx, id)
			if err == nil && existing != nil {
				baseTags = existing.Tags
			}
		}
		merged := mergeTags(baseTags, extracted)
		req.Tags = &merged
	}

	return s.store.UpdateNote(ctx, id, req)
}

// DeleteNote deletes a note by ID.
func (s *Service) DeleteNote(ctx context.Context, id string) error {
	return s.store.DeleteNote(ctx, id)
}

// ListNotes lists notes with filters.
func (s *Service) ListNotes(ctx context.Context, filter NoteFilter) ([]Note, error) {
	return s.store.ListNotes(ctx, filter)
}

// ListPublicNotes lists notes marked with sync_public = true.
func (s *Service) ListPublicNotes(ctx context.Context) ([]Note, error) {
	return s.store.ListPublicNotes(ctx)
}

// CreateSheet validates sheet bounds and recalculates formulas before persisting.
func (s *Service) CreateSheet(ctx context.Context, req *CreateSheetRequest) (*Sheet, error) {
	rowCnt := req.RowCount
	if rowCnt <= 0 {
		rowCnt = DefaultSheetRows
	}
	colCnt := req.ColCount
	if colCnt <= 0 {
		colCnt = DefaultSheetCols
	}

	if rowCnt > MaxSheetRows || colCnt > MaxSheetCols {
		return nil, ErrSheetDimensionsExceed
	}

	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = "ชีตไม่มีชื่อ"
	}

	dataJSON := strings.TrimSpace(req.DataJSON)
	if dataJSON == "" || dataJSON == "{}" {
		dataJSON = `{"cells":{}}`
	} else {
		// Evaluate formulas and ensure scalar "v" values are current
		var sheetData SheetData
		if err := json.Unmarshal([]byte(dataJSON), &sheetData); err == nil {
			if sheetData.Cells == nil {
				sheetData.Cells = make(map[string]SheetCell)
			}
			EvaluateFullSheet(&sheetData)
			if bytes, err := json.Marshal(sheetData); err == nil {
				dataJSON = string(bytes)
			}
		}
	}

	sheetReq := &CreateSheetRequest{
		Title:      title,
		RowCount:   rowCnt,
		ColCount:   colCnt,
		DataJSON:   dataJSON,
		SyncPublic: req.SyncPublic,
	}

	return s.store.CreateSheet(ctx, sheetReq)
}

// GetSheet retrieves a sheet by ID.
func (s *Service) GetSheet(ctx context.Context, id string) (*Sheet, error) {
	return s.store.GetSheet(ctx, id)
}

// UpdateSheet updates a sheet, re-evaluating formulas if data_json is updated.
func (s *Service) UpdateSheet(ctx context.Context, id string, req *UpdateSheetRequest) (*Sheet, error) {
	if req.RowCount != nil && *req.RowCount > MaxSheetRows {
		return nil, ErrSheetDimensionsExceed
	}
	if req.ColCount != nil && *req.ColCount > MaxSheetCols {
		return nil, ErrSheetDimensionsExceed
	}

	if req.DataJSON != nil {
		var sheetData SheetData
		if err := json.Unmarshal([]byte(*req.DataJSON), &sheetData); err == nil {
			if sheetData.Cells == nil {
				sheetData.Cells = make(map[string]SheetCell)
			}
			EvaluateFullSheet(&sheetData)
			if bytes, err := json.Marshal(sheetData); err == nil {
				reEvaluated := string(bytes)
				req.DataJSON = &reEvaluated
			}
		}
	}

	return s.store.UpdateSheet(ctx, id, req)
}

// DeleteSheet deletes a sheet by ID.
func (s *Service) DeleteSheet(ctx context.Context, id string) error {
	return s.store.DeleteSheet(ctx, id)
}

// ListSheets lists summary headers of all sheets.
func (s *Service) ListSheets(ctx context.Context) ([]SheetHeader, error) {
	return s.store.ListSheets(ctx)
}

// ListPublicSheets lists sheets marked with sync_public = true.
func (s *Service) ListPublicSheets(ctx context.Context) ([]Sheet, error) {
	return s.store.ListPublicSheets(ctx)
}

// AI Assistant Delegations

// RunNoteAI runs AI assistant on a note.
func (s *Service) RunNoteAI(ctx context.Context, noteID string, req *NoteAIRequest) (*NoteAIResponse, error) {
	if s.ai == nil {
		return nil, errors.New("AI assistant service not configured")
	}
	note, err := s.store.GetNote(ctx, noteID)
	if err != nil {
		return nil, err
	}
	return s.ai.AssistNote(ctx, note, req)
}

// RunSheetAI runs AI assistant on a sheet.
func (s *Service) RunSheetAI(ctx context.Context, sheetID string, req *SheetAIRequest) (*SheetAIResponse, error) {
	if s.ai == nil {
		return nil, errors.New("AI assistant service not configured")
	}
	sheet, err := s.store.GetSheet(ctx, sheetID)
	if err != nil {
		return nil, err
	}
	return s.ai.AssistSheet(ctx, sheet, req)
}
