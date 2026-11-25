package bot

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	// ErrAgentBusy is returned when a task is dispatched while another is active.
	ErrAgentBusy = errors.New("agent busy: another task is currently running")
	// ErrBinaryNotFound is returned when the agy executable cannot be located.
	ErrBinaryNotFound = errors.New("agy binary not found")
	// ErrTaskTimeout is returned when execution exceeds the allocated timeout.
	ErrTaskTimeout = errors.New("task execution timed out")
)

// DefaultTaskTimeout is the default duration allocated for background agy execution (1 hour).
const DefaultTaskTimeout = 1 * time.Hour

// TaskRequest represents a user request to run an agy task.
type TaskRequest struct {
	ChatID    int64
	RawPrompt string
	Effort    string
	CreatedAt time.Time
}

// AgentTaskStatus reports the live status of the runner.
type AgentTaskStatus struct {
	IsRunning bool
	StartedAt time.Time
	Prompt    string
	Elapsed   time.Duration
}

// SubprocessRunner executes the command and returns combined output, exit code, and error.
type SubprocessRunner func(ctx context.Context, dir string, env []string, binPath string, args ...string) ([]byte, int, error)

// AgentRunner coordinates single-flight background agy subprocess execution.
type AgentRunner struct {
	mu         sync.Mutex
	running    bool
	startedAt  time.Time
	prompt     string
	binPath    string
	workDir    string
	timeout    time.Duration
	execRunner SubprocessRunner
}

// AgentRunnerOption configures an AgentRunner.
type AgentRunnerOption func(*AgentRunner)

// WithCustomBinary configures a custom binary path for the runner.
func WithCustomBinary(binPath string) AgentRunnerOption {
	return func(r *AgentRunner) {
		r.binPath = binPath
	}
}

// WithCustomWorkDir configures a custom working directory for subprocess execution.
func WithCustomWorkDir(workDir string) AgentRunnerOption {
	return func(r *AgentRunner) {
		r.workDir = workDir
	}
}

// WithCustomTimeout configures a custom execution timeout.
func WithCustomTimeout(timeout time.Duration) AgentRunnerOption {
	return func(r *AgentRunner) {
		if timeout > 0 {
			r.timeout = timeout
		}
	}
}

// WithCustomExec configures a custom subprocess execution runner (useful for tests).
func WithCustomExec(exec SubprocessRunner) AgentRunnerOption {
	return func(r *AgentRunner) {
		if exec != nil {
			r.execRunner = exec
		}
	}
}

