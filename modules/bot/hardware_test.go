package bot

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type mockTorchRunner struct {
	mu      sync.Mutex
	history []string
	fail    bool
}

func (m *mockTorchRunner) Run(ctx context.Context, name string, args ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail {
		return errors.New("camera device busy or permission denied")
	}
	m.history = append(m.history, name+" "+strings.Join(args, " "))
	return nil
}

func (m *mockTorchRunner) LastCommand() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.history) == 0 {
		return ""
	}
	return m.history[len(m.history)-1]
}

func TestHardwareTorch(t *testing.T) {
	runner := &mockTorchRunner{}
	torch := NewTorchController(
		WithCommandRunner(runner.Run),
		WithAutoOffDuration(50*time.Millisecond), // Fast auto-off for unit test
	)

	ctx := context.Background()

	// Initial status
	if torch.IsOn() {
		t.Error("Expected torch initially OFF")
	}

	// 1. Turn ON
	msg, err := torch.On(ctx)
	if err != nil {
		t.Fatalf("Turn on failed: %v", err)
	}
	if !strings.Contains(msg, "Torch turned ON") {
		t.Errorf("Unexpected turn on message: %s", msg)
	}
	if !torch.IsOn() {
		t.Error("Expected torch to be ON")
	}
	if runner.LastCommand() != "termux-torch on" {
		t.Errorf("Expected command 'termux-torch on', got %s", runner.LastCommand())
	}

	// 2. Turn OFF
	msg, err = torch.Off(ctx)
	if err != nil {
		t.Fatalf("Turn off failed: %v", err)
	}
	if !strings.Contains(msg, "Torch turned OFF") {
		t.Errorf("Unexpected turn off message: %s", msg)
	}
	if torch.IsOn() {
		t.Error("Expected torch to be OFF")
	}
	if runner.LastCommand() != "termux-torch off" {
		t.Errorf("Expected command 'termux-torch off', got %s", runner.LastCommand())
	}

	// 3. Toggle
	msg, err = torch.Toggle(ctx)
	if err != nil {
		t.Fatalf("Toggle on failed: %v", err)
	}
	if !torch.IsOn() {
		t.Error("Expected torch to be ON after toggle")
	}

	msg, err = torch.Toggle(ctx)
	if err != nil {
		t.Fatalf("Toggle off failed: %v", err)
	}
	if torch.IsOn() {
		t.Error("Expected torch to be OFF after second toggle")
	}

	// 4. Test Auto-Off Safety Timer
	_, err = torch.On(ctx)
	if err != nil {
		t.Fatalf("Turn on failed: %v", err)
	}
	if !torch.IsOn() {
		t.Fatal("Torch should be ON before timer expiry")
	}

	// Wait for auto-off timer (50ms + margin)
	time.Sleep(100 * time.Millisecond)

	if torch.IsOn() {
		t.Error("Expected torch to be automatically turned OFF after timer expiry")
	}
	if runner.LastCommand() != "termux-torch off" {
		t.Errorf("Expected last command to be 'termux-torch off' from auto-off, got %s", runner.LastCommand())
	}

	// 5. Error handling when hardware runner fails
	runner.fail = true
	_, err = torch.On(ctx)
	if err == nil {
		t.Error("Expected error when runner fails, got nil")
	}
	if torch.IsOn() {
		t.Error("Torch should not be marked ON if command failed")
	}

	// 6. Test cancellation of timer on explicit Off() before timeout
	runner.fail = false
	runner.history = nil
	_, err = torch.On(ctx)
	if err != nil {
		t.Fatalf("Turn on failed: %v", err)
	}
	_, err = torch.Off(ctx)
	if err != nil {
		t.Fatalf("Turn off failed: %v", err)
	}

	// Wait past the 50ms auto-off duration to ensure timer was cancelled
	time.Sleep(100 * time.Millisecond)
	runner.mu.Lock()
	offCommandCount := 0
	for _, cmd := range runner.history {
		if cmd == "termux-torch off" {
			offCommandCount++
		}
	}
	runner.mu.Unlock()

	// Exactly 1 off command was issued by explicit Off(); timer must not issue a second one
	if offCommandCount != 1 {
		t.Errorf("Expected exactly 1 'termux-torch off' command, got %d (timer cancellation failed)", offCommandCount)
	}
}
