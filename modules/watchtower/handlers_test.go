package watchtower

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func setupTestRouter(svc *Service) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	handler := NewHandler(svc)
	handler.RegisterRoutes(r)
	return r
}

func TestHandlers_PublicEndpoints(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	store, _ := NewStore(db)
	svc := NewService(store, nil, "1804", nil)
	r := setupTestRouter(svc)

	// 1. GET /api/watchtower/summary (Public)
	reqSummary, _ := http.NewRequest(http.MethodGet, "/api/watchtower/summary", nil)
	wSummary := httptest.NewRecorder()
	r.ServeHTTP(wSummary, reqSummary)

	if wSummary.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for summary, got %d", wSummary.Code)
	}

	// 2. GET /api/watchtower/watchers (Public)
	reqList, _ := http.NewRequest(http.MethodGet, "/api/watchtower/watchers", nil)
	wList := httptest.NewRecorder()
	r.ServeHTTP(wList, reqList)

	if wList.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for watchers list, got %d", wList.Code)
	}
}

func TestHandlers_PINAuthentication(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	store, _ := NewStore(db)

	// Scenario A: PIN not configured (empty)
	svcUnconfigured := NewService(store, nil, "", nil)
	rUnconfigured := setupTestRouter(svcUnconfigured)

	body := bytes.NewBufferString(`{"name":"Todo FM","url":"https://todo-fm-d.vercel.app/"}`)
	reqNoPinConfig, _ := http.NewRequest(http.MethodPost, "/api/watchtower/watchers", body)
	wNoPinConfig := httptest.NewRecorder()
	rUnconfigured.ServeHTTP(wNoPinConfig, reqNoPinConfig)

	if wNoPinConfig.Code != http.StatusServiceUnavailable {
		t.Fatalf("Expected 503 Service Unavailable when PIN is unconfigured, got %d", wNoPinConfig.Code)
	}

	// Scenario B: PIN configured as "1804"
	svcConfigured := NewService(store, nil, "1804", nil)
	rConfigured := setupTestRouter(svcConfigured)

	// Missing PIN
	body1 := bytes.NewBufferString(`{"name":"Todo FM","url":"https://todo-fm-d.vercel.app/"}`)
	reqMissing, _ := http.NewRequest(http.MethodPost, "/api/watchtower/watchers", body1)
	wMissing := httptest.NewRecorder()
	rConfigured.ServeHTTP(wMissing, reqMissing)

	if wMissing.Code != http.StatusUnauthorized {
		t.Fatalf("Expected 401 Unauthorized for missing PIN, got %d", wMissing.Code)
	}

	// Invalid PIN
	body2 := bytes.NewBufferString(`{"name":"Todo FM","url":"https://todo-fm-d.vercel.app/"}`)
	reqInvalid, _ := http.NewRequest(http.MethodPost, "/api/watchtower/watchers", body2)
	reqInvalid.Header.Set("X-Watchtower-PIN", "9999")
	wInvalid := httptest.NewRecorder()
	rConfigured.ServeHTTP(wInvalid, reqInvalid)

	if wInvalid.Code != http.StatusUnauthorized {
		t.Fatalf("Expected 401 Unauthorized for invalid PIN, got %d", wInvalid.Code)
	}

	// Valid PIN via Header
	body3 := bytes.NewBufferString(`{"name":"Todo FM","url":"https://todo-fm-d.vercel.app/"}`)
	reqValid, _ := http.NewRequest(http.MethodPost, "/api/watchtower/watchers", body3)
	reqValid.Header.Set("X-Watchtower-PIN", "1804")
	wValid := httptest.NewRecorder()
	rConfigured.ServeHTTP(wValid, reqValid)

	if wValid.Code != http.StatusCreated {
		t.Fatalf("Expected 201 Created for valid PIN, got %d (body: %s)", wValid.Code, wValid.Body.String())
	}
}