// NewAgentRunner initializes an AgentRunner with default production configurations.
func NewAgentRunner(opts ...AgentRunnerOption) *AgentRunner {
	r := &AgentRunner{
		binPath:    "~/.local/bin/agy",
		workDir:    "/data/data/com.termux/files/home/go-app",
		timeout:    DefaultTaskTimeout,
		execRunner: DefaultSubprocessRunner,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Timeout returns the configured task execution timeout.
func (r *AgentRunner) Timeout() time.Duration {
	if r == nil {
		return DefaultTaskTimeout
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.timeout
}

// CanAcquireLock checks whether the runner is currently idle without modifying state.
func (r *AgentRunner) CanAcquireLock() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return !r.running
}

// IsRunning reports whether an agy task is currently executing.
func (r *AgentRunner) IsRunning() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running
}

// Status returns a snapshot of the current runner state.
func (r *AgentRunner) Status() AgentTaskStatus {
	r.mu.Lock()
	defer r.mu.Unlock()

	status := AgentTaskStatus{
		IsRunning: r.running,
		StartedAt: r.startedAt,
		Prompt:    r.prompt,
	}
	if r.running && !r.startedAt.IsZero() {
		status.Elapsed = time.Since(r.startedAt)
	}
	return status
}

// DispatchAsync attempts to acquire the single-flight lock and start the task in a goroutine.
// If another task is already executing, ErrAgentBusy is returned immediately.
func (r *AgentRunner) DispatchAsync(ctx context.Context, req TaskRequest, onComplete func(TaskResult)) error {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return ErrAgentBusy
	}
	r.running = true
	r.startedAt = time.Now()
	r.prompt = req.RawPrompt
	r.mu.Unlock()

	go func() {
		var result TaskResult
		var completed bool

		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("[bot:runner] PANIC in async execution: %v", rec)
				result.Success = false
				result.ErrorMessage = fmt.Sprintf("panic recovered: %v", rec)

				r.mu.Lock()
				r.running = false
				r.startedAt = time.Time{}
				r.prompt = ""
				r.mu.Unlock()

				if onComplete != nil && !completed {
					completed = true
					onComplete(result)
				}
			}
		}()

		execCtx, cancel := context.WithTimeout(ctx, r.timeout)
		defer cancel()

		startTime := time.Now()
		log.Printf("[bot:runner] starting async agy execution: chat_id=%d, timeout=%v", req.ChatID, r.timeout)

		cleanEnv := SanitizeEnvironment(os.Environ())
		effort := strings.ToLower(strings.TrimSpace(req.Effort))
		if effort == "median" {
			effort = "medium"
		}
		if effort != "low" && effort != "medium" && effort != "high" && effort != "max" {
			effort = "medium"
		}
		args := []string{
			"--dangerously-skip-permissions",
			"--effort", effort,
			"-p", req.RawPrompt,
		}

		output, exitCode, err := r.execRunner(execCtx, r.workDir, cleanEnv, r.binPath, args...)
		duration := time.Since(startTime)

		result.Duration = duration
		result.ExitCode = exitCode
		result.Output = string(output)

		if err == nil && exitCode == 0 {
			result.Success = true
			log.Printf("[bot:runner] task succeeded: chat_id=%d, duration=%v", req.ChatID, duration)
		} else {
			result.Success = false
			if errors.Is(err, ErrTaskTimeout) || errors.Is(err, context.DeadlineExceeded) || execCtx.Err() == context.DeadlineExceeded {
				result.ErrorMessage = ErrTaskTimeout.Error()
			} else if errors.Is(err, ErrBinaryNotFound) {
				result.ErrorMessage = ErrBinaryNotFound.Error()
			} else if err != nil {
				result.ErrorMessage = err.Error()
			} else {
				result.ErrorMessage = fmt.Sprintf("process exited with status %d", exitCode)
			}
			log.Printf("[bot:runner] task failed: chat_id=%d, exit_code=%d, error=%s, duration=%v",
				req.ChatID, exitCode, result.ErrorMessage, duration)
		}

		// Unlock state BEFORE invoking onComplete callback to avoid re-entrant deadlocks
		r.mu.Lock()
		r.running = false
		r.startedAt = time.Time{}
		r.prompt = ""
		r.mu.Unlock()

		if onComplete != nil {
			completed = true
			onComplete(result)
		}
	}()

	return nil
}

// SanitizeEnvironment strips sensitive parent service credentials from the environment.
func SanitizeEnvironment(environ []string) []string {
	stripped := make([]string, 0, len(environ))
	for _, env := range environ {
		key := env
		if idx := strings.IndexByte(env, '='); idx != -1 {
			key = env[:idx]
		}
		upper := strings.ToUpper(key)
		switch upper {
		case "TELEGRAM_BOT_TOKEN", "GEMINI_API_KEY", "GROQ_API_KEY", "OPENROUTER_API_KEY",
			"NGROK_AUTHTOKEN", "DISCORD_WEBHOOK_URL",
			"LOCATION_SECRET_TOKEN", "LOCATION_VIEWER_TOKEN", "FIRESTORE_CREDENTIALS",
			"RELAY_PIN", "REMINDER_PIN", "WATCHTOWER_PIN":
			continue
		default:
			stripped = append(stripped, env)
		}
	}
	return stripped
}

// resolvePath expands leading ~ to home directory.
func resolvePath(path string) string {
	if strings.HasPrefix(path, "~/") || path == "~" {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			home = os.Getenv("HOME")
		}
		if home == "" {
			home = "/data/data/com.termux/files/home"
		}
		if path == "~" {
			return home
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

// DefaultSubprocessRunner executes the agy subprocess with environment sanitization and timeout control.
func DefaultSubprocessRunner(ctx context.Context, dir string, env []string, binPath string, args ...string) ([]byte, int, error) {
	resolvedBin := resolvePath(binPath)

	if _, err := os.Stat(resolvedBin); err != nil {
		if lp, lErr := exec.LookPath(resolvedBin); lErr == nil {
			resolvedBin = lp
		} else if lp, lErr := exec.LookPath(filepath.Base(binPath)); lErr == nil {
			resolvedBin = lp
		} else {
			return nil, -1, ErrBinaryNotFound
		}
	}

	resolvedDir := dir
	if resolvedDir != "" {
		resolvedDir = resolvePath(resolvedDir)
		if _, err := os.Stat(resolvedDir); err != nil {
			resolvedDir = ""
		}
	}

	cmd := exec.CommandContext(ctx, resolvedBin, args...)
	if resolvedDir != "" {
		cmd.Dir = resolvedDir
	}
	if len(env) > 0 {
		cmd.Env = env
	}

	output, err := cmd.CombinedOutput()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return output, -1, ErrTaskTimeout
	}

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	return output, exitCode, err
}
