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
- All Phase 0–5 tasks (`tasks.md` T001–T024) implemented and verified with 100% passing tests (`go test -v ./...`).
- Phase 5: Two-Way Interactive Telegram Sentinel (`modules/bot`) implemented:
  - Long Polling engine (`bot.go`, `models.go`)
  - Strict single-user authorization gate (`auth.go`)
  - Command routing (`router.go`) for `/start`, `/help`, `/ping`, `/status`, `/reboot`
  - Safe diagnostic runner (`executor.go`) with immutable whitelist (`ps`, `df`, `free`, `uptime`, `pm2`) and 5s timeout
  - Hardware torch controller (`hardware.go`) with `termux-torch` and 3-minute safety auto-off timer
  - Interactive inline keyboards (`keyboards.go`) with in-place editing via `editMessageText`
  - Lifecycle integration in `main.go` and `main_test.go`
- Target binary compiled and uploaded to vivo 1804 (v11):
  - `server: ELF 64-bit LSB pie executable, ARM aarch64, version 1 (SYSV)` (25MB)
  - Remote process memory remains ~18.4MB (budget < 300MB).
  - Active Public Endpoint: `https://spindle-said-hemstitch.ngrok-free.dev`.

## Decisions
- [2026-11-14] Switched from Cloudflare to ngrok for public tunneling with free static domain.
- [2026-11-14] Wrapped ngrok in `run-ngrok.sh` using `termux-chroot` + `SSL_CERT_FILE=/etc/tls/cert.pem` to solve Android Kernel 4.4 syscall issues and TLS root CA lookup in Termux.
- [2026-11-14] Cross-compilation on macOS: `CGO_ENABLED=0 GOOS=android GOARCH=arm64` (never `GOOS=linux` due to Kernel 4.4 `faccessat2` crash).
- [2026-11-14] Dashboard design conforms strictly to `DESIGN.md` Apple design tokens (Action Blue `#0066cc`, SF Pro font scale, negative tracking, pill CTAs with `scale(0.95)` press state, alternating light/dark canvases).
- [2026-11-14] Alert system is edge-triggered to prevent notification storms; retains state across service restarts.
- [2026-11-14] Added `setupAndroidDNS()` to resolve `api.telegram.org` via Termux DNS / `8.8.8.8:53` on non-rooted Android without `/etc/resolv.conf`.
- [2026-11-15] Pocket Sentinel 2.0 adopts Telegram Long Polling (`getUpdates`) for tunnel-independent two-way interaction, strict single-user Chat ID whitelisting, 3-minute auto-off torch protection, and whitelisted safe `/exec`.
- [2026-11-15] Implemented AI Daily Digest (`modules/digest`, `cmd/digest`) using `gemini-3.5-flash-lite`, scheduled at 13:00 Bangkok time via Termux `crond`. Features 3-part sequential Telegram delivery (AI, Politics, Trivia) with 600ms pacing, 2-line concise summaries, and Web Dashboard segmented tab controls (`[ 🤖 AI ]`, `[ 🏛️ การเมือง ]`, `[ 💡 สาระน่ารู้ ]`) with custom Markdown rendering.
- [2026-11-15] Prepared optional Discord webhook notifier (`modules/alerts/discord.go`) with noop fallback when unconfigured.

## Next Steps
- Verify automated daily execution at 13:00 Bangkok time via `crond` on vivo V11.
