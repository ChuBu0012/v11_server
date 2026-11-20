# Telegram /deploy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Sending `/deploy [message]` in Telegram runs `scripts/deploy-v11.sh` on the v11 as a fully detached process.

**Architecture:** Add a `Deployer` in `modules/bot`, injected into `Router` through `WithDeployer`. It double-forks the existing script and tracks it with a PID file. The script now aborts on a failed `git pull`.

**Tech Stack:** Go (os/exec, syscall) and Bash.

**Spec:** `docs/superpowers/specs/2026-11-20-telegram-deploy-design.md`

## Global Constraints

- The deploy message is passed to the script as a positional argument (`"$1"`). It is never interpolated into the shell string.
- The script is launched as `bash -c 'setsid nohup bash "$0" -m "$1" >>"$2" 2>&1 </dev/null & echo $!' <script> <msg> <log>`, so it is reparented away from go-server and survives pm2 tree-kill.
- A deploy counts as running when the PID in `.deploy.pid` is alive (`syscall.Kill(pid, 0) == nil`) and the file is less than 15 minutes old.

## Review Focus

- A stale `.deploy.pid` (process gone, or file older than 15 min) must not block a new deploy. Test: `TestDeployerIgnoresStalePID`.
- A message containing shell metacharacters (`"; rm -rf ~ #`) must reach the script verbatim as one argument. Test: `TestDeployerPassesMessageAsArgument` asserts the exact argv.
- A deploy from a chat other than the owner must be ignored. This is covered by the existing `Authorizer`; `TestRouter_Deploy_Unauthorized` asserts the deployer is not called.

---

### Task 1: Deployer + /deploy command

**Files:**
- Create: `modules/bot/deploy.go`, `modules/bot/deploy_test.go`
- Modify: `modules/bot/router.go` (field, `WithDeployer`, `/deploy` case, help line), `main.go` (wiring)

**Interfaces:**
- `var ErrDeployRunning = errors.New("deploy already running")`
- `type DeployStarter interface { Start(message string) error }`
- `func NewDeployer(appDir string) *Deployer`. Its unexported fields `launch func(script, message, logPath string) (int, error)` and `alive func(pid int) bool` can be swapped in tests.
- `func (d *Deployer) Start(message string) error`
- `func WithDeployer(d DeployStarter) RouterOption`

- [ ] **Step 1: Write the failing tests.**
  - `TestDeployerStartsAndWritesPID`: the fake launch returns 4242. Assert that `.deploy.pid` contains `4242`, and that launch received script `<dir>/scripts/deploy-v11.sh` and log `<dir>/logs/deploy.log`.
  - `TestDeployerRefusesWhileRunning`: the PID file is fresh and `alive` returns true, so `Start` returns `ErrDeployRunning` and launch is not called.
  - `TestDeployerIgnoresStalePID`: in one case `alive` returns false, and in another the file mtime is set 20 minutes in the past. In both, launch is called.
  - `TestDeployerPassesMessageAsArgument`: uses the real launcher argv builder `deployArgs(script, msg, log) []string` and asserts the argv is exactly `["bash","-c",<const>, script, msg, log]` for msg `"; rm -rf ~ #`.
  - `TestRouter_Deploy`:
    - `/deploy hello` → the fake starter gets `"hello"` and the reply contains "รับคำสั่ง deploy".
    - When the starter returns `ErrDeployRunning`, the reply contains "กำลังทำงานอยู่".
    - With no deployer configured, the reply contains "not configured".
  - `TestRouter_Deploy_Unauthorized`: chat 999 is not called.
- [ ] **Step 2: Run** `go test ./modules/bot/ -run 'Deploy'`. Expect FAIL (undefined).
- [ ] **Step 3: Implement** `deploy.go`, the router case, the help line, and in `main.go` add `routerOpts = append(routerOpts, bot.WithDeployer(bot.NewDeployer(cwd)))`, where `cwd` comes from `os.Getwd()`.
- [ ] **Step 4: Run** `go test ./modules/bot/ && go vet ./...`. Expect PASS.
- [ ] **Step 5: Commit** with message `feat(bot): /deploy runs deploy-v11.sh detached from Telegram`.

### Task 2: deploy-v11.sh — abort on failed pull, post-pull default message

**Files:** Modify `scripts/deploy-v11.sh`

- [ ] **Step 1:** Move the "default `DEPLOY_MSG` from the latest commit" block to after the git pull step. Before the pull, use a placeholder only for the start notification, "ดึงโค้ดล่าสุดจาก GitHub…".
- [ ] **Step 2:** Replace `git pull origin main || echo "⚠️ …proceeding…"` with: on failure, send "❌ <b>Deploy ยกเลิก:</b> git pull ไม่สำเร็จ" and `exit 1`. Do this before any build or restart, and disable the ERR trap message duplication for this path.
- [ ] **Step 3:** Run `bash -n scripts/deploy-v11.sh`. Expect no output.
- [ ] **Step 4:** Commit with message `fix(deploy): abort on failed git pull; describe the pulled commit`, then `git push origin main`.
