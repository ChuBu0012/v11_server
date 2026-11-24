package sheets

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
	ErrNotFound = errors.New("sheet not found")
)

// Store handles persistence of spreadsheet documents in SQLite.
type Store struct {
	db *sql.DB
}

// NewStore creates a new sheets Store.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func generateID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return "sh_" + hex.EncodeToString(b)
}

// Create inserts a new sheet.
func (s *Store) Create(ctx context.Context, req CreateSheetRequest) (*Sheet, error) {
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = "สเปรดชีตไม่มีชื่อ"
	}
	data := strings.TrimSpace(req.DataJSON)
	if data == "" {
		data = "[]"
	}

	sheet := &Sheet{
		ID:        generateID(),
		Title:     title,
		DataJSON:  data,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	query := `INSERT INTO sheets (id, title, data_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`
	_, err := s.db.ExecContext(ctx, query, sheet.ID, sheet.Title, sheet.DataJSON, sheet.CreatedAt, sheet.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create sheet: %w", err)
	}
	return sheet, nil
}

// Get returns a single sheet by ID.
func (s *Store) Get(ctx context.Context, id string) (*Sheet, error) {
	query := `SELECT id, title, data_json, created_at, updated_at FROM sheets WHERE id = ?`
	row := s.db.QueryRowContext(ctx, query, id)

	var sh Sheet
	err := row.Scan(&sh.ID, &sh.Title, &sh.DataJSON, &sh.CreatedAt, &sh.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get sheet: %w", err)
	}
	return &sh, nil
}

// List returns all sheets ordered by last updated.
func (s *Store) List(ctx context.Context) ([]*Sheet, error) {
	query := `SELECT id, title, data_json, created_at, updated_at FROM sheets ORDER BY updated_at DESC`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list sheets: %w", err)
	}
	defer rows.Close()

	var sheets []*Sheet
	for rows.Next() {
		var sh Sheet
		if err := rows.Scan(&sh.ID, &sh.Title, &sh.DataJSON, &sh.CreatedAt, &sh.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan sheet: %w", err)
		}
		sheets = append(sheets, &sh)
	}
	return sheets, rows.Err()
}

// Update updates a sheet's title and/or data_json.
func (s *Store) Update(ctx context.Context, id string, req UpdateSheetRequest) (*Sheet, error) {
	existing, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	title := strings.TrimSpace(req.Title)
	if title != "" {
		existing.Title = title
	}
	if strings.TrimSpace(req.DataJSON) != "" {
		existing.DataJSON = strings.TrimSpace(req.DataJSON)
	}
	existing.UpdatedAt = time.Now().UTC()

	query := `UPDATE sheets SET title = ?, data_json = ?, updated_at = ? WHERE id = ?`
	_, err = s.db.ExecContext(ctx, query, existing.Title, existing.DataJSON, existing.UpdatedAt, id)
	if err != nil {
		return nil, fmt.Errorf("update sheet: %w", err)
	}
	return existing, nil
}

// Delete removes a sheet.
func (s *Store) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM sheets WHERE id = ?`
	res, err := s.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete sheet: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
