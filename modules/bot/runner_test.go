package bot

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestRunner_CanAcquireLock_Lifecycle verifies the single-flight locking state transitions:
// idle -> executing (busy, lock unavailable) -> completed (idle, lock restored).
func TestRunner_CanAcquireLock_Lifecycle(t *testing.T) {
	blockCh := make(chan struct{})
	mockExec := func(ctx context.Context, dir string, env []string, binPath string, args ...string) ([]byte, int, error) {
		<-blockCh
		return []byte("ok"), 0, nil
	}

	r := NewAgentRunner(WithCustomExec(mockExec))

	// 1. Initial idle state
	if !r.CanAcquireLock() {
		t.Fatal("expected CanAcquireLock to be true when idle")
	}
	if r.IsRunning() {
		t.Fatal("expected IsRunning to be false when idle")
	}

	// 2. Dispatch task 1
	done := make(chan TaskResult, 1)
	err := r.DispatchAsync(context.Background(), TaskRequest{RawPrompt: "job1"}, func(res TaskResult) {
		done <- res
	})
	if err != nil {
		t.Fatalf("unexpected dispatch error: %v", err)
	}

	// Small yield to ensure goroutine started
	time.Sleep(15 * time.Millisecond)

	// 3. Busy state while executing
	if r.CanAcquireLock() {
		t.Fatal("expected CanAcquireLock to be false while job1 is running")
	}
	if !r.IsRunning() {
		t.Fatal("expected IsRunning to be true while job1 is running")
	}

	// 4. Second task immediately rejected with ErrAgentBusy
	err2 := r.DispatchAsync(context.Background(), TaskRequest{RawPrompt: "job2"}, func(res TaskResult) {
		t.Fatal("job2 callback should never be invoked")
	})
	if !errors.Is(err2, ErrAgentBusy) {
		t.Fatalf("expected ErrAgentBusy, got: %v", err2)
	}

	// 5. Complete task 1 and verify lock released
	close(blockCh)
	select {
	case res := <-done:
		if !res.Success {
			t.Fatalf("expected job1 success, got err: %s", res.ErrorMessage)
		}
		if res.Output != "ok" {
			t.Fatalf("expected output 'ok', got: %s", res.Output)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for job1 completion")
	}

	// Allow runner unlock to complete
	time.Sleep(15 * time.Millisecond)

	if !r.CanAcquireLock() {
		t.Fatal("expected CanAcquireLock to return true after job1 completion")
	}
	if r.IsRunning() {
		t.Fatal("expected IsRunning to return false after job1 completion")
	}
}

// TestRunner_RapidConcurrentDispatch_Stress tests 50 simultaneous dispatches.
// Exactly 1 must succeed and 49 must fail with ErrAgentBusy.
func TestRunner_RapidConcurrentDispatch_Stress(t *testing.T) {
	blockCh := make(chan struct{})
	mockExec := func(ctx context.Context, dir string, env []string, binPath string, args ...string) ([]byte, int, error) {
		<-blockCh
		return []byte("done"), 0, nil
	}

	r := NewAgentRunner(WithCustomExec(mockExec))

	const numGoroutines = 50
	startBarrier := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	var successCount int64
	var busyCount int64
	done := make(chan TaskResult, 1)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			<-startBarrier // Synchronize simultaneous dispatch
			err := r.DispatchAsync(context.Background(), TaskRequest{RawPrompt: fmt.Sprintf("req-%d", id)}, func(res TaskResult) {
				done <- res
			})
			if err == nil {
				atomic.AddInt64(&successCount, 1)
			} else if errors.Is(err, ErrAgentBusy) {
				atomic.AddInt64(&busyCount, 1)
			}
		}(i)
	}

	// Fire all goroutines at the exact same instant
	close(startBarrier)
	wg.Wait()

	if atomic.LoadInt64(&successCount) != 1 {
		t.Fatalf("expected exactly 1 successful dispatch, got %d", successCount)
	}
	if atomic.LoadInt64(&busyCount) != int64(numGoroutines-1) {
		t.Fatalf("expected %d busy rejections, got %d", numGoroutines-1, busyCount)
	}

	// Release the single running task
	close(blockCh)
	select {
	case res := <-done:
		if !res.Success {
			t.Fatalf("expected running task to succeed, got: %s", res.ErrorMessage)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for single task completion")
	}

	time.Sleep(15 * time.Millisecond)
	if !r.CanAcquireLock() {
		t.Fatal("expected lock to be released after completion")
	}
}

