package bot

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"sync"
	"time"
)

// CommandRunner defines the signature for running system commands.
type CommandRunner func(ctx context.Context, name string, args ...string) error

// DefaultCommandRunner executes commands via os/exec.
func DefaultCommandRunner(ctx context.Context, name string, args ...string) error {
	cmdPath := name
	termuxBin := "/data/data/com.termux/files/usr/bin/" + name
	if _, err := os.Stat(termuxBin); err == nil {
		cmdPath = termuxBin
	}

	cmd := exec.CommandContext(ctx, cmdPath, args...)
	cmd.Env = append(os.Environ(), "PATH=/data/data/com.termux/files/usr/bin:/system/bin:"+os.Getenv("PATH"))
	return cmd.Run()
}

// TorchController controls the device LED flashlight safely with an auto-off timer.
type TorchController struct {
	mu           sync.Mutex
	isOn         bool
	autoOffTimer *time.Timer
	autoOffTime  time.Time
	autoOffDur   time.Duration
	runner       CommandRunner
}

// TorchOption allows configuring the TorchController.
type TorchOption func(*TorchController)

// WithAutoOffDuration sets the safety auto-off duration.
func WithAutoOffDuration(d time.Duration) TorchOption {
	return func(t *TorchController) {
		t.autoOffDur = d
	}
}

// WithCommandRunner overrides the system command runner.
func WithCommandRunner(r CommandRunner) TorchOption {
	return func(t *TorchController) {
		t.runner = r
	}
}

// NewTorchController creates a new TorchController instance.
func NewTorchController(opts ...TorchOption) *TorchController {
	t := &TorchController{
		autoOffDur: 3 * time.Minute, // 3-minute safety limit per spec
		runner:     DefaultCommandRunner,
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// On turns the flashlight on and arms the auto-off safety timer.
func (t *TorchController) On(ctx context.Context) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if err := t.runner(ctx, "termux-torch", "on"); err != nil {
		return "", fmt.Errorf("termux-torch on failed: %w", err)
	}

	t.isOn = true
	t.cancelTimerLocked()

	t.autoOffTime = time.Now().Add(t.autoOffDur)
	t.autoOffTimer = time.AfterFunc(t.autoOffDur, func() {
		t.autoOff()
	})

	return fmt.Sprintf("🔦 <b>Torch turned ON</b>\n⏳ Auto-off safety timer set for %v", t.autoOffDur), nil
}

// Off turns the flashlight off and cancels the safety timer.
func (t *TorchController) Off(ctx context.Context) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if err := t.runner(ctx, "termux-torch", "off"); err != nil {
		return "", fmt.Errorf("termux-torch off failed: %w", err)
	}

	t.isOn = false
	t.cancelTimerLocked()

	return "🔦 <b>Torch turned OFF</b>", nil
}

// Toggle toggles the flashlight state.
func (t *TorchController) Toggle(ctx context.Context) (string, error) {
	t.mu.Lock()
	currentState := t.isOn
	t.mu.Unlock()

	if currentState {
		return t.Off(ctx)
	}
	return t.On(ctx)
}

// IsOn returns whether the flashlight is currently on.
func (t *TorchController) IsOn() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.isOn
}

// Status returns a human-readable status string.
func (t *TorchController) Status() string {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.isOn {
		return "🔦 Torch is currently <b>OFF</b>."
	}

	remaining := time.Until(t.autoOffTime).Round(time.Second)
	if remaining < 0 {
		remaining = 0
	}
	return fmt.Sprintf("🔦 Torch is currently <b>ON</b> (auto-off in %v).", remaining)
}

func (t *TorchController) cancelTimerLocked() {
	if t.autoOffTimer != nil {
		t.autoOffTimer.Stop()
		t.autoOffTimer = nil
	}
	t.autoOffTime = time.Time{}
}

func (t *TorchController) autoOff() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.isOn {
		return
	}

	log.Printf("[hardware:torch] 3-minute safety timer expired. Automatically turning off flashlight.")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := t.runner(ctx, "termux-torch", "off"); err != nil {
		log.Printf("[hardware:torch] auto-off termux-torch failed: %v", err)
	}
	t.isOn = false
	t.autoOffTimer = nil
	t.autoOffTime = time.Time{}
}
