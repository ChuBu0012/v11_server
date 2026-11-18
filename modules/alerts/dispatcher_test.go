package alerts

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type recordingNotifier struct {
	mu       sync.Mutex
	messages []string
	delay    time.Duration
	failErr  error
}

func (n *recordingNotifier) Send(ctx context.Context, msg string) error {
	if n.delay > 0 {
		select {
		case <-time.After(n.delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.failErr != nil {
		return n.failErr
	}
	n.messages = append(n.messages, msg)
	return nil
}

func (n *recordingNotifier) count() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.messages)
}

type recordingLogger struct {
	mu     sync.Mutex
	events []string
}

func (l *recordingLogger) RecordLog(level, message, metadata string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, level+": "+message)
	return nil
}

func TestAlertManagerTransitionAndSuppression(t *testing.T) {
	store, cleanup := setupTestAlertStore(t)
	defer cleanup()

	notifier := &recordingNotifier{}
	logger := &recordingLogger{}

	thresholds := Thresholds{
		BatteryPercent: 20.0,
		DiskFreeGB:     5.0,
	}

	mgr := NewAlertManager(store, notifier, thresholds, logger,
		WithDeliveryTimeout(2*time.Second),
	)

	// Sample 1: Low disk (3 GB) -> should trigger alert
	s1 := Sample{
		Timestamp:  time.Now().UTC(),
		DiskFreeGB: 3.0,
		ServiceOK:  true,
	}
	if err := mgr.ProcessSample(s1); err != nil {
		t.Fatalf("ProcessSample failed: %v", err)
	}

	// Give async dispatch time to complete
	time.Sleep(100 * time.Millisecond)

	if notifier.count() != 1 {
		t.Fatalf("Expected 1 notification sent for LowDisk, got %d", notifier.count())
	}

	// Sample 2: Still low disk (2.5 GB) -> should suppress duplicate alert
	s2 := Sample{
		Timestamp:  time.Now().UTC(),
		DiskFreeGB: 2.5,
		ServiceOK:  true,
	}
	if err := mgr.ProcessSample(s2); err != nil {
		t.Fatalf("ProcessSample 2 failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	if notifier.count() != 1 {
		t.Errorf("Expected still 1 notification (duplicate suppressed), got %d", notifier.count())
	}

	// Sample 3: Disk recovered (15 GB) -> should trigger recovery notification
	s3 := Sample{
		Timestamp:  time.Now().UTC(),
		DiskFreeGB: 15.0,
		ServiceOK:  true,
	}
	if err := mgr.ProcessSample(s3); err != nil {
		t.Fatalf("ProcessSample 3 failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	if notifier.count() != 2 {
		t.Errorf("Expected 2 notifications total (alert + recovery), got %d", notifier.count())
	}
}

func TestAlertManagerSlowOrFailingNotifierNonBlocking(t *testing.T) {
	store, cleanup := setupTestAlertStore(t)
	defer cleanup()

	// Slow notifier that fails
	notifier := &recordingNotifier{
		delay:   500 * time.Millisecond,
		failErr: errors.New("network timeout"),
	}
	logger := &recordingLogger{}

	thresholds := Thresholds{DiskFreeGB: 5.0}
	mgr := NewAlertManager(store, notifier, thresholds, logger,
		WithDeliveryTimeout(1*time.Second),
	)

	s := Sample{
		Timestamp:  time.Now().UTC(),
		DiskFreeGB: 2.0,
		ServiceOK:  true,
	}

	start := time.Now()
	// ProcessSample MUST NOT block on the 500ms notifier delay
	if err := mgr.ProcessSample(s); err != nil {
		t.Fatalf("ProcessSample failed: %v", err)
	}
	duration := time.Since(start)

	if duration > 100*time.Millisecond {
		t.Errorf("ProcessSample blocked unexpectedly: took %v", duration)
	}

	// Wait for async dispatch to attempt delivery and log failure
	time.Sleep(700 * time.Millisecond)

	logger.mu.Lock()
	defer logger.mu.Unlock()
	hasErrorLog := false
	for _, e := range logger.events {
		if len(e) >= 5 && e[:5] == "error" {
			hasErrorLog = true
			break
		}
	}
	if !hasErrorLog {
		t.Errorf("Expected error log for failed delivery, got logs: %v", logger.events)
	}
}