// TestRunner_Timeout_CancelsExecution verifies that execution exceeding the configured
// timeout is cancelled and returns ErrTaskTimeout.
func TestRunner_Timeout_CancelsExecution(t *testing.T) {
	mockExec := func(ctx context.Context, dir string, env []string, binPath string, args ...string) ([]byte, int, error) {
		select {
		case <-time.After(1 * time.Second):
			return []byte("should not finish"), 0, nil
		case <-ctx.Done():
			return nil, -1, ctx.Err()
		}
	}

	customTimeout := 50 * time.Millisecond
	r := NewAgentRunner(WithCustomExec(mockExec), WithCustomTimeout(customTimeout))

	done := make(chan TaskResult, 1)
	start := time.Now()
	err := r.DispatchAsync(context.Background(), TaskRequest{RawPrompt: "long task"}, func(res TaskResult) {
		done <- res
	})
	if err != nil {
		t.Fatalf("failed to dispatch: %v", err)
	}

	select {
	case res := <-done:
		elapsed := time.Since(start)
		if res.Success {
			t.Fatal("expected task to fail on timeout, but got Success=true")
		}
		if elapsed < 40*time.Millisecond {
			t.Fatalf("expected execution to run at least ~50ms, finished in %v", elapsed)
		}
		if !strings.Contains(strings.ToLower(res.ErrorMessage), "timed out") && !strings.Contains(strings.ToLower(res.ErrorMessage), "timeout") {
			t.Errorf("expected timeout error message, got: %s", res.ErrorMessage)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("test hung: timeout was not enforced by runner")
	}

	// Ensure runner returns to idle
	time.Sleep(15 * time.Millisecond)
	if !r.CanAcquireLock() {
		t.Fatal("expected lock to be released after timeout")
	}
}

// TestRunner_EnvironmentScrubbing verifies that sensitive credentials are completely
// removed from the child process environment while non-sensitive variables are retained.
func TestRunner_EnvironmentScrubbing(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "123456789:ABCdefGHIjklMNOpqrSTUvwxYZ_12345678")
	t.Setenv("GEMINI_API_KEY", "AIzaSyD-1234567890abcdef1234567890abc12")
	t.Setenv("NGROK_AUTHTOKEN", "2AbCdEfGhIjKlMnOpQrStUvWxYz_12345")
	t.Setenv("DISCORD_WEBHOOK_URL", "https://discord.com/api/webhooks/123/token")
	t.Setenv("BENIGN_ENV_VAR", "pocket_sentinel_safe_value")

	var capturedEnv []string
	mockExec := func(ctx context.Context, dir string, env []string, binPath string, args ...string) ([]byte, int, error) {
		capturedEnv = env
		return []byte("ok"), 0, nil
	}

	r := NewAgentRunner(WithCustomExec(mockExec))

	done := make(chan TaskResult, 1)
	err := r.DispatchAsync(context.Background(), TaskRequest{RawPrompt: "env test"}, func(res TaskResult) {
		done <- res
	})
	if err != nil {
		t.Fatalf("dispatch error: %v", err)
	}
	<-done

	for _, envEntry := range capturedEnv {
		upper := strings.ToUpper(envEntry)
		if strings.HasPrefix(upper, "TELEGRAM_BOT_TOKEN=") {
			t.Errorf("SECURITY LEAK: TELEGRAM_BOT_TOKEN found in subprocess env: %s", envEntry)
		}
		if strings.HasPrefix(upper, "GEMINI_API_KEY=") {
			t.Errorf("SECURITY LEAK: GEMINI_API_KEY found in subprocess env: %s", envEntry)
		}
		if strings.HasPrefix(upper, "NGROK_AUTHTOKEN=") {
			t.Errorf("SECURITY LEAK: NGROK_AUTHTOKEN found in subprocess env: %s", envEntry)
		}
		if strings.HasPrefix(upper, "DISCORD_WEBHOOK_URL=") {
			t.Errorf("SECURITY LEAK: DISCORD_WEBHOOK_URL found in subprocess env: %s", envEntry)
		}
	}

	// Verify safe environment variable was preserved
	foundBenign := false
	for _, envEntry := range capturedEnv {
		if strings.HasPrefix(envEntry, "BENIGN_ENV_VAR=pocket_sentinel_safe_value") {
			foundBenign = true
			break
		}
	}
	if !foundBenign {
		t.Error("expected non-sensitive environment variables to be preserved in subprocess env")
	}
}

