package ailimit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type mockSender struct {
	sent []string
}

func (m *mockSender) SendMessage(ctx context.Context, chatID int64, text string, replyMarkup any) (*struct{}, error) {
	m.sent = append(m.sent, text)
	return nil, nil
}

func TestService_CollectAndGetStatus(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewRepository(db)
	mockRunner := func(ctx context.Context) (string, error) {
		return `Gemini Models	Weekly Limit Remaining	60%	2026-11-28T08:50:17Z
Gemini Models	Five Hour Limit Remaining	80%	2026-11-22T09:38:36Z
Claude and GPT models	Weekly Limit Remaining	10%	2026-11-26T16:48:07Z
Claude and GPT models	Five Hour Limit Remaining	100%	2026-11-22T10:23:00Z`, nil
	}

	service := NewService(repo, nil, 0, mockRunner)
	ctx := context.Background()

	resp, err := service.Collect(ctx)
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	if len(resp.Buckets) != 4 {
		t.Fatalf("expected 4 buckets, got %d", len(resp.Buckets))
	}

	if resp.Insight.Level != "normal" {
		t.Errorf("expected normal insight because Claude is ignored and Gemini is healthy, got: %s", resp.Insight.Level)
	}

	// Test GetStatus
	status, err := service.GetStatus(ctx, true, 24)
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if len(status.History) != 4 {
		t.Fatalf("expected 4 history snapshots, got %d", len(status.History))
	}

	// Test FormatTelegramMessage
	msg := service.FormatTelegramMessage(status)
	if msg == "" || len(msg) < 50 {
		t.Errorf("expected formatted telegram message, got %q", msg)
	}
}

func TestHandlers_GetAndRefresh(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer db.Close()

	repo := NewRepository(db)
	mockRunner := func(ctx context.Context) (string, error) {
		return `Gemini Models	Weekly Limit Remaining	65%	2026-11-28T08:50:17Z
Gemini Models	Five Hour Limit Remaining	85%	2026-11-22T09:38:36Z
Claude and GPT models	Weekly Limit Remaining	50%	2026-11-26T16:48:07Z
Claude and GPT models	Five Hour Limit Remaining	100%	2026-11-22T10:23:00Z`, nil
	}

	service := NewService(repo, nil, 0, mockRunner)
	collector := NewCollector(service, 10*time.Minute)
	mod := NewModule(service, collector)

	r := gin.New()
	mod.RegisterRoutes(r.Group("/api/limits"))

	// Test GET /api/limits
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/limits?history=true&hours=12", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var res QuotaResponse
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal response failed: %v", err)
	}
	if len(res.Buckets) != 4 {
		t.Errorf("expected 4 buckets, got %d", len(res.Buckets))
	}
	if res.Insight.Level != "normal" {
		t.Errorf("expected normal level, got %s", res.Insight.Level)
	}

	// Test POST /api/limits/refresh
	wRef := httptest.NewRecorder()
	reqRef, _ := http.NewRequest(http.MethodPost, "/api/limits/refresh", nil)
	r.ServeHTTP(wRef, reqRef)

	if wRef.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on refresh, got %d: %s", wRef.Code, wRef.Body.String())
	}
}
