package watchtower

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// MockNotifier captures sent alerts and allows simulating delivery failures.
type MockNotifier struct {
	mu          sync.Mutex
	messages    []string
	returnError error
}

func (m *MockNotifier) Send(ctx context.Context, msg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.returnError != nil {
		return m.returnError
	}
	m.messages = append(m.messages, msg)
	return nil
}

func (m *MockNotifier) MessageCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.messages)
}

func (m *MockNotifier) LastMessage() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.messages) == 0 {
		return ""
	}
	return m.messages[len(m.messages)-1]
}

// MockEventLogger captures event logs for assertion.
type MockEventLogger struct {
	mu     sync.Mutex
	events []string
}

func (m *MockEventLogger) Log(level, message, metadata string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, level+": "+message)
	return nil
}

func (m *MockEventLogger) EventCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.events)
}

func TestService_DeterministicStateMachine(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create event_logs table for test DB
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS event_logs (id INTEGER PRIMARY KEY, level TEXT, message TEXT, metadata TEXT, created_at DATETIME);`)

	store, err := NewStore(db)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	notifier := &MockNotifier{}
	eventLogger := &MockEventLogger{}

	// Setup custom roundtripper checker so we control check outcomes
	var simulateSuccess bool
	var simulateStatusCode int = 500

	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		rec := httptest.NewRecorder()
		if simulateSuccess {
			rec.WriteHeader(http.StatusOK)
			rec.WriteString("OK")
		} else {
			rec.WriteHeader(simulateStatusCode)
			rec.WriteString("Fail")
		}
		return rec.Result(), nil
	})

	checker := NewChecker(nil)
	checker.client.Transport = rt

	svc := NewService(store, notifier, "1804", nil)
	svc.SetChecker(checker)
	svc.SetEventLogger(eventLogger)

	ctx := context.Background()

	// Create watcher: initial state unknown
	w, err := store.CreateWatcher(ctx, CreateWatcherInput{
		Name:           "Todo FM",
		URL:            "https://todo-fm-d.vercel.app/",
		ExpectedStatus: 200,
	})
	if err != nil {
		t.Fatalf("CreateWatcher failed: %v", err)
	}

	// 1. Fail 1st cycle: streak=1, state stays unknown, 0 alerts
	simulateSuccess = false
	_, err = svc.CheckWatcher(ctx, w.ID, false)
	if err != nil {
		t.Fatalf("CheckWatcher failed: %v", err)
	}
	w1, _ := store.GetWatcher(ctx, w.ID)
	if w1.ConsecutiveFailures != 1 || w1.State != StateUnknown || notifier.MessageCount() != 0 {
		t.Fatalf("Cycle 1 mismatch: streak=%d, state=%s, alerts=%d", w1.ConsecutiveFailures, w1.State, notifier.MessageCount())
	}

	// 2. Fail 2nd cycle: streak=2, state stays unknown, 0 alerts
	_, _ = svc.CheckWatcher(ctx, w.ID, false)
	w2, _ := store.GetWatcher(ctx, w.ID)
	if w2.ConsecutiveFailures != 2 || w2.State != StateUnknown || notifier.MessageCount() != 0 {
		t.Fatalf("Cycle 2 mismatch: streak=%d, state=%s, alerts=%d", w2.ConsecutiveFailures, w2.State, notifier.MessageCount())
	}

	// Diagnostic check now between failures: streak must NOT change!
	_, _ = svc.CheckWatcher(ctx, w.ID, true)
	wDiag, _ := store.GetWatcher(ctx, w.ID)
	if wDiag.ConsecutiveFailures != 2 || wDiag.State != StateUnknown || notifier.MessageCount() != 0 {
		t.Fatalf("Diagnostic check perturbed state: streak=%d, alerts=%d", wDiag.ConsecutiveFailures, notifier.MessageCount())
	}

	// 3. Fail 3rd cycle: streak=3, transition to DOWN, exactly ONE outage alert sent!
	_, _ = svc.CheckWatcher(ctx, w.ID, false)
	w3, _ := store.GetWatcher(ctx, w.ID)
	if w3.ConsecutiveFailures != 3 || w3.State != StateDown || notifier.MessageCount() != 1 {
		t.Fatalf("Cycle 3 mismatch: streak=%d, state=%s, alerts=%d", w3.ConsecutiveFailures, w3.State, notifier.MessageCount())
	}
	if !strings.Contains(notifier.LastMessage(), "Watchtower Outage Detected") {
		t.Fatalf("Expected outage alert message, got: %s", notifier.LastMessage())
	}

	// 4. Fail 4th cycle: streak=4, stays DOWN, NO duplicate alert!
	_, _ = svc.CheckWatcher(ctx, w.ID, false)
	w4, _ := store.GetWatcher(ctx, w.ID)
	if w4.ConsecutiveFailures != 4 || w4.State != StateDown || notifier.MessageCount() != 1 {
		t.Fatalf("Cycle 4 duplicate alert sent! streak=%d, alerts=%d", w4.ConsecutiveFailures, notifier.MessageCount())
	}

	// 5. Success 1st cycle: recovery, transition to UP, streak=0, exactly ONE recovery alert sent!
	simulateSuccess = true
	_, _ = svc.CheckWatcher(ctx, w.ID, false)
	wRec, _ := store.GetWatcher(ctx, w.ID)
	if wRec.ConsecutiveFailures != 0 || wRec.State != StateUp || notifier.MessageCount() != 2 {
		t.Fatalf("Recovery cycle mismatch: streak=%d, state=%s, alerts=%d", wRec.ConsecutiveFailures, wRec.State, notifier.MessageCount())
	}
	if !strings.Contains(notifier.LastMessage(), "Watchtower Recovery") {
		t.Fatalf("Expected recovery alert message, got: %s", notifier.LastMessage())
	}

	// 6. Success 2nd cycle: stays UP, NO duplicate recovery alert!
	_, _ = svc.CheckWatcher(ctx, w.ID, false)
	wUp, _ := store.GetWatcher(ctx, w.ID)
	if wUp.ConsecutiveFailures != 0 || wUp.State != StateUp || notifier.MessageCount() != 2 {
		t.Fatalf("Duplicate recovery alert sent! streak=%d, alerts=%d", wUp.ConsecutiveFailures, notifier.MessageCount())
	}
}

func TestService_AlertDecoupling_TelegramFailure(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS event_logs (id INTEGER PRIMARY KEY, level TEXT, message TEXT, metadata TEXT, created_at DATETIME);`)

	store, _ := NewStore(db)

	// Notifier that simulates Telegram API 500 error / network disconnect
	notifier := &MockNotifier{
		returnError: errors.New("telegram: 500 Internal Server Error"),
	}
	eventLogger := &MockEventLogger{}

	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		rec := httptest.NewRecorder()
		rec.WriteHeader(http.StatusBadGateway)
		return rec.Result(), nil
	})

	checker := NewChecker(nil)
	checker.client.Transport = rt

	svc := NewService(store, notifier, "1804", nil)
	svc.SetChecker(checker)
	svc.SetEventLogger(eventLogger)

	ctx := context.Background()

	w, _ := store.CreateWatcher(ctx, CreateWatcherInput{
		Name: "Decouple Target",
		URL:  "https://todo-fm-d.vercel.app/",
	})

	// Pre-set streak to 2 failures so the next check triggers the 3rd failure (transition to down)
	now := time.Now().UTC()
	_ = store.UpdateWatcherState(ctx, w.ID, StateUnknown, 2, &now)

	// Execute 3rd check: Telegram will fail, but state MUST become DOWN and event log must record ERROR
	_, err := svc.CheckWatcher(ctx, w.ID, false)
	if err != nil {
		t.Fatalf("CheckWatcher returned error: %v", err)
	}

	wFinal, err := store.GetWatcher(ctx, w.ID)
	if err != nil {
		t.Fatalf("GetWatcher failed: %v", err)
	}
	if wFinal.State != StateDown || wFinal.ConsecutiveFailures != 3 {
		t.Fatalf("State corrupted after telegram failure: state=%s, streak=%d", wFinal.State, wFinal.ConsecutiveFailures)
	}

	// Verify error was recorded in EventLogger
	if eventLogger.EventCount() == 0 {
		t.Fatalf("Expected error to be recorded in event logger, but found 0 events")
	}
}