// TestSanitizeEnvironment_Unit directly tests SanitizeEnvironment logic with mixed casing and edge cases.
func TestSanitizeEnvironment_Unit(t *testing.T) {
	input := []string{
		"PATH=/usr/bin:/bin",
		"telegram_bot_token=secret_bot_1",
		"TELEGRAM_BOT_TOKEN=secret_bot_2",
		"Telegram_Bot_Token=secret_bot_3",
		"gemini_api_key=secret_gemini",
		"GEMINI_API_KEY=secret_gemini_2",
		"ngrok_authtoken=secret_ngrok",
		"NGROK_AUTHTOKEN=secret_ngrok_2",
		"discord_webhook_url=https://discord.com/api/webhooks/xyz",
		"DISCORD_WEBHOOK_URL=https://discord.com/api/webhooks/xyz2",
		"HOME=/home/user",
		"USER=app",
		"NO_EQUALS_SIGN_VAL",
	}

	clean := SanitizeEnvironment(input)

	for _, entry := range clean {
		upper := strings.ToUpper(entry)
		if strings.HasPrefix(upper, "TELEGRAM_BOT_TOKEN=") ||
			strings.HasPrefix(upper, "GEMINI_API_KEY=") ||
			strings.HasPrefix(upper, "NGROK_AUTHTOKEN=") ||
			strings.HasPrefix(upper, "DISCORD_WEBHOOK_URL=") {
			t.Fatalf("sanitized environment still contains secret: %s", entry)
		}
	}

	expectedRetained := map[string]bool{
		"PATH=/usr/bin:/bin": false,
		"HOME=/home/user":    false,
		"USER=app":           false,
		"NO_EQUALS_SIGN_VAL": false,
	}

	for _, entry := range clean {
		if _, ok := expectedRetained[entry]; ok {
			expectedRetained[entry] = true
		}
	}

	for k, found := range expectedRetained {
		if !found {
			t.Errorf("expected %q to be retained after sanitization", k)
		}
	}
}

// TestRunner_MockExec_ArgumentsAndFraming verifies that custom binary, custom workdir,
// and arguments vector match the required invocation structure.
func TestRunner_MockExec_ArgumentsAndFraming(t *testing.T) {
	var capturedBin string
	var capturedDir string
	var capturedArgs []string

	mockExec := func(ctx context.Context, dir string, env []string, binPath string, args ...string) ([]byte, int, error) {
		capturedBin = binPath
		capturedDir = dir
		capturedArgs = args
		return []byte("all good"), 0, nil
	}

	r := NewAgentRunner(
		WithCustomBinary("/mock/bin/agy"),
		WithCustomWorkDir("/data/custom/workdir"),
		WithCustomExec(mockExec),
	)

	testPrompt := "เช็คสถานะระบบ"
	guard := NewPromptGuard()
	framedPrompt := guard.Frame(testPrompt)

	done := make(chan TaskResult, 1)
	err := r.DispatchAsync(context.Background(), TaskRequest{ChatID: 123456, RawPrompt: framedPrompt}, func(res TaskResult) {
		done <- res
	})
	if err != nil {
		t.Fatalf("dispatch error: %v", err)
	}
	res := <-done

	if !res.Success {
		t.Fatalf("expected success, got err: %s", res.ErrorMessage)
	}
	if capturedBin != "/mock/bin/agy" {
		t.Errorf("expected binPath /mock/bin/agy, got: %s", capturedBin)
	}
	if capturedDir != "/data/custom/workdir" {
		t.Errorf("expected workDir /data/custom/workdir, got: %s", capturedDir)
	}

	// Required args per spec: --dangerously-skip-permissions --effort medium -p "<framed_prompt>"
	expectedArgs := []string{"--dangerously-skip-permissions", "--effort", "medium", "-p", framedPrompt}
	if len(capturedArgs) != len(expectedArgs) {
		t.Fatalf("expected %d args, got %d: %v", len(expectedArgs), len(capturedArgs), capturedArgs)
	}
	for i, exp := range expectedArgs {
		if capturedArgs[i] != exp {
			t.Errorf("arg[%d]: expected %q, got %q", i, exp, capturedArgs[i])
		}
	}
}

