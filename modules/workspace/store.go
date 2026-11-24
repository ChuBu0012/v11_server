package workspace

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrNoteNotFound  = errors.New("note not found")
	ErrSheetNotFound = errors.New("sheet not found")
)

// Store handles SQLite persistence for notes and sheets.
type Store struct {
	db *sql.DB
}

// NewStore initializes a new workspace store.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// generateID generates a unique prefixed ID (e.g. nt_a1b2c3d4, sh_1a2b3c4d).
func generateID(prefix string) string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(b))
}

// parseTags splits comma-separated string into string slice.
func parseTags(tagsStr string) []string {
	if strings.TrimSpace(tagsStr) == "" {
		return []string{}
	}
	parts := strings.Split(tagsStr, ",")
	tags := make([]string, 0, len(parts))
	for _, p := range parts {
		cleaned := strings.TrimSpace(p)
		if cleaned != "" {
			tags = append(tags, cleaned)
		}
	}
	return tags
}

// formatTags joins tags slice into comma-separated string.
func formatTags(tags []string) string {
	cleaned := make([]string, 0, len(tags))
	for _, t := range tags {
		c := strings.TrimSpace(strings.TrimPrefix(t, "#"))
		if c != "" {
			cleaned = append(cleaned, c)
		}
	}
	return strings.Join(cleaned, ",")
}

// CreateNote inserts a new note into SQLite.
func (s *Store) CreateNote(ctx context.Context, req *CreateNoteRequest) (*Note, error) {
	id := generateID("nt")
	tagsStr := formatTags(req.Tags)
	pinnedInt := 0
	if req.IsPinned {
		pinnedInt = 1
	}
	syncInt := 0
	if req.SyncPublic {
		syncInt = 1
	}

	query := `
	INSERT INTO workspace_notes (id, title, content, tags, is_pinned, sync_public, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`
	_, err := s.db.ExecContext(ctx, query, id, req.Title, req.Content, tagsStr, pinnedInt, syncInt)
	if err != nil {
		return nil, fmt.Errorf("failed to create note: %w", err)
	}

	return s.GetNote(ctx, id)
}

// GetNote retrieves a single note by ID.
func (s *Store) GetNote(ctx context.Context, id string) (*Note, error) {
	query := `
	SELECT id, title, content, tags, is_pinned, sync_public, created_at, updated_at
	FROM workspace_notes
	WHERE id = ?
	`
	row := s.db.QueryRowContext(ctx, query, id)

	var n Note
	var tagsStr string
	var pinnedInt, syncInt int
	var createdAtStr, updatedAtStr string

	err := row.Scan(&n.ID, &n.Title, &n.Content, &tagsStr, &pinnedInt, &syncInt, &createdAtStr, &updatedAtStr)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoteNotFound
		}
		return nil, fmt.Errorf("failed to get note %s: %w", id, err)
	}

	n.Tags = parseTags(tagsStr)
	n.IsPinned = pinnedInt == 1
	n.SyncPublic = syncInt == 1
	n.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
	if n.CreatedAt.IsZero() {
		n.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAtStr)
	}
	n.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAtStr)
	if n.UpdatedAt.IsZero() {
		n.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAtStr)
	}

	return &n, nil
}

// UpdateNote updates fields of an existing note.
func (s *Store) UpdateNote(ctx context.Context, id string, req *UpdateNoteRequest) (*Note, error) {
	// First verify note exists
	existing, err := s.GetNote(ctx, id)
	if err != nil {
		return nil, err
	}

	title := existing.Title
	if req.Title != nil {
		title = *req.Title
	}

	content := existing.Content
	if req.Content != nil {
		content = *req.Content
	}

	tagsStr := formatTags(existing.Tags)
	if req.Tags != nil {
		tagsStr = formatTags(*req.Tags)
	}

	pinnedInt := 0
	if existing.IsPinned {
		pinnedInt = 1
	}
	if req.IsPinned != nil {
		if *req.IsPinned {
			pinnedInt = 1
		} else {
			pinnedInt = 0
		}
	}

	syncInt := 0
	if existing.SyncPublic {
		syncInt = 1
	}
	if req.SyncPublic != nil {
		if *req.SyncPublic {
			syncInt = 1
		} else {
			syncInt = 0
		}
	}

	query := `
	UPDATE workspace_notes
	SET title = ?, content = ?, tags = ?, is_pinned = ?, sync_public = ?, updated_at = CURRENT_TIMESTAMP
	WHERE id = ?
	`
	res, err := s.db.ExecContext(ctx, query, title, content, tagsStr, pinnedInt, syncInt, id)
	if err != nil {
		return nil, fmt.Errorf("failed to update note %s: %w", id, err)
	}
	rowsAff, _ := res.RowsAffected()
	if rowsAff == 0 {
		return nil, ErrNoteNotFound
	}

	return s.GetNote(ctx, id)
}