func TestHandlers_WatcherLifecycle(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	store, _ := NewStore(db)

	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		rec := httptest.NewRecorder()
		rec.WriteHeader(http.StatusOK)
		rec.WriteString("OK")
		return rec.Result(), nil
	})

	checker := NewChecker(nil)
	checker.client.Transport = rt

	svc := NewService(store, nil, "1804", nil)
	svc.SetChecker(checker)
	r := setupTestRouter(svc)

	// 1. Create Watcher
	createBody, _ := json.Marshal(CreateWatcherInput{
		Name:           "Todo FM",
		URL:            "https://todo-fm-d.vercel.app/",
		ExpectedStatus: 200,
	})
	reqCreate, _ := http.NewRequest(http.MethodPost, "/api/watchtower/watchers", bytes.NewBuffer(createBody))
	reqCreate.Header.Set("X-Watchtower-PIN", "1804")
	wCreate := httptest.NewRecorder()
	r.ServeHTTP(wCreate, reqCreate)

	if wCreate.Code != http.StatusCreated {
		t.Fatalf("Create failed: %d (%s)", wCreate.Code, wCreate.Body.String())
	}

	var created Watcher
	if err := json.Unmarshal(wCreate.Body.Bytes(), &created); err != nil {
		t.Fatalf("Failed to parse created watcher: %v", err)
	}

	// 2. Check Now
	reqCheck, _ := http.NewRequest(http.MethodPost, "/api/watchtower/watchers/1/check", nil)
	reqCheck.Header.Set("X-Watchtower-PIN", "1804")
	wCheck := httptest.NewRecorder()
	r.ServeHTTP(wCheck, reqCheck)

	if wCheck.Code != http.StatusOK {
		t.Fatalf("Check now failed: %d (%s)", wCheck.Code, wCheck.Body.String())
	}

	// 3. Pause Watcher
	reqPause, _ := http.NewRequest(http.MethodPost, "/api/watchtower/watchers/1/pause", nil)
	reqPause.Header.Set("X-Watchtower-PIN", "1804")
	wPause := httptest.NewRecorder()
	r.ServeHTTP(wPause, reqPause)

	if wPause.Code != http.StatusOK {
		t.Fatalf("Pause failed: %d (%s)", wPause.Code, wPause.Body.String())
	}

	// 4. Resume Watcher
	reqResume, _ := http.NewRequest(http.MethodPost, "/api/watchtower/watchers/1/resume", nil)
	reqResume.Header.Set("X-Watchtower-PIN", "1804")
	wResume := httptest.NewRecorder()
	r.ServeHTTP(wResume, reqResume)

	if wResume.Code != http.StatusOK {
		t.Fatalf("Resume failed: %d (%s)", wResume.Code, wResume.Body.String())
	}

	// 5. Update Watcher
	newStatus := 204
	updateBody, _ := json.Marshal(UpdateWatcherInput{
		ExpectedStatus: &newStatus,
	})
	reqUpdate, _ := http.NewRequest(http.MethodPatch, "/api/watchtower/watchers/1", bytes.NewBuffer(updateBody))
	reqUpdate.Header.Set("X-Watchtower-PIN", "1804")
	wUpdate := httptest.NewRecorder()
	r.ServeHTTP(wUpdate, reqUpdate)

	if wUpdate.Code != http.StatusOK {
		t.Fatalf("Update failed: %d (%s)", wUpdate.Code, wUpdate.Body.String())
	}

	// 6. Delete Watcher
	reqDelete, _ := http.NewRequest(http.MethodDelete, "/api/watchtower/watchers/1", nil)
	reqDelete.Header.Set("X-Watchtower-PIN", "1804")
	wDelete := httptest.NewRecorder()
	r.ServeHTTP(wDelete, reqDelete)

	if wDelete.Code != http.StatusNoContent {
		t.Fatalf("Delete failed: %d (%s)", wDelete.Code, wDelete.Body.String())
	}
}

func TestModule_Wrapper(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	store, _ := NewStore(db)
	svc := NewService(store, nil, "1804", nil)
	mod := NewModule(svc)

	if mod.Service() != svc {
		t.Fatalf("Expected mod.Service() to match svc")
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	mod.RegisterRoutes(r)

	req, _ := http.NewRequest(http.MethodGet, "/api/watchtower/summary", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK via Module RegisterRoutes, got %d", w.Code)
	}
}