// TestRunner_Status_Lifecycle verifies status inquiry during idle, active, and completed phases.
func TestRunner_Status_Lifecycle(t *testing.T) {
	startExec := make(chan struct{})
	finishExec := make(chan struct{})

	mockExec := func(ctx context.Context, dir string, env []string, binPath string, args ...string) ([]byte, int, error) {
		close(startExec)
		<-finishExec
		return []byte("done"), 0, nil
	}

	r := NewAgentRunner(WithCustomExec(mockExec))

	// 1. Idle status
	st := r.Status()
	if st.IsRunning || st.Prompt != "" || st.Elapsed != 0 || !st.StartedAt.IsZero() {
		t.Errorf("expected idle status, got %+v", st)
	}

	// 2. Active status
	done := make(chan TaskResult, 1)
	promptText := "status lifecycle test prompt"
	_ = r.DispatchAsync(context.Background(), TaskRequest{RawPrompt: promptText}, func(res TaskResult) {
		done <- res
	})

	<-startExec
	time.Sleep(20 * time.Millisecond)

	st = r.Status()
	if !st.IsRunning {
		t.Error("expected Status.IsRunning to be true while task is active")
	}
	if st.Prompt != promptText {
		t.Errorf("expected Status.Prompt %q, got %q", promptText, st.Prompt)
	}
	if st.Elapsed < 10*time.Millisecond {
		t.Errorf("expected Status.Elapsed >= 10ms, got %v", st.Elapsed)
	}
	if st.StartedAt.IsZero() {
		t.Error("expected Status.StartedAt to be non-zero")
	}

	// 3. Complete status
	close(finishExec)
	<-done
	time.Sleep(15 * time.Millisecond)

	st = r.Status()
	if st.IsRunning {
		t.Error("expected Status.IsRunning to be false after completion")
	}
	if st.Prompt != "" {
		t.Errorf("expected Status.Prompt to reset to empty, got %q", st.Prompt)
	}
}

// TestRunner_BinaryResolution_Missing verifies that a non-existent binary returns ErrBinaryNotFound.
func TestRunner_BinaryResolution_Missing(t *testing.T) {
	r := NewAgentRunner(WithCustomBinary("/tmp/non_existent_agy_binary_xyz_12345"))

	done := make(chan TaskResult, 1)
	err := r.DispatchAsync(context.Background(), TaskRequest{RawPrompt: "test"}, func(res TaskResult) {
		done <- res
	})
	if err != nil {
		if !errors.Is(err, ErrBinaryNotFound) {
			t.Errorf("expected ErrBinaryNotFound on dispatch, got: %v", err)
		}
		return
	}

	select {
	case res := <-done:
		if res.Success {
			t.Error("expected failure when binary is missing")
		}
		if !strings.Contains(res.ErrorMessage, "not found") {
			t.Errorf("expected error message to indicate binary not found, got: %s", res.ErrorMessage)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for binary missing failure")
	}
}

// TestRunner_RaceCondition_ConcurrentAccess stresses readers and writers under the Go race detector.
func TestRunner_RaceCondition_ConcurrentAccess(t *testing.T) {
	mockExec := func(ctx context.Context, dir string, env []string, binPath string, args ...string) ([]byte, int, error) {
		time.Sleep(2 * time.Millisecond)
		return []byte("race test output"), 0, nil
	}

	r := NewAgentRunner(WithCustomExec(mockExec))

	stopCh := make(chan struct{})
	var wg sync.WaitGroup

	// Reader goroutines
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stopCh:
					return
				default:
					_ = r.IsRunning()
					_ = r.CanAcquireLock()
					_ = r.Status()
				}
			}
		}()
	}

	// Writer/dispatcher goroutines
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 30; j++ {
				_ = r.DispatchAsync(context.Background(), TaskRequest{RawPrompt: fmt.Sprintf("race-%d-%d", id, j)}, func(res TaskResult) {})
				time.Sleep(1 * time.Millisecond)
			}
		}(i)
	}

	// Wait for writers to complete
	time.Sleep(150 * time.Millisecond)
	close(stopCh)
	wg.Wait()
}