// DeleteNote deletes a note by ID.
func (s *Store) DeleteNote(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM workspace_notes WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete note %s: %w", id, err)
	}
	rowsAff, _ := res.RowsAffected()
	if rowsAff == 0 {
		return ErrNoteNotFound
	}
	return nil
}

// ListNotes retrieves notes according to filters.
func (s *Store) ListNotes(ctx context.Context, filter NoteFilter) ([]Note, error) {
	var whereClauses []string
	var args []any

	if filter.Tag != "" {
		tag := strings.TrimPrefix(filter.Tag, "#")
		whereClauses = append(whereClauses, "instr(',' || tags || ',', ',' || ? || ',') > 0")
		args = append(args, tag)
	}

	if filter.Search != "" {
		like := "%" + filter.Search + "%"
		whereClauses = append(whereClauses, "(title LIKE ? OR content LIKE ? OR tags LIKE ?)")
		args = append(args, like, like, like)
	}

	query := "SELECT id, title, content, tags, is_pinned, sync_public, created_at, updated_at FROM workspace_notes"
	if len(whereClauses) > 0 {
		query += " WHERE " + strings.Join(whereClauses, " AND ")
	}
	query += " ORDER BY is_pinned DESC, updated_at DESC"

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
		if filter.Offset > 0 {
			query += fmt.Sprintf(" OFFSET %d", filter.Offset)
		}
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list notes: %w", err)
	}
	defer rows.Close()

	notes := make([]Note, 0)
	for rows.Next() {
		var n Note
		var tagsStr string
		var pinnedInt, syncInt int
		var createdAtStr, updatedAtStr string

		if err := rows.Scan(&n.ID, &n.Title, &n.Content, &tagsStr, &pinnedInt, &syncInt, &createdAtStr, &updatedAtStr); err != nil {
			return nil, fmt.Errorf("failed to scan note row: %w", err)
		}

		n.Tags = parseTags(tagsStr)
		n.IsPinned = pinnedInt == 1
		n.SyncPublic = syncInt == 1
		n.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
		if n.CreatedAt.IsZero() {
			n.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAtStr)
		}
		n.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAtStr)
		if n.UpdatedAt.IsZero() {
			n.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAtStr)
		}
		notes = append(notes, n)
	}

	return notes, rows.Err()
}

// ListPublicNotes retrieves notes with sync_public = 1.
func (s *Store) ListPublicNotes(ctx context.Context) ([]Note, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, title, content, tags, is_pinned, sync_public, created_at, updated_at
		FROM workspace_notes
		WHERE sync_public = 1
		ORDER BY is_pinned DESC, updated_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query public notes: %w", err)
	}
	defer rows.Close()

	var notes []Note
	for rows.Next() {
		var n Note
		var tagsStr string
		var pinnedInt, syncInt int
		var createdAtStr, updatedAtStr string

		if err := rows.Scan(&n.ID, &n.Title, &n.Content, &tagsStr, &pinnedInt, &syncInt, &createdAtStr, &updatedAtStr); err != nil {
			return nil, err
		}
		n.Tags = parseTags(tagsStr)
		n.IsPinned = pinnedInt == 1
		n.SyncPublic = syncInt == 1
		n.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
		if n.CreatedAt.IsZero() {
			n.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAtStr)
		}
		n.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAtStr)
		if n.UpdatedAt.IsZero() {
			n.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAtStr)
		}
		notes = append(notes, n)
	}
	return notes, rows.Err()
}

// CreateSheet inserts a new sheet into SQLite.
func (s *Store) CreateSheet(ctx context.Context, req *CreateSheetRequest) (*Sheet, error) {
	id := generateID("sh")
	rowCnt := req.RowCount
	if rowCnt <= 0 {
		rowCnt = 30
	}
	colCnt := req.ColCount
	if colCnt <= 0 {
		colCnt = 10
	}
	dataJSON := req.DataJSON
	if strings.TrimSpace(dataJSON) == "" {
		dataJSON = "{}"
	}

	syncInt := 0
	if req.SyncPublic {
		syncInt = 1
	}

	query := `
	INSERT INTO workspace_sheets (id, title, row_count, col_count, data_json, sync_public, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`
	_, err := s.db.ExecContext(ctx, query, id, req.Title, rowCnt, colCnt, dataJSON, syncInt)
	if err != nil {
		return nil, fmt.Errorf("failed to create sheet: %w", err)
	}

	return s.GetSheet(ctx, id)
}

