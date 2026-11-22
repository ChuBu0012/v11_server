# Rules Memory: v11 Pocket Server

## Must
- [2026-11-19 | User alignment] Pull first & Push last: Always run `git pull origin main` before starting work, and commit + `git push origin main` immediately upon completing work.
- [2026-11-14 | User] Use scp -r for file transfers to v11 (do not use rsync).
- [2026-11-14 | User / AGENTS.md] Keep Go processes lightweight (< 300MB RAM) to avoid Android Low Memory Killer (LMK).
- [2026-11-14 | AGENTS.md] Always compile to a standalone binary before running under PM2; never use `pm2 start "go run ..."`.
- [2026-11-14 | AGENTS.md] Use pure-Go libraries (e.g. `modernc.org/sqlite` without CGO) for ARM64 Linux/Android compatibility.
- [2026-11-14 | AGENTS.md] Always pass `--nostream` when running remote `pm2 logs` via SSH to avoid hanging sessions.
- [2026-11-14 | AGENTS.md] Run `pm2 save` after PM2 process changes for persistence across Termux restarts.
- [2026-11-21 | Learning/Post-mortem] SPA Reverse Proxy: When mounting an external SPA under a subpath (e.g. `/status/`), always patch dynamic router history base (`location.pathname.startsWith('/status') ? '/status/' : '/'`), proxy all root asset paths (`/img/*`, `/js/*`, `/css/*`, favicons), bypass host PWA service worker (`sw.js`), and set `no-cache` headers.
- [2026-11-21 | Learning/Post-mortem] SQLite Connection Pool PRAGMA: In Go `database/sql`, always configure SQLite PRAGMAs (busy_timeout, journal_mode, synchronous) via DSN query parameters (`_pragma=busy_timeout(10000)...`), never via `db.Exec()`, ensuring all connections in the pool inherit the configuration.
- [2026-11-21 | Learning/Post-mortem] Mobile Telemetry Throttling: Debounce high-frequency mobile GPS/telemetry at the service layer (ignore stationary jitter < 5m within 10s with identical battery) to prevent SQLite write lock collisions and bandwidth exhaustion.
- [2026-11-21 | Testing] Gin ReverseProxy tests: Wrap `httptest.ResponseRecorder` with `http.CloseNotifier` (`testCloseNotifierRecorder`) to prevent runtime type-assertion panics in unit tests.
- [2026-11-21 | Learning/Device] Android Termux Port Binding: Android limits unprivileged users (`u0_a244`) from binding to ports < 1024 (e.g. port 80 fails with `EACCES: permission denied`). Always use unprivileged ports (8080, 8081) on Tailscale IP (`http://100.92.194.87:8080`).
- [2026-11-21 | Learning/Device] Termux exec.Command Node Shebang: Node CLI scripts with `#!/usr/bin/env node` fail when spawned via Go `exec.Command` because `/usr/bin/env` does not exist in Android root; invoke `node /data/data/com.termux/files/usr/bin/<cli>` directly.
- [2026-11-22 | User alignment / Post-mortem] Agent Notification: Always execute `~/.local/bin/v11-notify -s success -p "v11_server" -t "..." -m "..."` immediately upon task completion to report status to Telegram without waiting for user prompt.
- [2026-11-22 | Learning / Performance] Fast Remote Deploy: When deploying code already pushed to GitHub, prefer `ssh v11 "cd ~/go-app && ./scripts/deploy-v11.sh -m '...'"` (git pull on-device) over transferring 45MB binaries over slow scp/Tailscale.

## Must Not
- [2026-11-14 | AGENTS.md] Never use `sudo`, `systemctl`, `service`, or `docker` on Android/Termux.
- [2026-11-14 | User alignment] Do not implement disruptive hardware actions (torch, vibrate, TTS, sound, notification) without explicit user permission.

## Approval Required
- [2026-11-14 | User alignment] Implementing any hardware action that impacts physical device state or human user attention.

## Commands
- Build (cross-compile for v11): `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o server .`
- Deploy to v11: `./scripts/deploy.sh`
- Status check: `./scripts/status.sh`
- Remote logs: `ssh v11 "pm2 logs go-server --lines 30 --nostream"`
