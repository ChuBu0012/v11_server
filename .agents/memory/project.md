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
- [2026-11-18] Completed Pocket Sentinel v2.1 (Telegram-to-agy Async Agent Bridge with Multi-Layer Prompt Injection Guard, Tasks T101–T106).
- [2026-11-18] Fixed Three.js and Chart.js proxy conflicts: extracted `tc` and `chartInstances` completely outside Alpine.js reactive data proxies to eliminate `modelViewMatrix` TypeError and ensure continuous charts rendering with direct fallback loader.
- [2026-11-18] Added dynamic reasoning effort parsing (`[low|medium|high|max]` / `--effort=...`) in Telegram `/task` and passed to `agy`.
- [2026-11-18] Implemented dual-level Telegram lifecycle & deploy notifications (boot/shutdown in `main.go`, start/error/success in `scripts/deploy.sh`) and 15s in-place progress heartbeat.
- [2026-11-18] Configured deterministic post-tool quality guardrail hook (`.agents/hooks.json` & `scripts/hook-validate.sh`) enforcing `gofmt -w` and `go vet ./...`.
- [2026-11-18] Successfully committed, pushed to GitHub `origin/main`, pulled directly on vivo v11, recompiled standalone ARM64 binary on-device, and reloaded PM2 `go-server` (PID 4967, ~11.7MB RAM, Health Check 200 OK).
- [2026-11-19] Modernized Pocket Sentinel Live Map UI to Modern Utility Dashboard standard (Deep Navy dark mode + Clean Slate light mode, compact 56px header, grouped floating map controls, marker click profile cards, 3-column device cards with battery progress bar, compact expandable geofence list, iOS toggles, and masked PIN input).
- [2026-11-19] Fixed Place Picker wizard flow: removed invalid HTML5 hidden field constraints, moved Geofence radius slider/presets directly into Step 1 for real-time map preview circle adjustment, added place suggestion chips in Step 2.
- [2026-11-19] Added System Mandate rule 6 in `modules/bot/guard.go` and `ExtractSummary()` in `modules/bot/scrubber.go` so `/task` executions in Telegram bot automatically attach a structured Thai summary block above raw logs.
- [2026-11-19] Performance optimization pass: SQLite `cache_size=-8000` (8MB page cache) + `mmap_size=268435456` (256MB mmap I/O) + `temp_store=MEMORY` PRAGMAs added to `shared/db/db.go`. DB pool tuned to `MaxOpenConns=3`, `MaxIdleConns=3` for WAL reader reuse on ARM64. HTTP gzip middleware (`gzipMiddleware()` with `sync.Pool` gzip writer) added to `main.go` for all responses. `hook-validate.sh` updated to scope `go vet` to affected package instead of `./...` for faster feedback.
- [2026-11-19] Enhanced `scripts/deploy.sh` and `scripts/deploy-v11.sh` with `-m` / `--message` flag support, git commit auto-detection fallback, and HTML-safe Telegram broadcast notifications on deploy start, success, and error.
- [2026-11-19] Verified 100% passing tests, deployed to v11 (PM2 PID 27267, HTTP 200 OK), and pushed to `origin/main`.
