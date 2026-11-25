package notes

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var (
	// ErrNotFound is returned when a requested note does not exist.
	ErrNotFound = errors.New("note not found")
)

// Store provides SQLite persistence for Pocket Notes.
type Store struct {
	db *sql.DB
}

// NewStore constructs a new notes Store.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Create inserts a new note or checklist into SQLite.
func (s *Store) Create(ctx context.Context, input CreateInput) (Note, error) {
	title := strings.TrimSpace(input.Title)
	content := strings.TrimSpace(input.Content)
	kind := strings.TrimSpace(input.Kind)
	items := input.Items

	if kind == "" {
		if len(items) > 0 {
			kind = "checklist"
		} else if parsed, ok := parseChecklistLines(content); ok {
			kind = "checklist"
			items = parsed
			content = ""
		} else {
			kind = "note"
		}
	}
	if kind != "checklist" {
		kind = "note"
	}
	if items == nil {
		items = []ChecklistItem{}
	}
	if title == "" && content == "" && len(items) == 0 {
		return Note{}, errors.New("กรุณาระบุข้อความหรือรายการอย่างน้อย 1 อย่าง")
	}

	tags := normalizeTags(input.Tags)
	itemsJSON, err := json.Marshal(items)
	if err != nil {
		return Note{}, err
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	pinnedInt := 0
	if input.Pinned {
		pinnedInt = 1
	}

	res, err := s.db.ExecContext(
		ctx,
		`INSERT INTO notes (title, content, kind, items_json, tags, pinned, archived, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, 0, ?, ?)`,
		title, content, kind, string(itemsJSON), tags, pinnedInt, now, now,
	)
	if err != nil {
		return Note{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Note{}, err
	}
	return s.Get(ctx, id)
}

// Get returns a single note by ID.
func (s *Store) Get(ctx context.Context, id int64) (Note, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT id, title, content, kind, items_json, tags, pinned, archived, created_at, updated_at
		 FROM notes WHERE id = ?`,
		id,
	)
	return scanNote(row)
}

// List returns notes ordered by pinned first, then most recently updated.
func (s *Store) List(ctx context.Context, query string, archived bool, limit int) ([]Note, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	archInt := 0
	if archived {
		archInt = 1
	}

	q := strings.TrimSpace(query)
	var rows *sql.Rows
	var err error
	if q != "" {
		like := "%" + q + "%"
		rows, err = s.db.QueryContext(
			ctx,
			`SELECT id, title, content, kind, items_json, tags, pinned, archived, created_at, updated_at
			 FROM notes
			 WHERE archived = ? AND (title LIKE ? OR content LIKE ? OR items_json LIKE ? OR tags LIKE ?)
			 ORDER BY pinned DESC, updated_at DESC, id DESC
			 LIMIT ?`,
			archInt, like, like, like, like, limit,
		)
	} else {
		rows, err = s.db.QueryContext(
			ctx,
			`SELECT id, title, content, kind, items_json, tags, pinned, archived, created_at, updated_at
			 FROM notes
			 WHERE archived = ?
			 ORDER BY pinned DESC, updated_at DESC, id DESC
			 LIMIT ?`,
			archInt, limit,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Note{}
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// Update modifies fields on an existing note.
func (s *Store) Update(ctx context.Context, id int64, input UpdateInput) (Note, error) {
	cur, err := s.Get(ctx, id)
	if err != nil {
		return Note{}, err
	}

	if input.Title != nil {
		cur.Title = strings.TrimSpace(*input.Title)
	}
	if input.Content != nil {
		cur.Content = strings.TrimSpace(*input.Content)
	}
	if input.Kind != nil {
		if *input.Kind == "checklist" {
			cur.Kind = "checklist"
		} else {
			cur.Kind = "note"
		}
	}
	if input.Items != nil {
		cur.Items = *input.Items
		if cur.Items == nil {
			cur.Items = []ChecklistItem{}
		}
	}
	if input.Tags != nil {
		cur.Tags = normalizeTags(*input.Tags)
	}
	if input.Pinned != nil {
		cur.Pinned = *input.Pinned
	}
	if input.Archived != nil {
		cur.Archived = *input.Archived
	}

	itemsJSON, err := json.Marshal(cur.Items)
	if err != nil {
		return Note{}, err
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	pinnedInt, archInt := 0, 0
	if cur.Pinned {
		pinnedInt = 1
	}
	if cur.Archived {
		archInt = 1
	}

	_, err = s.db.ExecContext(
		ctx,
		`UPDATE notes
		 SET title = ?, content = ?, kind = ?, items_json = ?, tags = ?, pinned = ?, archived = ?, updated_at = ?
		 WHERE id = ?`,
		cur.Title, cur.Content, cur.Kind, string(itemsJSON), cur.Tags, pinnedInt, archInt, now, id,
	)
	if err != nil {
		return Note{}, err
	}
	return s.Get(ctx, id)
}

// TogglePin flips the pinned state of a note.
func (s *Store) TogglePin(ctx context.Context, id int64) (Note, error) {
	cur, err := s.Get(ctx, id)
	if err != nil {
		return Note{}, err
	}
	next := !cur.Pinned
	return s.Update(ctx, id, UpdateInput{Pinned: &next})
}

// ToggleArchive flips the archived state of a note.
func (s *Store) ToggleArchive(ctx context.Context, id int64) (Note, error) {
	cur, err := s.Get(ctx, id)
	if err != nil {
		return Note{}, err
	}
	next := !cur.Archived
	return s.Update(ctx, id, UpdateInput{Archived: &next})
}

// Delete permanently removes a note.
func (s *Store) Delete(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM notes WHERE id = ?`, id)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

type noteScanner interface {
	Scan(dest ...any) error
}

func scanNote(s noteScanner) (Note, error) {
	var n Note
	var itemsRaw, createdRaw, updatedRaw string
	var pinnedInt, archInt int
	err := s.Scan(&n.ID, &n.Title, &n.Content, &n.Kind, &itemsRaw, &n.Tags, &pinnedInt, &archInt, &createdRaw, &updatedRaw)
	if errors.Is(err, sql.ErrNoRows) {
		return Note{}, ErrNotFound
	}
	if err != nil {
		return Note{}, err
	}
	n.Pinned = pinnedInt == 1
	n.Archived = archInt == 1
	if err := json.Unmarshal([]byte(itemsRaw), &n.Items); err != nil || n.Items == nil {
		n.Items = []ChecklistItem{}
	}
	n.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdRaw)
	n.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedRaw)
	return n, nil
}

func parseChecklistLines(content string) ([]ChecklistItem, bool) {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	var items []ChecklistItem
	hasCheckbox := false
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "- [ ] ") || strings.HasPrefix(line, "* [ ] ") {
			hasCheckbox = true
			items = append(items, ChecklistItem{Text: strings.TrimSpace(line[6:]), Done: false})
		} else if strings.HasPrefix(line, "- [x] ") || strings.HasPrefix(line, "- [X] ") ||
			strings.HasPrefix(line, "* [x] ") || strings.HasPrefix(line, "* [X] ") {
			hasCheckbox = true
			items = append(items, ChecklistItem{Text: strings.TrimSpace(line[6:]), Done: true})
		} else {
			items = append(items, ChecklistItem{Text: strings.TrimPrefix(strings.TrimPrefix(line, "- "), "* "), Done: false})
		}
	}
	if !hasCheckbox || len(items) == 0 {
		return nil, false
	}
	return items, true
}

func normalizeTags(raw string) string {
	seen := make(map[string]struct{})
	var out []string
	for _, part := range strings.Split(raw, ",") {
		t := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(part), "#"))
		if t == "" {
			continue
		}
		key := strings.ToLower(t)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, t)
	}
	return strings.Join(out, ",")
}
