package bot

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestExecutor(t *testing.T) {
	mockRunner := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		switch name {
		case "df":
			return []byte("Filesystem 107G 32G 74G 31% /data"), nil
		case "uptime":
			return []byte(" 02:22:00 up 3:15, 1 user, load average: 0.15, 0.12, 0.08"), nil
		case "slow":
			select {
			case <-time.After(2 * time.Second):
				return []byte("done"), nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		default:
			return []byte("mock output for " + name), nil
		}
	}

	exec := NewSafeExecutor(WithExecRunner(mockRunner), WithExecTimeout(50*time.Millisecond))
	ctx := context.Background()

	// 1. Whitelisted command (df -h)
	out := exec.Execute(ctx, "df -h")
	if !strings.Contains(out, "Filesystem 107G 32G 74G 31% /data") {
		t.Errorf("Expected df output, got: %s", out)
	}
	if !strings.Contains(out, "<pre>") || !strings.Contains(out, "</pre>") {
		t.Errorf("Expected HTML pre tags in output, got: %s", out)
	}

	// 2. Whitelisted command (uptime)
	out = exec.Execute(ctx, "uptime")
	if !strings.Contains(out, "load average") {
		t.Errorf("Expected uptime output, got: %s", out)
	}

	// 3. Unwhitelisted command rejection
	unwhitelisted := []string{
		"rm -rf /",
		"cat /data/data/com.termux/files/home/.bashrc",
		"ls -la",
		"sh",
		"reboot",
		"curl http://malicious.site",
	}
	for _, cmd := range unwhitelisted {
		out = exec.Execute(ctx, cmd)
		if !strings.Contains(out, "is not permitted") {
			t.Errorf("Expected command '%s' to be rejected, got: %s", cmd, out)
		}
	}

	// 4. Injection and metacharacter rejection
	injectionAttempts := []string{
		"ps; rm -rf /",
		"df | grep data",
		"free && uptime",
		"uptime `whoami`",
		"df $(whoami)",
		"pm2 > /tmp/evil",
		"uptime < /dev/zero",
		"ps\nls",
		"df\rwhoami",
	}
	for _, inj := range injectionAttempts {
		out = exec.Execute(ctx, inj)
		if !strings.Contains(out, "metacharacters are strictly forbidden") {
			t.Errorf("Expected injection '%s' to be rejected with security error, got: %s", inj, out)
		}
	}

	// 5. Empty command
	out = exec.Execute(ctx, "")
	if !strings.Contains(out, "Please specify a command") {
		t.Errorf("Expected prompt for empty command, got: %s", out)
	}

	// 6. Timeout handling (using isolated executor with allowed 'slow' command)
	timeoutExec := NewSafeExecutor(
		WithExecRunner(mockRunner),
		WithExecTimeout(50*time.Millisecond),
		WithAllowedCheck(func(cmd string) bool { return cmd == "slow" || IsWhitelistedCommand(cmd) }),
	)
	out = timeoutExec.Execute(ctx, "slow")
	if !strings.Contains(out, "timed out") {
		t.Errorf("Expected timeout message, got: %s", out)
	}

	// 7. Output truncation
	largeRunner := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return []byte(strings.Repeat("A", 4000)), nil
	}
	largeExec := NewSafeExecutor(WithExecRunner(largeRunner))
	out = largeExec.Execute(ctx, "ps")
	if !strings.Contains(out, "[truncated due to message size limit]") {
		t.Errorf("Expected truncation message for large output, got length: %d", len(out))
	}
}
