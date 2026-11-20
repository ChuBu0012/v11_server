package bot

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

type launchCall struct{ script, message, logPath string }

func newTestDeployer(t *testing.T, pid int, alive bool) (*Deployer, *[]launchCall) {
	t.Helper()
	dir := t.TempDir()
	calls := []launchCall{}
	d := NewDeployer(dir)
	d.launch = func(script, message, logPath string) (int, error) {
		calls = append(calls, launchCall{script, message, logPath})
		return pid, nil
	}
	d.alive = func(int) bool { return alive }
	return d, &calls
}

func TestDeployerStartsAndWritesPID(t *testing.T) {
	d, calls := newTestDeployer(t, 4242, false)
	if err := d.Start("hello"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if len(*calls) != 1 {
		t.Fatalf("launch calls = %d, want 1", len(*calls))
	}
	call := (*calls)[0]
	if call.script != filepath.Join(d.appDir, "scripts", "deploy-v11.sh") || call.logPath != filepath.Join(d.appDir, "logs", "deploy.log") || call.message != "hello" {
		t.Fatalf("unexpected launch call %+v", call)
	}
	data, err := os.ReadFile(filepath.Join(d.appDir, ".deploy.pid"))
	if err != nil || strings.TrimSpace(string(data)) != "4242" {
		t.Fatalf("pid file = %q err=%v", data, err)
	}
}

func TestDeployerRefusesWhileRunning(t *testing.T) {
	d, calls := newTestDeployer(t, 1, true)
	if err := os.WriteFile(filepath.Join(d.appDir, ".deploy.pid"), []byte("777"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := d.Start("x"); !errors.Is(err, ErrDeployRunning) {
		t.Fatalf("err = %v, want ErrDeployRunning", err)
	}
	if len(*calls) != 0 {
		t.Fatalf("launch must not be called while a deploy is running")
	}
}

func TestDeployerIgnoresStalePID(t *testing.T) {
	// Process gone.
	d, calls := newTestDeployer(t, 5, false)
	pidPath := filepath.Join(d.appDir, ".deploy.pid")
	_ = os.WriteFile(pidPath, []byte("777"), 0o644)
	if err := d.Start("x"); err != nil || len(*calls) != 1 {
		t.Fatalf("dead pid: err=%v calls=%d", err, len(*calls))
	}
	// PID alive but file too old (likely PID reuse).
	d2, calls2 := newTestDeployer(t, 6, true)
	pidPath2 := filepath.Join(d2.appDir, ".deploy.pid")
	_ = os.WriteFile(pidPath2, []byte("777"), 0o644)
	old := time.Now().Add(-20 * time.Minute)
	_ = os.Chtimes(pidPath2, old, old)
	if err := d2.Start("x"); err != nil || len(*calls2) != 1 {
		t.Fatalf("old pid file: err=%v calls=%d", err, len(*calls2))
	}
}

func TestDeployerPassesMessageAsArgument(t *testing.T) {
	msg := `"; rm -rf ~ #`
	got := deployArgs("/app/scripts/deploy-v11.sh", msg, "/app/logs/deploy.log")
	want := []string{"bash", "-c", deployShell, "/app/scripts/deploy-v11.sh", msg, "/app/logs/deploy.log"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("argv = %#v\nwant %#v", got, want)
	}
	if strings.Contains(deployShell, "rm -rf") {
		t.Fatal("message must never be interpolated into the shell string")
	}
}

type fakeStarter struct {
	messages []string
	err      error
}

func (f *fakeStarter) Start(message string) error {
	f.messages = append(f.messages, message)
	return f.err
}

func sendCommand(t *testing.T, r *Router, chatID int64, text string) {
	t.Helper()
	if err := r.HandleUpdate(context.Background(), Update{UpdateID: 1, Message: &Message{MessageID: 1, Chat: &Chat{ID: chatID}, Text: text}}); err != nil {
		t.Fatalf("HandleUpdate(%q): %v", text, err)
	}
}

func TestRouter_Deploy(t *testing.T) {
	auth := NewAuthorizer("123456", nil)

	sender := &mockDirectSender{}
	starter := &fakeStarter{}
	r := NewRouter(nil, auth, nil, nil, nil, WithTelegramSender(sender), WithDeployer(starter))
	sendCommand(t, r, 123456, "/deploy  แก้แผนที่ ")
	if len(starter.messages) != 1 || starter.messages[0] != "แก้แผนที่" {
		t.Fatalf("starter messages = %#v", starter.messages)
	}
	if len(sender.sent) != 1 || !strings.Contains(sender.sent[0], "รับคำสั่ง deploy") {
		t.Fatalf("reply = %#v", sender.sent)
	}

	busySender := &mockDirectSender{}
	busy := NewRouter(nil, auth, nil, nil, nil, WithTelegramSender(busySender), WithDeployer(&fakeStarter{err: ErrDeployRunning}))
	sendCommand(t, busy, 123456, "/deploy")
	if len(busySender.sent) != 1 || !strings.Contains(busySender.sent[0], "กำลังทำงานอยู่") {
		t.Fatalf("busy reply = %#v", busySender.sent)
	}

	noneSender := &mockDirectSender{}
	none := NewRouter(nil, auth, nil, nil, nil, WithTelegramSender(noneSender))
	sendCommand(t, none, 123456, "/deploy")
	if len(noneSender.sent) != 1 || !strings.Contains(noneSender.sent[0], "not configured") {
		t.Fatalf("unconfigured reply = %#v", noneSender.sent)
	}
}

func TestRouter_Deploy_Unauthorized(t *testing.T) {
	starter := &fakeStarter{}
	r := NewRouter(nil, NewAuthorizer("123456", nil), nil, nil, nil, WithTelegramSender(&mockDirectSender{}), WithDeployer(starter))
	sendCommand(t, r, 999, "/deploy")
	if len(starter.messages) != 0 {
		t.Fatalf("unauthorized chat triggered a deploy: %#v", starter.messages)
	}
}

// Runs the real launcher against a stub script: the script must receive the
// message verbatim and must not be a child of this process (pm2 tree-kill).
func TestLaunchDetachedReparentsAndPassesMessage(t *testing.T) {
	dir := t.TempDir()
	scripts := filepath.Join(dir, "scripts")
	_ = os.MkdirAll(scripts, 0o755)
	out := filepath.Join(dir, "args.txt")
	stub := "#!/bin/bash\nprintf '%s|%s' \"$1\" \"$2\" > " + out + "\nsleep 2\n"
	script := filepath.Join(scripts, "deploy-v11.sh")
	if err := os.WriteFile(script, []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	msg := `"; echo pwned #`
	pid, err := launchDetached(script, msg, filepath.Join(dir, "deploy.log"))
	if err != nil || pid <= 0 {
		t.Fatalf("launchDetached pid=%d err=%v", pid, err)
	}
	ppidOut, err := exec.Command("ps", "-o", "ppid=", "-p", strconv.Itoa(pid)).Output()
	if err == nil {
		if ppid, _ := strconv.Atoi(strings.TrimSpace(string(ppidOut))); ppid == os.Getpid() {
			t.Fatalf("deploy script is still a child of the server (ppid=%d)", ppid)
		}
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if data, _ := os.ReadFile(out); len(data) > 0 {
			if string(data) != "-m|"+msg {
				t.Fatalf("script args = %q, want %q", data, "-m|"+msg)
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("stub script never ran")
}
