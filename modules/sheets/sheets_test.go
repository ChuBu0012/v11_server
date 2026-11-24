package sheets

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite: %v", err)
	}

	schema := `
	CREATE TABLE IF NOT EXISTS sheets (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		data_json TEXT NOT NULL DEFAULT '[]',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}
	return db
}

type mockAI struct {
	response string
	hasKey   bool
}

func (m *mockAI) Generate(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	return m.response, nil
}

func (m *mockAI) HasAPIKey() bool {
	return m.hasKey
}

func TestStore_CRUD(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	store := NewStore(db)
	ctx := context.Background()

	// 1. Create
	sh, err := store.Create(ctx, CreateSheetRequest{
		Title:    "งบประมาณ 2026",
		DataJSON: `[{"name":"sheet1"}]`,
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if sh.ID == "" || sh.Title != "งบประมาณ 2026" {
		t.Fatalf("unexpected created sheet: %+v", sh)
	}

	// 2. Get
	fetched, err := store.Get(ctx, sh.ID)
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if fetched.DataJSON != `[{"name":"sheet1"}]` {
		t.Fatalf("unexpected fetched data: %s", fetched.DataJSON)
	}

	// 3. List
	list, err := store.List(ctx)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 sheet, got %d", len(list))
	}

	// 4. Update
	updated, err := store.Update(ctx, sh.ID, UpdateSheetRequest{
		Title:    "งบประมาณ 2026 (แก้ไข)",
		DataJSON: `[{"name":"sheet1","rows":{"0":{"cells":{"0":{"text":"รายรับ"}}}}}]`,
	})
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if updated.Title != "งบประมาณ 2026 (แก้ไข)" {
		t.Fatalf("unexpected updated title: %s", updated.Title)
	}

	// 5. Delete
	if err := store.Delete(ctx, sh.ID); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	_, err = store.Get(ctx, sh.ID)
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestHandler_Routes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer db.Close()

	store := NewStore(db)
	ai := &mockAI{response: "=SUM(A1:A5)", hasKey: true}
	handler := NewHandler(store, ai)

	r := gin.New()
	api := r.Group("/api")
	handler.RegisterRoutes(api)

	// 1. Create Sheet
	body, _ := json.Marshal(CreateSheetRequest{Title: "เทสต์ตาราง", DataJSON: "[]"})
	req := httptest.NewRequest(http.MethodPost, "/api/sheets", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("POST /api/sheets status = %d, want %d: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var created Sheet
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// 2. GET /api/sheets
	req = httptest.NewRequest(http.MethodGet, "/api/sheets", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/sheets status = %d", w.Code)
	}

	// 3. GET /api/sheets/:id
	req = httptest.NewRequest(http.MethodGet, "/api/sheets/"+created.ID, nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/sheets/:id status = %d", w.Code)
	}

	// 4. POST /api/sheets/:id/ai
	aiBody, _ := json.Marshal(AIRequest{Action: "formula", Prompt: "ขอสูตรคำนวณผลรวม A1 ถึง A5"})
	req = httptest.NewRequest(http.MethodPost, "/api/sheets/"+created.ID+"/ai", bytes.NewReader(aiBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("POST /api/sheets/:id/ai status = %d", w.Code)
	}

	// 5. DELETE /api/sheets/:id
	req = httptest.NewRequest(http.MethodDelete, "/api/sheets/"+created.ID, nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("DELETE /api/sheets/:id status = %d", w.Code)
	}
}
