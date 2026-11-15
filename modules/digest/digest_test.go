package digest

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	_ "modernc.org/sqlite"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestStoreOperations(t *testing.T) {
	db := newTestDB(t)
	store, err := NewStore(db)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	ctx := context.Background()

	// 1. Initial settings
	st, err := store.GetSettings(ctx)
	if err != nil {
		t.Fatalf("GetSettings failed: %v", err)
	}
	if !st.Enabled {
		t.Errorf("expected default enabled=true, got %v", st.Enabled)
	}
	if st.Schedule != "13:00" {
		t.Errorf("expected default schedule=13:00, got %s", st.Schedule)
	}

	// 2. Toggle enabled
	if err := store.SetEnabled(ctx, false); err != nil {
		t.Fatalf("SetEnabled failed: %v", err)
	}
	st, _ = store.GetSettings(ctx)
	if st.Enabled {
		t.Errorf("expected enabled=false after toggle, got %v", st.Enabled)
	}

	// 3. Record runs
	id, err := store.RecordStart(ctx)
	if err != nil {
		t.Fatalf("RecordStart failed: %v", err)
	}
	if id <= 0 {
		t.Errorf("expected positive run ID, got %d", id)
	}

	if err := store.RecordSuccess(ctx, id, "Sample Summary", 5); err != nil {
		t.Fatalf("RecordSuccess failed: %v", err)
	}

	latest, err := store.GetLatestRun(ctx)
	if err != nil {
		t.Fatalf("GetLatestRun failed: %v", err)
	}
	if latest == nil || latest.Status != "success" || latest.SummaryMD != "Sample Summary" {
		t.Errorf("unexpected latest run: %+v", latest)
	}

	// 4. Record failure
	failID, _ := store.RecordStart(ctx)
	_ = store.RecordFailure(ctx, failID, "API Rate Limited")
	latestFail, _ := store.GetLatestRun(ctx)
	if latestFail.Status != "failed" || latestFail.ErrorMessage != "API Rate Limited" {
		t.Errorf("unexpected failed run: %+v", latestFail)
	}
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newMockClient(fn roundTripFunc) *http.Client {
	return &http.Client{Transport: fn}
}

func TestFetchFeedRSS(t *testing.T) {
	mockRSS := `<?xml version="1.0" encoding="UTF-8"?>
	<rss version="2.0">
	<channel>
		<title>AI News</title>
		<item>
			<title>OpenAI Releases New Model</title>
			<link>https://example.com/openai</link>
			<description><![CDATA[<p>A breakthrough in reasoning.</p>]]></description>
			<pubDate>Sun, 15 Nov 2026 10:00:00 GMT</pubDate>
		</item>
	</channel>
	</rss>`

	client := newMockClient(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(mockRSS)),
			Header:     make(http.Header),
		}, nil
	})

	items, err := FetchFeed(context.Background(), "AI", "TestFeed", "http://mock.feed/rss", 5, client)
	if err != nil {
		t.Fatalf("FetchFeed failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Title != "OpenAI Releases New Model" {
		t.Errorf("unexpected title: %s", items[0].Title)
	}
	if items[0].Description != "A breakthrough in reasoning." {
		t.Errorf("unexpected description: %s", items[0].Description)
	}
}

func TestGeminiClientSummarize(t *testing.T) {
	mockResp := geminiResponse{
		Candidates: []geminiCandidate{
			{
				Content: geminiContent{
					Parts: []geminiPart{
						{Text: "🤖 **AI News Today**\n1. Breakthrough in reasoning model."},
					},
				},
			},
		},
	}
	respBytes, _ := json.Marshal(mockResp)

	client := newMockClient(func(req *http.Request) (*http.Response, error) {
		if !strings.Contains(req.URL.RawQuery, "key=TEST_KEY") {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Body:       io.NopCloser(strings.NewReader("missing key")),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader(respBytes)),
			Header:     make(http.Header),
		}, nil
	})

	gc := NewGeminiClient("TEST_KEY", WithBaseURL("http://mock.gemini"), WithHTTPClient(client))
	summary, err := gc.SummarizeNews(context.Background(), []NewsItem{
		{Category: "AI", Title: "OpenAI Breakthrough", Source: "TechFeed"},
	})
	if err != nil {
		t.Fatalf("SummarizeNews failed: %v", err)
	}
	if !strings.Contains(summary, "Breakthrough in reasoning") {
		t.Errorf("unexpected summary text: %s", summary)
	}
}

type mockNotifier struct {
	sent []string
}

func (m *mockNotifier) Send(ctx context.Context, msg string) error {
	m.sent = append(m.sent, msg)
	return nil
}

func TestServiceGenerateDigest(t *testing.T) {
	db := newTestDB(t)
	store, _ := NewStore(db)

	mockResp := geminiResponse{
		Candidates: []geminiCandidate{
			{
				Content: geminiContent{
					Parts: []geminiPart{
						{Text: "🤖 AI Digest Summary Test"},
					},
				},
			},
		},
	}
	respBytes, _ := json.Marshal(mockResp)

	client := newMockClient(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader(respBytes)),
			Header:     make(http.Header),
		}, nil
	})

	gc := NewGeminiClient("KEY", WithBaseURL("http://mock.gemini"), WithHTTPClient(client))
	notifier := &mockNotifier{}

	svc := NewService(store, gc, notifier, WithFeeds(nil)) // no external feeds for deterministic test

	// 1. Force run
	record, err := svc.GenerateDigest(context.Background(), true)
	if err != nil {
		t.Fatalf("GenerateDigest failed: %v", err)
	}
	if record == nil || record.Status != "success" {
		t.Fatalf("unexpected record: %+v", record)
	}

	// 2. Check disabled skip
	_ = store.SetEnabled(context.Background(), false)
	recSkip, err := svc.GenerateDigest(context.Background(), false)
	if err != nil {
		t.Fatalf("unexpected error on skip: %v", err)
	}
	if recSkip != nil {
		t.Errorf("expected nil record on skipped run, got: %+v", recSkip)
	}
}

func TestDigestHTTPHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newTestDB(t)
	store, _ := NewStore(db)

	mockResp := geminiResponse{
		Candidates: []geminiCandidate{
			{
				Content: geminiContent{
					Parts: []geminiPart{
						{Text: "Test Summary"},
					},
				},
			},
		},
	}
	respBytes, _ := json.Marshal(mockResp)

	client := newMockClient(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader(respBytes)),
			Header:     make(http.Header),
		}, nil
	})

	gc := NewGeminiClient("KEY", WithBaseURL("http://mock.gemini"), WithHTTPClient(client))
	svc := NewService(store, gc, nil, WithFeeds(nil))
	mod := New(svc)

	r := gin.New()
	mod.RegisterRoutes(r.Group("/api/digest"))

	// 1. GET /api/digest/status
	req := httptest.NewRequest(http.MethodGet, "/api/digest/status", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status code = %d, want 200", rr.Code)
	}

	// 2. POST /api/digest/toggle
	toggleBody := `{"enabled": false}`
	req = httptest.NewRequest(http.MethodPost, "/api/digest/toggle", bytes.NewReader([]byte(toggleBody)))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("toggle status code = %d, want 200", rr.Code)
	}

	// 3. POST /api/digest/run
	req = httptest.NewRequest(http.MethodPost, "/api/digest/run", nil)
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("run status code = %d, want 200", rr.Code)
	}

	// 4. GET /api/digest/history
	req = httptest.NewRequest(http.MethodGet, "/api/digest/history", nil)
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("history status code = %d, want 200", rr.Code)
	}
}