// TestRunner_PanicRecovery verifies that panics inside the subprocess runner do not crash the daemon
// and that the mutex is safely unlocked.
func TestRunner_PanicRecovery(t *testing.T) {
	mockExec := func(ctx context.Context, dir string, env []string, binPath string, args ...string) ([]byte, int, error) {
		panic("simulated critical crash in executor")
	}

	r := NewAgentRunner(WithCustomExec(mockExec))

	done := make(chan TaskResult, 1)
	err := r.DispatchAsync(context.Background(), TaskRequest{RawPrompt: "panic test"}, func(res TaskResult) {
		done <- res
	})
	if err != nil {
		t.Fatalf("unexpected dispatch error: %v", err)
	}

	select {
	case res := <-done:
		if res.Success {
			t.Fatal("expected failure on panic")
		}
		if !strings.Contains(res.ErrorMessage, "panic recovered") {
			t.Errorf("expected panic recovery message, got: %s", res.ErrorMessage)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for panic recovery")
	}

	// Ensure runner released lock and can run another task
	time.Sleep(15 * time.Millisecond)
	if !r.CanAcquireLock() {
		t.Fatal("expected lock to be released after panic recovery")
	}
}

// TestRunner_NonZeroExitCode verifies handling of non-zero exit codes from the subprocess.
func TestRunner_NonZeroExitCode(t *testing.T) {
	mockExec := func(ctx context.Context, dir string, env []string, binPath string, args ...string) ([]byte, int, error) {
		return []byte("syntax error in prompt"), 2, errors.New("exit status 2")
	}

	r := NewAgentRunner(WithCustomExec(mockExec))

	done := make(chan TaskResult, 1)
	err := r.DispatchAsync(context.Background(), TaskRequest{RawPrompt: "bad command"}, func(res TaskResult) {
		done <- res
	})
	if err != nil {
		t.Fatalf("dispatch error: %v", err)
	}

	select {
	case res := <-done:
		if res.Success {
			t.Fatal("expected success=false for exit code 2")
		}
		if res.ExitCode != 2 {
			t.Errorf("expected exit code 2, got %d", res.ExitCode)
		}
		if res.Output != "syntax error in prompt" {
			t.Errorf("expected output preserved, got: %s", res.Output)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for result")
	}
}

// TestRunner_ResolvePath verifies tilde expansion for paths.
func TestRunner_ResolvePath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = os.Getenv("HOME")
	}
	if home == "" {
		home = "/data/data/com.termux/files/home"
	}

	cases := []struct {
		input    string
		expected string
	}{
		{"~", home},
		{"~/", home},
		{"~/.local/bin/agy", filepath.Join(home, ".local/bin/agy")},
		{"/usr/local/bin/agy", "/usr/local/bin/agy"},
		{"relative/path/bin", "relative/path/bin"},
	}

	for _, tc := range cases {
		got := resolvePath(tc.input)
		if got != tc.expected {
			t.Errorf("resolvePath(%q) = %q; expected %q", tc.input, got, tc.expected)
		}
	}
}

