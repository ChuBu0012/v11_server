package bot

import (
	"context"
	"fmt"
	"html"
	"os"
	"os/exec"
	"strings"
	"time"
)

// WhitelistedCommands is kept for backward compatibility.
var WhitelistedCommands = map[string]bool{
	"ps":     true,
	"df":     true,
	"free":   true,
	"uptime": true,
	"pm2":    true,
}

// IsWhitelistedCommand checks if the command is in the immutable whitelist.
func IsWhitelistedCommand(name string) bool {
	switch name {
	case "ps", "df", "free", "uptime", "pm2":
		return true
	default:
		return false
	}
}

// ExecOutputRunner defines execution function returning combined stdout/stderr.
type ExecOutputRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

// DefaultExecRunner executes binary commands directly without a shell.
func DefaultExecRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmdPath := name
	termuxBin := "/data/data/com.termux/files/usr/bin/" + name
	if _, err := os.Stat(termuxBin); err == nil {
		cmdPath = termuxBin
	}

	cmd := exec.CommandContext(ctx, cmdPath, args...)
	cmd.Env = append(os.Environ(), "PATH=/data/data/com.termux/files/usr/bin:/system/bin:"+os.Getenv("PATH"))
	return cmd.CombinedOutput()
}

// SafeExecutor validates and executes permitted diagnostic commands.
type SafeExecutor struct {
	runner    ExecOutputRunner
	timeout   time.Duration
	isAllowed func(string) bool
}

// SafeExecutorOption configures a SafeExecutor.
type SafeExecutorOption func(*SafeExecutor)

// WithExecRunner overrides the command runner for testing.
func WithExecRunner(r ExecOutputRunner) SafeExecutorOption {
	return func(e *SafeExecutor) {
		e.runner = r
	}
}

// WithExecTimeout overrides the default command execution timeout.
func WithExecTimeout(d time.Duration) SafeExecutorOption {
	return func(e *SafeExecutor) {
		e.timeout = d
	}
}

// WithAllowedCheck overrides the command whitelist checker (useful for testing).
func WithAllowedCheck(check func(string) bool) SafeExecutorOption {
	return func(e *SafeExecutor) {
		e.isAllowed = check
	}
}

// NewSafeExecutor creates a SafeExecutor with 5s timeout and default runner.
func NewSafeExecutor(opts ...SafeExecutorOption) *SafeExecutor {
	e := &SafeExecutor{
		runner:    DefaultExecRunner,
		timeout:   5 * time.Second, // 5s timeout per spec
		isAllowed: IsWhitelistedCommand,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Execute parses, validates, and runs a command string.
func (e *SafeExecutor) Execute(ctx context.Context, input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return "⚠️ Please specify a command to execute.\n<i>Allowed commands: ps, df, free, uptime, pm2</i>"
	}

	// 1. Detect any shell metacharacters / injection attempts
	forbiddenChars := []string{";", "&", "|", "`", "$", ">", "<", "\n", "\r", "\x00", "(", ")", "{", "}"}
	for _, c := range forbiddenChars {
		if strings.Contains(input, c) {
			return "🚫 <b>Security Error:</b> Shell chaining and metacharacters are strictly forbidden."
		}
	}

	// 2. Tokenize into command and arguments
	fields := strings.Fields(input)
	if len(fields) == 0 {
		return "⚠️ Please specify a command to execute."
	}

	cmdName := fields[0]
	args := fields[1:]

	// 3. Verify against immutable whitelist
	allowed := false
	if e.isAllowed != nil {
		allowed = e.isAllowed(cmdName)
	} else {
		allowed = IsWhitelistedCommand(cmdName)
	}

	if !allowed {
		return fmt.Sprintf("❌ <b>Command '%s' is not permitted.</b>\n<i>Allowed commands: ps, df, free, uptime, pm2</i>", html.EscapeString(cmdName))
	}

	// 4. Run with context timeout
	execCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	outputBytes, err := e.runner(execCtx, cmdName, args...)
	output := string(outputBytes)

	if execCtx.Err() == context.DeadlineExceeded {
		return fmt.Sprintf("⏱️ <b>Command timed out:</b> Exceeded %v execution limit.", e.timeout)
	}

	if err != nil && len(output) == 0 {
		output = fmt.Sprintf("Execution error: %v", err)
	}

	// 5. Truncate if larger than 3500 chars (Telegram message limit: 4096)
	maxLen := 3500
	if len(output) > maxLen {
		output = output[:maxLen] + "\n... [truncated due to message size limit]"
	}

	if strings.TrimSpace(output) == "" {
		output = "(no output)"
	}

	return fmt.Sprintf("⚡ <b>Output (%s):</b>\n<pre>%s</pre>", html.EscapeString(input), html.EscapeString(output))
}
