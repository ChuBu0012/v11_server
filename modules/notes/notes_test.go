package notes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"

	"v11-server/shared/db"
)

func setupTestNotes(t *testing.T) (*Store, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	database, err := db.InitDB(filepath.Join(t.TempDir(), "notes.db"))
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	store := NewStore(database)
	bridge := func(ctx context.Context, rawText string) (string, string, error) {
		return "เตือน: " + rawText, "2026-11-26T09:00:00+07:00", nil
	}
	mod := NewModule(store, nil, bridge)
	r := gin.New()
	mod.RegisterRoutes(r)
	return store, r
}

func TestStoreCRUDAndChecklist(t *testing.T) {
	store, _ := setupTestNotes(t)
	ctx := context.Background()

	// 1. Create plain note
	n1, err := store.Create(ctx, CreateInput{
		Title:   "ไอเดียแอป",
		Content: "ทำหน้าโน้ตมินิมอล",
		Tags:    "#งาน, #ไอเดีย, งาน",
	})
	if err != nil {
		t.Fatalf("Create note failed: %v", err)
	}
	if n1.Kind != "note" || n1.Tags != "งาน,ไอเดีย" {
		t.Fatalf("unexpected note: %+v", n1)
	}

	// 2. Create checklist via markdown lines
	n2, err := store.Create(ctx, CreateInput{
		Title:   "ซื้อของเข้าบ้าน",
		Content: "- [ ] นมสด\n- [x] ไข่ไก่",
		Pinned:  true,
	})
	if err != nil {
		t.Fatalf("Create checklist failed: %v", err)
	}
	if n2.Kind != "checklist" || len(n2.Items) != 2 || n2.Items[0].Done || !n2.Items[1].Done {
		t.Fatalf("unexpected checklist: %+v", n2)
	}

	// 3. List ordering: pinned first
	list, err := store.List(ctx, "", false, 50)
	if err != nil || len(list) != 2 {
		t.Fatalf("List failed: len=%d err=%v", len(list), err)
	}
	if list[0].ID != n2.ID {
		t.Errorf("expected pinned note first, got id=%d", list[0].ID)
	}

	// 4. Search filter
	filtered, err := store.List(ctx, "นมสด", false, 50)
	if err != nil || len(filtered) != 1 || filtered[0].ID != n2.ID {
		t.Fatalf("Search failed: %+v err=%v", filtered, err)
	}

	// 5. Toggle pin & archive
	unpinned, err := store.TogglePin(ctx, n2.ID)
	if err != nil || unpinned.Pinned {
		t.Fatalf("TogglePin failed: %+v err=%v", unpinned, err)
	}
	archived, err := store.ToggleArchive(ctx, n1.ID)
	if err != nil || !archived.Archived {
		t.Fatalf("ToggleArchive failed: %+v err=%v", archived, err)
	}
	activeOnly, _ := store.List(ctx, "", false, 50)
	if len(activeOnly) != 1 {
		t.Errorf("expected 1 active note, got %d", len(activeOnly))
	}

	// 6. Delete
	if err := store.Delete(ctx, n1.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if _, err := store.Get(ctx, n1.ID); err == nil {
		t.Errorf("expected ErrNotFound after delete")
	}
}

func TestNotesHTTPHandlers(t *testing.T) {
	_, r := setupTestNotes(t)

	// POST /api/notes
	body := `{"title":"จ่ายค่าไฟ","content":"กำหนดจ่ายพรุ่งนี้เช้า","kind":"note"}`
	req := httptest.NewRequest(http.MethodPost, "/api/notes", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("POST /api/notes status=%d body=%s", rr.Code, rr.Body.String())
	}

	var created Note
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	// POST /api/notes/:id/auto-tag
	rrTag := httptest.NewRecorder()
	r.ServeHTTP(rrTag, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/notes/%d/auto-tag", created.ID), nil))
	if rrTag.Code != http.StatusOK {
		t.Fatalf("auto-tag status=%d", rrTag.Code)
	}
	var tagged Note
	_ = json.Unmarshal(rrTag.Body.Bytes(), &tagged)
	if tagged.Tags == "" {
		t.Errorf("expected non-empty fallback tag")
	}

	// POST /api/notes/:id/to-schedule
	rrSched := httptest.NewRecorder()
	r.ServeHTTP(rrSched, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/notes/%d/to-schedule", created.ID), nil))
	if rrSched.Code != http.StatusOK {
		t.Fatalf("to-schedule status=%d body=%s", rrSched.Code, rrSched.Body.String())
	}

	// GET /api/notes
	rrList := httptest.NewRecorder()
	r.ServeHTTP(rrList, httptest.NewRequest(http.MethodGet, "/api/notes?q=ค่าไฟ", nil))
	if rrList.Code != http.StatusOK || !bytes.Contains(rrList.Body.Bytes(), []byte("จ่ายค่าไฟ")) {
		t.Fatalf("GET /api/notes failed: %d %s", rrList.Code, rrList.Body.String())
	}

	// DELETE /api/notes/:id
	rrDel := httptest.NewRecorder()
	r.ServeHTTP(rrDel, httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/notes/%d", created.ID), nil))
	if rrDel.Code != http.StatusOK {
		t.Fatalf("DELETE /api/notes status=%d", rrDel.Code)
	}
}