// TestRunner_DefaultOptions verifies constructor defaults and option guards.
func TestRunner_DefaultOptions(t *testing.T) {
	r := NewAgentRunner()
	if r.binPath != "~/.local/bin/agy" {
		t.Errorf("expected default binPath ~/.local/bin/agy, got %q", r.binPath)
	}
	if r.workDir != "/data/data/com.termux/files/home/go-app" {
		t.Errorf("expected default workDir, got %q", r.workDir)
	}
	if r.timeout != 3*time.Minute {
		t.Errorf("expected default timeout 3m, got %v", r.timeout)
	}

	// Zero or negative timeout should retain default
	r2 := NewAgentRunner(WithCustomTimeout(0))
	if r2.timeout != 3*time.Minute {
		t.Errorf("expected 0 timeout to retain default 3m, got %v", r2.timeout)
	}

	r3 := NewAgentRunner(WithCustomTimeout(-1 * time.Minute))
	if r3.timeout != 3*time.Minute {
		t.Errorf("expected negative timeout to retain default 3m, got %v", r3.timeout)
	}

	// nil exec should retain default
	r4 := NewAgentRunner(WithCustomExec(nil))
	if r4.execRunner == nil {
		t.Error("expected non-nil default execRunner when nil passed")
	}
}

// TestRunner_NilOnComplete verifies that passing nil as onComplete does not panic.
func TestRunner_NilOnComplete(t *testing.T) {
	mockExec := func(ctx context.Context, dir string, env []string, binPath string, args ...string) ([]byte, int, error) {
		return []byte("success"), 0, nil
	}

	r := NewAgentRunner(WithCustomExec(mockExec))
	err := r.DispatchAsync(context.Background(), TaskRequest{RawPrompt: "nil callback test"}, nil)
	if err != nil {
		t.Fatalf("unexpected dispatch error: %v", err)
	}

	// Give goroutine time to complete
	time.Sleep(30 * time.Millisecond)

	if !r.CanAcquireLock() {
		t.Fatal("expected runner to return to idle after nil-callback task completes")
	}
}

// TestRunner_ReentrantCallbackSafety asserts that onComplete can query runner status
// without causing a deadlock (proving mutex was released before callback was invoked).
func TestRunner_ReentrantCallbackSafety(t *testing.T) {
	var r *AgentRunner
	mockExec := func(ctx context.Context, dir string, env []string, binPath string, args ...string) ([]byte, int, error) {
		return []byte("reentrant ok"), 0, nil
	}

	r = NewAgentRunner(WithCustomExec(mockExec))

	done := make(chan bool, 1)
	err := r.DispatchAsync(context.Background(), TaskRequest{RawPrompt: "reentrant test"}, func(res TaskResult) {
		// Calling Status() and CanAcquireLock() inside onComplete would deadlock
		// if the mutex was still held by the runner goroutine.
		st := r.Status()
		canLock := r.CanAcquireLock()
		isRun := r.IsRunning()

		if isRun {
			t.Error("expected IsRunning to be false in onComplete")
		}
		if !canLock {
			t.Error("expected CanAcquireLock to be true in onComplete")
		}
		if st.IsRunning {
			t.Error("expected Status.IsRunning to be false in onComplete")
		}
		done <- true
	})
	if err != nil {
		t.Fatalf("dispatch error: %v", err)
	}

	select {
	case <-done:
		// Success! No deadlock occurred.
	case <-time.After(2 * time.Second):
		t.Fatal("DEADLOCK DETECTED: onComplete could not query runner status")
	}
}

// TestRunner_EffortFlagPassed verifies that Effort specified in TaskRequest is passed as --effort to the agy command.
func TestRunner_EffortFlagPassed(t *testing.T) {
	var capturedArgs []string
	mockExec := func(ctx context.Context, dir string, env []string, binPath string, args ...string) ([]byte, int, error) {
		capturedArgs = args
		return []byte("done"), 0, nil
	}

	r := NewAgentRunner(WithCustomExec(mockExec))

	done := make(chan bool, 1)
	err := r.DispatchAsync(context.Background(), TaskRequest{
		RawPrompt: "check battery status",
		Effort:    "low",
	}, func(res TaskResult) {
		done <- true
	})
	if err != nil {
		t.Fatalf("dispatch error: %v", err)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for task completion")
	}

	foundEffort := false
	for i, arg := range capturedArgs {
		if arg == "--effort" && i+1 < len(capturedArgs) && capturedArgs[i+1] == "low" {
			foundEffort = true
			break
		}
	}
	if !foundEffort {
		t.Fatalf("expected args to contain '--effort low', got: %v", capturedArgs)
	}
}
