# Project Memory: v11 Pocket Server (Pocket Sentinel)

## Purpose
A modular lightweight Go Gin HTTP service running on vivo 1804 (V11) Android 10 (Termux arm64) managed by PM2. Functions as "Pocket Sentinel" — an autonomous health and telemetry monitoring service with Apple HIG-inspired web dashboard, SQLite time-series telemetry storage, configurable threshold alert state machine, and Telegram notifications, exposed via ngrok tunnel.

## Scope
- **In-Scope:**
  - Lightweight Go HTTP server (< 300MB RAM budget, using Gin in ReleaseMode, currently running at ~14.0MB).
  - Multi-app modular monolith structure:
    - `shared/db`: Embedded pure-Go SQLite (`modernc.org/sqlite`) for logs, telemetry samples, and alert states.
    - `shared/config`: Centralized typed config with secret redaction and safe defaults.
    - `shared/worker`: Background ticker worker lifecycle with context cancellation.
    - `modules/telemetry`: Periodic collector (battery, RAM, disk, uptime), time-series repo, retention cleanup, `/api/telemetry/history` and `/api/telemetry/current`.
    - `modules/alerts`: Pure threshold evaluation, SQLite alert state persistence, edge-triggered state machine (alert on OK->Problem, suppress on Problem->Problem, recover on Problem->OK), Telegram notifier client.
    - `modules/dashboard`: Responsive single-page dashboard strictly adhering to Apple design specification (`DESIGN.md`), embedded via `//go:embed` at `GET /`.
    - `modules/system` & `modules/logs`: System info, Termux API battery querying, and event logs.
  - Process orchestration via `ecosystem.config.js` (`go-server` + `ngrok-tunnel`).
  - Cross-compile build & deploy pipeline (`scripts/deploy.sh`, `scripts/status.sh`).
- **Out-of-Scope (Initial Phase):**
  - Disruptive hardware control (torch, vibration, notification, audio) without explicit user permission.
  - Heavy external frameworks or CGO dependencies.

## Current State
- All 16 spec tasks (`tasks.md` T001–T016) implemented and verified with passing tests (`go test -v ./...`).
- Deployed and verified on vivo 1804 (v11):
  - `go-server` (PID 25778): online, ~14.0MB RAM.
  - `ngrok-tunnel` (PID 26473): online, ~2.4MB RAM.
- Active Public Endpoint: `https://spindle-said-hemstitch.ngrok-free.dev`.
- Web dashboard accessible at `https://spindle-said-hemstitch.ngrok-free.dev/` with live 30s auto-refresh.
- Telemetry collector actively sampling and persisting health samples every 60s into SQLite.

## Decisions
- [2026-11-14] Switched from Cloudflare to ngrok for public tunneling with free static domain.
- [2026-11-14] Wrapped ngrok in `run-ngrok.sh` using `termux-chroot` + `SSL_CERT_FILE=/etc/tls/cert.pem` to solve Android Kernel 4.4 syscall issues and TLS root CA lookup in Termux.
- [2026-11-14] Cross-compilation on macOS: `CGO_ENABLED=0 GOOS=android GOARCH=arm64` (never `GOOS=linux` due to Kernel 4.4 `faccessat2` crash).
- [2026-11-14] Dashboard design conforms strictly to `DESIGN.md` Apple design tokens (Action Blue `#0066cc`, SF Pro font scale, negative tracking, pill CTAs with `scale(0.95)` press state, alternating light/dark canvases).
- [2026-11-14] Alert system is edge-triggered to prevent notification storms; retains state across service restarts.

## Next Steps
- (Optional) Configure Telegram Bot Token (`TELEGRAM_BOT_TOKEN`) and Chat ID (`TELEGRAM_CHAT_ID`) in `.env` if the user wants Telegram notifications enabled.
