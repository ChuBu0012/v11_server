package bot

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// ErrDeployRunning is returned when a previous deploy is still in progress.
var ErrDeployRunning = errors.New("deploy already running")

// DeployStarter starts a deploy without waiting for it to finish.
type DeployStarter interface {
	Start(message string) error
}

// deployStaleAfter bounds how long a PID file is trusted; after this the PID
// may have been reused by an unrelated process, so a new deploy is allowed.
const deployStaleAfter = 15 * time.Minute

// deployShell double-forks the deploy script so it is reparented to init.
// The outer shell already runs in its own session (SysProcAttr.Setsid); it
// backgrounds the script, prints its PID and exits, orphaning the script.
// pm2 restarts go-server with tree-kill (walking parent→child PIDs), so a
// plain child process — even one in its own session — would be killed by the
// very restart it triggers. Arguments arrive as $0 (script), $1 (message) and
// $2 (log path); the message is never interpolated into this string.
const deployShell = `nohup bash "$0" -m "$1" >>"$2" 2>&1 </dev/null & echo $!`

// Deployer runs scripts/deploy-v11.sh detached from the bot's own process.
type Deployer struct {
	appDir string
	launch func(script, message, logPath string) (int, error)
	alive  func(pid int) bool
}

// NewDeployer creates a deployer rooted at the server's working directory.
func NewDeployer(appDir string) *Deployer {
	return &Deployer{appDir: appDir, launch: launchDetached, alive: processAlive}
}

func (d *Deployer) pidPath() string { return filepath.Join(d.appDir, ".deploy.pid") }

// Start launches a deploy, or returns ErrDeployRunning if one is in progress.
func (d *Deployer) Start(message string) error {
	if d.running() {
		return ErrDeployRunning
	}
	script := filepath.Join(d.appDir, "scripts", "deploy-v11.sh")
	logPath := filepath.Join(d.appDir, "logs", "deploy.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return fmt.Errorf("prepare log dir: %w", err)
	}
	pid, err := d.launch(script, strings.TrimSpace(message), logPath)
	if err != nil {
		return err
	}
	return os.WriteFile(d.pidPath(), []byte(strconv.Itoa(pid)), 0o644)
}

func (d *Deployer) running() bool {
	info, err := os.Stat(d.pidPath())
	if err != nil || time.Since(info.ModTime()) > deployStaleAfter {
		return false
	}
	data, err := os.ReadFile(d.pidPath())
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	return err == nil && pid > 0 && d.alive(pid)
}

func deployArgs(script, message, logPath string) []string {
	return []string{"bash", "-c", deployShell, script, message, logPath}
}

func launchDetached(script, message, logPath string) (int, error) {
	if _, err := os.Stat(script); err != nil {
		return 0, fmt.Errorf("deploy script not found: %w", err)
	}
	args := deployArgs(script, message, logPath)
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = filepath.Dir(filepath.Dir(script))
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	out, err := cmd.Output() // the outer shell exits as soon as it has forked
	if err != nil {
		return 0, fmt.Errorf("launch deploy: %w", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0, fmt.Errorf("read deploy pid: %w", err)
	}
	return pid, nil
}

func processAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}