// GetSheet retrieves full sheet by ID.
func (s *Store) GetSheet(ctx context.Context, id string) (*Sheet, error) {
	query := `
	SELECT id, title, row_count, col_count, data_json, sync_public, created_at, updated_at
	FROM workspace_sheets
	WHERE id = ?
	`
	row := s.db.QueryRowContext(ctx, query, id)

	var sh Sheet
	var syncInt int
	var createdAtStr, updatedAtStr string

	err := row.Scan(&sh.ID, &sh.Title, &sh.RowCount, &sh.ColCount, &sh.DataJSON, &syncInt, &createdAtStr, &updatedAtStr)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrSheetNotFound
		}
		return nil, fmt.Errorf("failed to get sheet %s: %w", id, err)
	}

	sh.SyncPublic = syncInt == 1
	sh.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
	if sh.CreatedAt.IsZero() {
		sh.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAtStr)
	}
	sh.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAtStr)
	if sh.UpdatedAt.IsZero() {
		sh.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAtStr)
	}

	return &sh, nil
}

// UpdateSheet updates an existing sheet.
func (s *Store) UpdateSheet(ctx context.Context, id string, req *UpdateSheetRequest) (*Sheet, error) {
	existing, err := s.GetSheet(ctx, id)
	if err != nil {
		return nil, err
	}

	title := existing.Title
	if req.Title != nil {
		title = *req.Title
	}

	rowCount := existing.RowCount
	if req.RowCount != nil {
		rowCount = *req.RowCount
	}

	colCount := existing.ColCount
	if req.ColCount != nil {
		colCount = *req.ColCount
	}

	dataJSON := existing.DataJSON
	if req.DataJSON != nil {
		dataJSON = *req.DataJSON
	}

	syncInt := 0
	if existing.SyncPublic {
		syncInt = 1
	}
	if req.SyncPublic != nil {
		if *req.SyncPublic {
			syncInt = 1
		} else {
			syncInt = 0
		}
	}

	query := `
	UPDATE workspace_sheets
	SET title = ?, row_count = ?, col_count = ?, data_json = ?, sync_public = ?, updated_at = CURRENT_TIMESTAMP
	WHERE id = ?
	`
	res, err := s.db.ExecContext(ctx, query, title, rowCount, colCount, dataJSON, syncInt, id)
	if err != nil {
		return nil, fmt.Errorf("failed to update sheet %s: %w", id, err)
	}
	rowsAff, _ := res.RowsAffected()
	if rowsAff == 0 {
		return nil, ErrSheetNotFound
	}

	return s.GetSheet(ctx, id)
}

// DeleteSheet deletes a sheet by ID.
func (s *Store) DeleteSheet(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM workspace_sheets WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete sheet %s: %w", id, err)
	}
	rowsAff, _ := res.RowsAffected()
	if rowsAff == 0 {
		return ErrSheetNotFound
	}
	return nil
}

// ListSheets returns summary headers of all sheets.
func (s *Store) ListSheets(ctx context.Context) ([]SheetHeader, error) {
	query := `
	SELECT id, title, row_count, col_count, sync_public, created_at, updated_at
	FROM workspace_sheets
	ORDER BY updated_at DESC
	`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list sheets: %w", err)
	}
	defer rows.Close()

	sheets := make([]SheetHeader, 0)
	for rows.Next() {
		var sh SheetHeader
		var syncInt int
		var createdAtStr, updatedAtStr string

		if err := rows.Scan(&sh.ID, &sh.Title, &sh.RowCount, &sh.ColCount, &syncInt, &createdAtStr, &updatedAtStr); err != nil {
			return nil, fmt.Errorf("failed to scan sheet row: %w", err)
		}

		sh.SyncPublic = syncInt == 1
		sh.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
		if sh.CreatedAt.IsZero() {
			sh.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAtStr)
		}
		sh.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAtStr)
		if sh.UpdatedAt.IsZero() {
			sh.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAtStr)
		}

		sheets = append(sheets, sh)
	}

	return sheets, rows.Err()
}

// ListPublicSheets retrieves full sheets with sync_public = 1.
func (s *Store) ListPublicSheets(ctx context.Context) ([]Sheet, error) {
	query := `
	SELECT id, title, row_count, col_count, data_json, sync_public, created_at, updated_at
	FROM workspace_sheets
	WHERE sync_public = 1
	ORDER BY updated_at DESC
	`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list public sheets: %w", err)
	}
	defer rows.Close()

	var sheets []Sheet
	for rows.Next() {
		var sh Sheet
		var syncInt int
		var createdAtStr, updatedAtStr string

		if err := rows.Scan(&sh.ID, &sh.Title, &sh.RowCount, &sh.ColCount, &sh.DataJSON, &syncInt, &createdAtStr, &updatedAtStr); err != nil {
			return nil, err
		}
		sh.SyncPublic = syncInt == 1
		sh.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
		if sh.CreatedAt.IsZero() {
			sh.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAtStr)
		}
		sh.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAtStr)
		if sh.UpdatedAt.IsZero() {
			sh.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAtStr)
		}
		sheets = append(sheets, sh)
	}
	return sheets, rows.Err()
}