func TestService_ConfigEdit_BaselineReset(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	store, _ := NewStore(db)
	notifier := &MockNotifier{}

	svc := NewService(store, notifier, "1804", nil)
	_ = svc
	ctx := context.Background()

	w, _ := store.CreateWatcher(ctx, CreateWatcherInput{
		Name: "Edit Target",
		URL:  "https://todo-fm-d.vercel.app/",
	})

	// Simulate down state with 3 consecutive failures
	now := time.Now().UTC()
	_ = store.UpdateWatcherState(ctx, w.ID, StateDown, 3, &now)

	// Reset baseline as when editing configuration
	if err := store.ResetWatcherBaseline(ctx, w.ID); err != nil {
		t.Fatalf("ResetWatcherBaseline failed: %v", err)
	}

	wAfter, _ := store.GetWatcher(ctx, w.ID)
	if wAfter.State != StateUnknown || wAfter.ConsecutiveFailures != 0 || wAfter.DownSince != nil {
		t.Fatalf("Baseline reset failed: %+v", wAfter)
	}

	// Must NEVER emit recovery alert on config edit
	if notifier.MessageCount() != 0 {
		t.Fatalf("Recovery alert unexpectedly emitted on config edit: %s", notifier.LastMessage())
	}
}

func TestService_IncidentSummarizer_OutageEnrichment(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	store, _ := NewStore(db)
	notifier := &MockNotifier{}

	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		rec := httptest.NewRecorder()
		rec.WriteHeader(http.StatusServiceUnavailable)
		rec.WriteString("Fail")
		return rec.Result(), nil
	})

	checker := NewChecker(nil)
	checker.client.Transport = rt

	svc := NewService(store, notifier, "1804", nil)
	svc.SetChecker(checker)

	summarizerCalled := false
	svc.SetIncidentSummarizer(func(ctx context.Context, w *Watcher, res CheckResult) string {
		summarizerCalled = true
		return "📌 AI Analysis: Origin server error 503"
	})

	ctx := context.Background()
	w, _ := store.CreateWatcher(ctx, CreateWatcherInput{
		Name: "AI Incident Target",
		URL:  "https://todo-fm-d.vercel.app/",
	})

	// Pre-set streak to 2 failures
	now := time.Now().UTC()
	_ = store.UpdateWatcherState(ctx, w.ID, StateUnknown, 2, &now)

	// Check 3rd failure -> outage alert with AI enrichment
	_, err := svc.CheckWatcher(ctx, w.ID, false)
	if err != nil {
		t.Fatalf("CheckWatcher failed: %v", err)
	}

	if !summarizerCalled {
		t.Fatalf("Expected incident summarizer to be called on outage transition")
	}

	lastMsg := notifier.LastMessage()
	if !strings.Contains(lastMsg, "📌 AI Analysis: Origin server error 503") {
		t.Fatalf("Expected outage alert to contain AI incident summary, got: %s", lastMsg)
	}
}

func TestService_StartLifecycle(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	store, _ := NewStore(db)
	notifier := &MockNotifier{}
	svc := NewService(store, notifier, "1804", nil)

	ctx, cancel := context.WithCancel(context.Background())
	svc.Start(ctx)

	// Cancel promptly and stop
	time.Sleep(20 * time.Millisecond)
	cancel()
	svc.Stop()
}
