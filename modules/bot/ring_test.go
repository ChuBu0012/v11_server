package bot

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRingController_Lifecycle(t *testing.T) {
	var mu sync.Mutex
	var commandsExecuted [][]string

	mockRunner := func(ctx context.Context, name string, args ...string) error {
		mu.Lock()
		defer mu.Unlock()
		cmd := append([]string{name}, args...)
		commandsExecuted = append(commandsExecuted, cmd)
		return nil
	}

	ctrl := NewRingController(
		WithRingDuration(50*time.Millisecond),
		WithRingCommandRunner(mockRunner),
	)

	ctx := context.Background()

	// 1. Initial state
	if ctrl.IsRinging() {
		t.Fatalf("Expected initial IsRinging() to be false")
	}

	// 2. Trigger Ring
	msg, err := ctrl.Ring(ctx)
	if err != nil {
		t.Fatalf("Ring() returned error: %v", err)
	}
	if !ctrl.IsRinging() {
		t.Fatalf("Expected IsRinging() to be true after Ring()")
	}
	if !strings.Contains(msg, "ส่งเสียงเรียกโทรศัพท์แล้ว") {
		t.Errorf("Unexpected Ring message: %s", msg)
	}

	// Verify executed commands
	mu.Lock()
	hasVolume := false
	hasPlay := false
	hasNotification := false
	for _, cmd := range commandsExecuted {
		if len(cmd) > 0 {
			switch cmd[0] {
			case "termux-volume":
				hasVolume = true
			case "termux-media-player":
				if len(cmd) > 1 && cmd[1] == "play" {
					hasPlay = true
				}
			case "termux-notification":
				hasNotification = true
			}
		}
	}
	mu.Unlock()

	if !hasVolume || !hasPlay || !hasNotification {
		t.Errorf("Missing expected shell commands: volume=%v, play=%v, noti=%v", hasVolume, hasPlay, hasNotification)
	}

	// 3. Trigger Stop
	stopMsg, err := ctrl.Stop(ctx)
	if err != nil {
		t.Fatalf("Stop() returned error: %v", err)
	}
	if ctrl.IsRinging() {
		t.Fatalf("Expected IsRinging() to be false after Stop()")
	}
	if !strings.Contains(stopMsg, "ปิดเสียงโทรศัพท์เรียบร้อยแล้ว") {
		t.Errorf("Unexpected Stop message: %s", stopMsg)
	}

	// 4. Trigger Stop again when already stopped
	secondStopMsg, err := ctrl.Stop(ctx)
	if err != nil {
		t.Fatalf("Second Stop() returned error: %v", err)
	}
	if !strings.Contains(secondStopMsg, "ไม่ได้กำลังส่งเสียงอยู่") {
		t.Errorf("Unexpected second Stop message: %s", secondStopMsg)
	}

	// 5. Test Auto-Off Timer
	_, err = ctrl.Ring(ctx)
	if err != nil {
		t.Fatalf("Ring for auto-off test failed: %v", err)
	}
	if !ctrl.IsRinging() {
		t.Fatalf("Expected IsRinging() to be true")
	}

	// Wait for 50ms auto-off timer to fire
	time.Sleep(100 * time.Millisecond)

	if ctrl.IsRinging() {
		t.Errorf("Expected auto-off timer to set IsRinging() to false after timeout")
	}
}
