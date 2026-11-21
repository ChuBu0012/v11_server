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
- [2026-11-21 | Testing] Gin ReverseProxy tests: Wrap `httptest.ResponseRecorder` with `http.CloseNotifier` (`testCloseNotifierRecorder`) to prevent runtime type-assertion panics in unit tests.

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
