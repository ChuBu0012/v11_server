package workspace

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func setupTestRouter(t *testing.T) (*gin.Engine, *Service) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	store := NewStore(db)
	svc := NewService(store, nil)
	mod := NewModule(svc)

	r := gin.New()
	mod.RegisterRoutes(r)
	return r, svc
}

func TestHandler_NotesEndpoints(t *testing.T) {
	r, _ := setupTestRouter(t)

	// 1. POST /api/notes
	createBody := CreateNoteRequest{
		Title:      "เทสต์โน้ต",
		Content:    "- [ ] ทำงาน #todo",
		Tags:       []string{"work"},
		IsPinned:   true,
		SyncPublic: false,
	}
	bodyBytes, _ := json.Marshal(createBody)

	req := httptest.NewRequest(http.MethodPost, "/api/notes", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/notes status = %d, want 201: %s", rec.Code, rec.Body.String())
	}

	var created Note
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if created.ID == "" || created.Title != "เทสต์โน้ต" {
		t.Errorf("unexpected created note: %+v", created)
	}

	// 2. GET /api/notes
	req = httptest.NewRequest(http.MethodGet, "/api/notes?tag=todo", nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/notes status = %d, want 200", rec.Code)
	}
	var notes []Note
	if err := json.Unmarshal(rec.Body.Bytes(), &notes); err != nil {
		t.Fatalf("failed to decode notes list: %v", err)
	}
	if len(notes) != 1 || notes[0].ID != created.ID {
		t.Errorf("unexpected notes list: %v", notes)
	}

	// 3. GET /api/notes/:id
	req = httptest.NewRequest(http.MethodGet, "/api/notes/"+created.ID, nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/notes/:id status = %d, want 200", rec.Code)
	}

	// 4. PUT /api/notes/:id
	newTitle := "เทสต์โน้ตแก้ไขแล้ว"
	updateBody := UpdateNoteRequest{
		Title: &newTitle,
	}
	upBytes, _ := json.Marshal(updateBody)
	req = httptest.NewRequest(http.MethodPut, "/api/notes/"+created.ID, bytes.NewReader(upBytes))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("PUT /api/notes/:id status = %d, want 200", rec.Code)
	}

	// 5. DELETE /api/notes/:id
	req = httptest.NewRequest(http.MethodDelete, "/api/notes/"+created.ID, nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE /api/notes/:id status = %d, want 200", rec.Code)
	}

	// 6. Verify 404 after deletion
	req = httptest.NewRequest(http.MethodGet, "/api/notes/"+created.ID, nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET /api/notes/:id after deletion status = %d, want 404", rec.Code)
	}
}

func TestHandler_SheetsEndpoints(t *testing.T) {
	r, _ := setupTestRouter(t)

	// 1. POST /api/sheets
	createBody := CreateSheetRequest{
		Title:      "ชีตงบประมาณ",
		RowCount:   15,
		ColCount:   5,
		DataJSON:   `{"cells":{"A1":{"raw":"100","v":100}}}`,
		SyncPublic: true,
	}
	bodyBytes, _ := json.Marshal(createBody)

	req := httptest.NewRequest(http.MethodPost, "/api/sheets", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/sheets status = %d, want 201: %s", rec.Code, rec.Body.String())
	}

	var created Sheet
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to decode sheet: %v", err)
	}
	if created.ID == "" || created.Title != "ชีตงบประมาณ" {
		t.Errorf("unexpected sheet: %+v", created)
	}

	// 2. GET /api/sheets
	req = httptest.NewRequest(http.MethodGet, "/api/sheets", nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/sheets status = %d, want 200", rec.Code)
	}
	var headers []SheetHeader
	if err := json.Unmarshal(rec.Body.Bytes(), &headers); err != nil {
		t.Fatalf("failed to decode sheet headers: %v", err)
	}
	if len(headers) != 1 || headers[0].ID != created.ID {
		t.Errorf("unexpected headers: %v", headers)
	}

	// 3. GET /api/sheets/:id
	req = httptest.NewRequest(http.MethodGet, "/api/sheets/"+created.ID, nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/sheets/:id status = %d, want 200", rec.Code)
	}

	// 4. PUT /api/sheets/:id
	newTitle := "ชีตปรับปรุงใหม่"
	updateBody := UpdateSheetRequest{
		Title: &newTitle,
	}
	upBytes, _ := json.Marshal(updateBody)
	req = httptest.NewRequest(http.MethodPut, "/api/sheets/"+created.ID, bytes.NewReader(upBytes))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("PUT /api/sheets/:id status = %d, want 200", rec.Code)
	}

	// 5. DELETE /api/sheets/:id
	req = httptest.NewRequest(http.MethodDelete, "/api/sheets/"+created.ID, nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE /api/sheets/:id status = %d, want 200", rec.Code)
	}

	// 6. Verify 404 after deletion
	req = httptest.NewRequest(http.MethodGet, "/api/sheets/"+created.ID, nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET /api/sheets/:id after deletion status = %d, want 404", rec.Code)
	}
}
