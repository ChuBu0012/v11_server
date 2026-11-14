package worker

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestWorkerRunsAndStops(t *testing.T) {
	var count atomic.Int32

	fn := func(ctx context.Context) error {
		count.Add(1)
		return nil
	}

	w := New("test-worker", 50*time.Millisecond, fn)

	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)

	// Wait enough for at least the immediate run + a few ticks.
	time.Sleep(180 * time.Millisecond)
	cancel()

	// Small grace period for the goroutine to exit.
	time.Sleep(30 * time.Millisecond)

	c := count.Load()
	// Expect at least 3: immediate + ~2-3 ticks in 180ms at 50ms interval
	if c < 3 {
		t.Errorf("expected at least 3 executions, got %d", c)
	}
}

func TestWorkerStopsPromptly(t *testing.T) {
	var count atomic.Int32

	fn := func(ctx context.Context) error {
		count.Add(1)
		return nil
	}

	w := New("stop-test", 1*time.Second, fn)

	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)

	// Wait for the immediate execution.
	time.Sleep(50 * time.Millisecond)
	cancel()

	// Give a small grace period.
	time.Sleep(50 * time.Millisecond)

	before := count.Load()
	time.Sleep(200 * time.Millisecond)
	after := count.Load()

	if after != before {
		t.Errorf("worker continued after cancel: before=%d, after=%d", before, after)
	}
}

func TestWorkerErrorDoesNotStop(t *testing.T) {
	var count atomic.Int32

	fn := func(ctx context.Context) error {
		count.Add(1)
		return context.DeadlineExceeded // simulate error
	}

	w := New("error-test", 50*time.Millisecond, fn)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Start(ctx)

	time.Sleep(180 * time.Millisecond)

	c := count.Load()
	if c < 3 {
		t.Errorf("expected worker to continue despite errors, got %d executions", c)
	}
}
