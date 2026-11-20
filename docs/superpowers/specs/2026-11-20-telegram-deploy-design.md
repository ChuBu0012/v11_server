# Telegram `/deploy` — Short Design

Date: 2026-11-20 · Small change, approved by the owner in chat.

- **Intent:** let the owner deploy the latest pushed `main` to the vivo v11 from Telegram instead of SSHing in. The owner commits and pushes, then sends `/deploy`.
- **Goals:**
  - `/deploy [message]` starts `scripts/deploy-v11.sh` immediately, with no confirmation step (owner's choice).
  - The bot replies with an acknowledgement.
  - The script's existing Telegram notifications (start / success / failure / rollback) report the outcome.
  - A second `/deploy` while one is running is refused.
- **Non-goals:** no confirmation button, no commit preview, no new deploy logic in Go.
- **Anti-goals:**
  - The deploy being killed half-way by its own `pm2 restart`.
  - Reporting "success" when `git pull` failed.
- **Constraints:**
  - Only the owner chat may use the command; the existing `Authorizer` already drops other chats.
  - The message is passed as an argument, never interpolated into a shell string.
  - `pm2 restart` uses tree-kill, which walks parent → child PIDs, so a `setsid` child of `go-server` would still be killed.
- **Approaches:**
  1. **The bot launches the existing script fully detached (chosen).** The script is launched through a double fork (`bash -c 'setsid nohup bash script … & echo $!'`), so it is reparented to init and survives the restart. This reuses build, health check, rollback and notifications.
  2. Calling the script as a normal child is ruled out because tree-kill stops it mid-deploy.
  3. Reimplementing deploy in Go is ruled out because it duplicates the script.
- **Design:**
  - `modules/bot/deploy.go` defines a `Deployer` that holds `appDir`, the script path, `logs/deploy.log` and `.deploy.pid`.
    - `Start(message string) error` returns `ErrDeployRunning` when the PID in `.deploy.pid` is alive and the file is younger than 15 minutes.
    - Otherwise it double-forks the script with stdout and stderr appended to the log, then writes the new PID.
    - The launcher and liveness check can be injected for tests.
  - The router gets a `/deploy` case and a `WithDeployer` option. Replies:
    - On success: "🚀 รับคำสั่ง deploy แล้ว…"
    - When a deploy is already running: "⏳ มี deploy กำลังทำงานอยู่…"
    - On error: "❌ เริ่ม deploy ไม่ได้: …"
    - When no deployer is configured: "⚠️ Deploy not configured"
  - `/help` lists `/deploy`.
  - `main.go` wires `bot.NewDeployer(appDir)`, where `appDir` is the working directory of the running server.
  - `scripts/deploy-v11.sh` changes:
    - If `git pull` fails, the script sends a failure notification and exits 1, without building or restarting.
    - When no message is given, the default message comes from the latest commit **after** the pull, not before.
- **Left to the builder:** exact Thai wording beyond the lines above.
