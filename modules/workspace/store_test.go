package workspace

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open memory sqlite: %v", err)
	}

	createTables := `
	CREATE TABLE workspace_notes (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		content TEXT NOT NULL DEFAULT '',
		tags TEXT NOT NULL DEFAULT '',
		is_pinned INTEGER NOT NULL DEFAULT 0,
		sync_public INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX idx_workspace_notes_updated ON workspace_notes(updated_at DESC);
	CREATE INDEX idx_workspace_notes_pinned ON workspace_notes(is_pinned DESC, updated_at DESC);

	CREATE TABLE workspace_sheets (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		row_count INTEGER NOT NULL DEFAULT 30,
		col_count INTEGER NOT NULL DEFAULT 10,
		data_json TEXT NOT NULL DEFAULT '{}',
		sync_public INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX idx_workspace_sheets_updated ON workspace_sheets(updated_at DESC);
	`
	if _, err := db.Exec(createTables); err != nil {
		t.Fatalf("failed to create tables: %v", err)
	}

	t.Cleanup(func() {
		_ = db.Close()
	})
	return db
}

func TestStore_NoteCRUD(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	ctx := context.Background()

	// 1. Create note
	created, err := store.CreateNote(ctx, &CreateNoteRequest{
		Title:      "รายการซื้อของ",
		Content:    "- [ ] นมสด\n- [ ] ขนมปัง",
		Tags:       []string{"todo", "groceries"},
		IsPinned:   false,
		SyncPublic: false,
	})
	if err != nil {
		t.Fatalf("CreateNote failed: %v", err)
	}
	if created.ID == "" {
		t.Fatalf("expected non-empty ID")
	}
	if created.Title != "รายการซื้อของ" {
		t.Errorf("expected title 'รายการซื้อของ', got %q", created.Title)
	}
	if len(created.Tags) != 2 || created.Tags[0] != "todo" {
		t.Errorf("unexpected tags: %v", created.Tags)
	}

	// 2. Get note
	fetched, err := store.GetNote(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetNote failed: %v", err)
	}
	if fetched.Content != created.Content {
		t.Errorf("content mismatch: got %q, want %q", fetched.Content, created.Content)
	}

	// 3. Update note
	newTitle := "รายการซื้อของด่วน"
	isPinned := true
	updated, err := store.UpdateNote(ctx, created.ID, &UpdateNoteRequest{
		Title:    &newTitle,
		IsPinned: &isPinned,
	})
	if err != nil {
		t.Fatalf("UpdateNote failed: %v", err)
	}
	if updated.Title != newTitle || !updated.IsPinned {
		t.Errorf("update failed to apply: %+v", updated)
	}

	// 4. List notes with search and tags
	_, err = store.CreateNote(ctx, &CreateNoteRequest{
		Title:      "แผนงานโปรเจกต์",
		Content:    "เขียนสเปกชีต",
		Tags:       []string{"dev"},
		IsPinned:   false,
		SyncPublic: true,
	})
	if err != nil {
		t.Fatalf("CreateNote 2 failed: %v", err)
	}

	list, err := store.ListNotes(ctx, NoteFilter{})
	if err != nil {
		t.Fatalf("ListNotes failed: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 notes, got %d", len(list))
	}
	// Pinned note should be first
	if list[0].ID != created.ID {
		t.Errorf("expected pinned note to be first, got %s", list[0].ID)
	}

	// Filter by tag
	tagged, err := store.ListNotes(ctx, NoteFilter{Tag: "dev"})
	if err != nil {
		t.Fatalf("ListNotes with tag failed: %v", err)
	}
	if len(tagged) != 1 || tagged[0].Title != "แผนงานโปรเจกต์" {
		t.Errorf("unexpected tagged results: %v", tagged)
	}

	// Filter by search keyword
	searched, err := store.ListNotes(ctx, NoteFilter{Search: "ขนมปัง"})
	if err != nil {
		t.Fatalf("ListNotes with search failed: %v", err)
	}
	if len(searched) != 1 || searched[0].ID != created.ID {
		t.Errorf("unexpected searched results: %v", searched)
	}

	// 5. Delete note
	if err := store.DeleteNote(ctx, created.ID); err != nil {
		t.Fatalf("DeleteNote failed: %v", err)
	}
	_, err = store.GetNote(ctx, created.ID)
	if err != ErrNoteNotFound {
		t.Errorf("expected ErrNoteNotFound, got %v", err)
	}
}

func TestStore_SheetCRUD(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	ctx := context.Background()

	// 1. Create sheet
	created, err := store.CreateSheet(ctx, &CreateSheetRequest{
		Title:      "งบประมาณ 2026",
		RowCount:   20,
		ColCount:   5,
		DataJSON:   `{"cells":{"A1":{"raw":"ค่าไฟ","v":"ค่าไฟ"}}}`,
		SyncPublic: true,
	})
	if err != nil {
		t.Fatalf("CreateSheet failed: %v", err)
	}
	if created.ID == "" {
		t.Fatalf("expected non-empty sheet ID")
	}
	if created.RowCount != 20 || created.ColCount != 5 {
		t.Errorf("unexpected dimensions: %dx%d", created.RowCount, created.ColCount)
	}

	// 2. Get sheet
	fetched, err := store.GetSheet(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetSheet failed: %v", err)
	}
	if fetched.Title != "งบประมาณ 2026" {
		t.Errorf("unexpected title: %q", fetched.Title)
	}

	// 3. Update sheet
	newTitle := "งบประมาณปรับปรุง"
	newData := `{"cells":{"A1":{"raw":"100","v":100}}}`
	updated, err := store.UpdateSheet(ctx, created.ID, &UpdateSheetRequest{
		Title:    &newTitle,
		DataJSON: &newData,
	})
	if err != nil {
		t.Fatalf("UpdateSheet failed: %v", err)
	}
	if updated.Title != newTitle || updated.DataJSON != newData {
		t.Errorf("update failed: %+v", updated)
	}

	// 4. List sheets
	sheets, err := store.ListSheets(ctx)
	if err != nil {
		t.Fatalf("ListSheets failed: %v", err)
	}
	if len(sheets) != 1 || sheets[0].ID != created.ID {
		t.Errorf("unexpected sheet list: %v", sheets)
	}

	// 5. Public sheets
	pubSheets, err := store.ListPublicSheets(ctx)
	if err != nil {
		t.Fatalf("ListPublicSheets failed: %v", err)
	}
	if len(pubSheets) != 1 {
		t.Errorf("expected 1 public sheet, got %d", len(pubSheets))
	}

	// 6. Delete sheet
	if err := store.DeleteSheet(ctx, created.ID); err != nil {
		t.Fatalf("DeleteSheet failed: %v", err)
	}
	_, err = store.GetSheet(ctx, created.ID)
	if err != ErrSheetNotFound {
		t.Errorf("expected ErrSheetNotFound, got %v", err)
	}
}
