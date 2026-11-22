# Project Memory: v11 Pocket Server (Pocket Sentinel)

## Purpose
A modular lightweight Go Gin HTTP service running on vivo 1804 (V11) Android 10 (Termux arm64) managed by PM2. Functions as "Pocket Sentinel" — an autonomous health and telemetry monitoring service with Apple HIG-inspired web dashboard, SQLite time-series telemetry storage, configurable threshold alert state machine, and Telegram notifications, accessible via Tailscale private mesh.

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
  - Process orchestration via `ecosystem.config.js` (`go-server`).
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
  - Active Endpoint: `http://100.92.194.87:8080` (Tailscale Private Mesh).

## Decisions
- [2026-11-21] Decommissioned public ngrok ingress and transitioned entirely to Tailscale (`http://100.92.194.87:8080`) to eliminate 1GB/month bandwidth quota limits and protocol overhead. Removed ngrok-tunnel PM2 process.
- [2026-11-21] Decommissioned Gatus reverse proxy from Pocket Sentinel (`main.go`) and cleaned up root asset proxy routes; replaced `/status` with 301 redirect to standalone Gatus at `http://100.92.194.87:8081/`. Removed inline endpoint card section from `index.html`. Added dedicated Apple HIG Endpoint Manager at `/endpoints` (`modules/dashboard/endpoints.html`). Configured Gatus UI floating action button to route to `/endpoints` and set `logo: /apple-touch-icon.png` to fix broken logo. Completely uninstalled ngrok and cloudflared from vivo V11. Bumped `sw.js` cache to `sentinel-shell-v43`. All tests pass.
- [2026-11-21] Implemented Telegram Chat Interactive Control Center (`/panel`) and Persistent Reply Keyboard (Hybrid Pattern). Features 3x2 quick shortcut reply keyboard at bottom of screen (`[ 🎛️ แผงควบคุม ]`, `[ 📊 สถานะระบบ ]`, `[ 🗺️ พิกัดสด ]`, `[ 📰 สรุปประจำวัน ]`, `[ ☀️ มอร์นิ่งอินไซต์ ]`, `[ 🤖 สั่งงาน AI ]`), 4-tab interactive inline card with in-place editing (`[ • 📊 ระบบ • ]`, `[ • 🌐 บริการ • ]`, `[ • 📰 ข่าว/ทอง • ]`, `[ • 🗺️ พิกัด • ]`), clean formatting with zero clutter, and full integration with live battery, RAM, disk, services, gold/stocks digest, and location tracker. All tests pass.
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
- [2026-11-19] Performance optimization pass: SQLite `cache_size=-8000` (8MB page cache) + `mmap_size=268435456` (256MB mmap I/O) + `temp_store=MEMORY` PRAGMAs added to `shared/db/db.go`. DB pool tuned to `MaxOpenConns=3`, `MaxIdleConns=3` for WAL reader reuse on ARM64. Removed custom gzip middleware to prevent HTTP/2 Content-Length mismatch protocol error (`net::ERR_HTTP2_PROTOCOL_ERROR`) across ngrok tunnel. `hook-validate.sh` updated to scope `go vet` to affected package instead of `./...` for faster feedback.
- [2026-11-19] Enhanced `scripts/deploy.sh` and `scripts/deploy-v11.sh` with `-m` / `--message` flag support, git commit auto-detection fallback, and HTML-safe Telegram broadcast notifications on deploy start, success, and error.
- [2026-11-19] Verified 100% passing tests, deployed to v11 (PM2 PID 27267, HTTP 200 OK), and pushed to `origin/main`.
- [2026-11-20] Redesigned Map / Location notifications to Minimal 2-Line pattern (Noti แบบเรียบ: no bullet clutter, middle dots, omit raw lat/lon coords, clean HH:mm time) in `modules/location/notifier.go` and `location_telegram.go`. Updated and documented templates in `.agents/skills/telegram-discord-message-templates/SKILL.md`. Verified all tests and pushed to `origin/main`.
- [2026-11-20] Redesigned Sentinel Dashboard UI to Sentinel Soft Design System (eliminated harsh brutalist RawBlock override; gentle contrast in both dark #1a1b23 and light #ffffff modes, rounded corners with --radius: 18px, Remix Icon system, icon theme toggle, re-ordered information hierarchy: Hero -> Metrics -> System Conditions -> Schedule -> Location -> Digest -> Charts -> Logs, floating rounded bottom navigation with icons, sw.js cache bump v32, updated DESIGN.md). All tests pass.
- [2026-11-20] Mobile-first redesign (spec/plan in docs/superpowers/*/2026-11-20-mobile-first-redesign*). Rewrote all page CSS (~4.8k → ~1k lines): shared `styles.css` + `dashboard.css`/`schedule.css`/`live-map.css`/`relay.css`; dropped Remix Icon for inline SVG sprite. Schedule = compact month grid + selected-day list + add bottom sheet (AI parse inside). Map = full-screen + status chip + friend bottom sheet. sw.js cache v33. All tests pass.
- [2026-11-20] Map places & settings cleanup: PUT /api/location/geofences/:id (edits silently re-baseline geofence state, no Discord alerts), POST /api/location/resolve-link (Google Maps short-link resolver, Google-host allowlist). Map UI: live-dot/theme/⚙/show-both/trail controls column, ⚙ Manage sheet (places + Discord toggles), one-step place sheet (my spot / friend spot / long-press / pasted link / draggable pin), shortcuts Navigate + Copy coords, default fit-both. Schedule add sheet hides the manual form unless AI parse returns fallback. sw.js v35.
- [2026-11-20] Removed all PINs by owner's choice (public): relay send/proofread, schedule writes/parse, location management + resolve-link. RELAY_PIN/REMINDER_PIN config removed; OwnTracks secret token and /live viewer token kept. Frontend PIN UI removed; sw.js v36. Test TestPagesHaveNoPINPrompts guards against regressions.
- [2026-11-20] Telegram /deploy [msg]: bot double-forks scripts/deploy-v11.sh (escapes pm2 tree-kill), .deploy.pid lock (15 min stale), log at logs/deploy.log. deploy-v11.sh now uses git pull --ff-only and aborts (no build/restart) on pull failure; default message = pulled commit + rev range.
- [2026-11-20] Morning insight (modules/insight): Go computes v11 battery/RAM/disk verdicts vs 48h baseline, per-person walk vs total km, places, phone battery, together minutes (≤80 m), today's reminders; narrated OpenRouter → Gemini → template (AI text rejected unless it keeps every number and only b/i/code tags). Delivered 07:00 Bangkok via Telegram (catch-up until 12:00, once per day), /insight (60 s cooldown), Overview card (/api/insight/latest, POST /api/insight/run). insight_reports table. sw.js v37.
- [2026-11-20] Owner chose Telegram-only insight: Overview insight card, insight.js and /api/insight/* routes removed (sw.js v38). /insight and the 07:00 morning push remain.
- [2026-11-21] Redesigned AI Daily Digest to single Telegram message with in-place interactive category buttons (`[🥇 ทอง] [📈 หุ้น] [🤖 AI] / [🏛️ การเมือง] [💡 สาระ] [🏠 ภาพรวม]`) editing via `editMessageText` and `dg:<runID>:<key>` callback routing. Added live Gold prices (world spot from xaus.com + Thai Gold Traders Association classic scrape with calculated estimate fallback) and Thai stock news feeds (Kaohoon, Prachachat Finance). Stored quote snapshots in `digest_history.gold_json`. Updated Web Dashboard digest card with stocks and gold tabs, bumped `sw.js` cache to `sentinel-shell-v39`. All tests pass.
- [2026-11-21] Set up Gatus (automated health & telemetry dashboard in Go) on vivo V11 under `~/gatus` managed by PM2 (`gatus` process, port 8081). Configured SQLite persistence (`gatus.db`), integrated `setupAndroidDNS()` to resolve external domains via Termux/Google DNS (`8.8.8.8:53`) on non-rooted Android, and monitored 3 core endpoints (Pocket Sentinel Local API, Public Ingress ngrok, and Internet Connectivity Cloudflare). Accessible privately via Tailscale at `http://100.92.194.87:8081`. RAM usage ~25MB.
- [2026-11-21] Added Gatus reverse proxy in Pocket Sentinel (`main.go`) serving Gatus health dashboard at `/status/` (and forwarding `/api/v1/*`, `/js/*`, `/css/*`, `/img/*`, and favicon static assets) over public ngrok domain without Tailscale. Patched Gatus Vue Router history base in `app.js` to dynamically support `/status/`, excluded Gatus paths from Pocket Sentinel PWA service worker (`sw.js` cache bumped to `sentinel-shell-v42`), and added `no-cache` headers to prevent stale SPA caches. Updated Dashboard UI endpoint card link to `/status/`. All tests pass.
- [2026-11-21] Fixed SQLite lock contention (`SQLITE_BUSY`): passed PRAGMAs in DSN (`_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)...`) in `shared/db/db.go` so all pool connections inherit busy_timeout, increased `storeOperationTimeout` to 10s, and implemented smart stationary debouncing in `modules/location/service.go` (skipping redundant DB writes for stationary fixes < 5m within 10s). All tests pass.
- [2026-11-21] Fixed Telegram deploy and boot notification failures: (1) Added `cleanReplyMarkup()` in `modules/bot/bot.go` to convert typed nil pointers (`*InlineKeyboardMarkup(nil)`) in `any` interface to untyped nil, preventing `json.Marshal` from outputting `"reply_markup": null` which caused Telegram 400 `object expected as reply markup` on boot notifications and `/start`; (2) In `scripts/deploy.sh` and `scripts/deploy-v11.sh`, enforced `--http1.1 -4 --connect-timeout 4 --max-time 8` with remote SSH fallback to bypass macOS IPv6 route/handshake stalls to `api.telegram.org`; (3) Added 3-attempt retry loop and PM2 logging for server boot notification in `main.go:585-612`. Verified full deploy cycle and boot notification delivery. All tests pass.
- [2026-11-22] Implemented AI Limit Hub (POC) in `modules/ailimit`: autonomous on-device quota collection from Google Antigravity (`agy -p '/usage'`) running on vivo 1804 (v11) Termux with zero Mac dependency. Tracks 5-hour and Weekly limits for Gemini and Claude/GPT models. SQLite persistence (`ai_quota_states`, `ai_quota_snapshots`), 15-minute background collector, debounced Telegram low-quota alerts (<= 15%), actionable Quota Insights, Web Dashboard Chart.js line graph and countdowns, and Telegram `/quota` command. Deployed and verified on live endpoint `http://100.92.194.87:8080/api/limits`.
- [2026-11-22] Streamlined AI Limit Hub UI: combined 5h and weekly limits into a single unified gauge ("หลอดเดียวเข้าใจเลย") reflecting true available bottleneck capacity with concise sub-details. Filtered out Claude/3P models completely from UI, graphs, and alert dispatch to eliminate noise. Rebuilt Chart.js into a single area-filled trend line for Gemini available %. Bumped `sw.js` cache to `sentinel-shell-v45`. Deployed and verified on v11. All tests pass.
- [2026-11-22] Enhanced Telegram Bot Quota Integration: Added System/Question Mandate Rule 7 ("Be concise, direct, and avoid excessive token usage to conserve AI model quota") in `modules/bot/guard.go`. Added 30-second delayed asynchronous quota follow-up notification in `modules/bot/router.go` for `/task` and `/ask` reporting effective quota remaining and used. Made Web UI unified quota card expand to 100% full width (`.quota-card`). Bumped `sw.js` cache to `sentinel-shell-v46`. Deployed to v11 and pushed to `origin/main`.



