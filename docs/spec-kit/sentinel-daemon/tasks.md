# Decoupled SRE Sentinel — Tasks & Evidence of Done

**Status key:** `TODO` → `IN PROGRESS` → `BLOCKED` / `DONE`

---

## T001 — SQLite SRE Store & Schema Migration
**Status:** `TODO`  
**Scope:**
- Add `downtime_incidents` table in `shared/db/db.go` with indexes on `started_at`.
- Implement `modules/sre/store.go` providing `RecordIncidentStart`, `RecordIncidentRecovery`, `GetRecentIncidents`, and `Get24hAvailability`.
- Verify SQLite WAL concurrency under multiple simultaneous readers and writers.

**Acceptance Criteria:**
- Incident starts and recoveries are recorded transactionally with millisecond precision.
- Availability query calculates exact 24h percentage accounting for active and recovered incidents.

**หลักฐานที่บอกว่าเสร็จแล้ว (Definition of Done & Evidence):**
1. Command: `go test -v ./modules/sre -run TestSREStore`
   - Output: `PASS`, exit code 0.
2. Concurrent stress test: 10 parallel goroutines writing samples while 10 read availability stats.
   - Output: zero `database is locked` errors; 100% successful executions.
3. Database inspection:
   - Output from `sqlite3 data.db ".schema downtime_incidents"` displays valid schema with WAL mode enabled (`PRAGMA journal_mode = wal`).

---

## T002 — SRE Prober & Downtime State Machine
**Status:** `TODO`  
**Scope:**
- Implement `modules/sre/prober.go`: HTTP client probing `http://127.0.0.1:8080/health` every 3s with a 1.5s timeout.
- Implement `modules/sre/tracker.go`: State machine handling `UP`, `DOWN_PROVISIONAL`, `OUTAGE_CRITICAL`, and `RECOVERED`.
- Wire alert threshold:
  - If downtime < 15 seconds: record to DB silently (no Telegram alert).
  - If downtime $\ge$ 15 seconds: dispatch Telegram alert with target URL, start time, and diagnostic advice.
  - On recovery from critical outage: dispatch recovery Telegram notification with total downtime duration.

**Acceptance Criteria:**
- State machine correctly ignores micro-glitches (< 1 probe) if configured, and transitions reliably.
- Accurate millisecond duration calculation between failure timestamp and recovery timestamp.

**หลักฐานที่บอกว่าเสร็จแล้ว (Definition of Done & Evidence):**
1. Unit test: `go test -v ./modules/sre -run TestStateTracker`
   - Output: `PASS`, verifying that a 2-second downtime logs without firing the alert hook, while a 16-second simulated downtime invokes the mock Telegram alert hook exactly once.
2. Recovery test: `go test -v ./modules/sre -run TestRecoveryNotification`
   - Output: `PASS`, verifying recovery hook called with formatted duration string (e.g. `16.4 วินาที`).

---

## T003 — Sentinel Daemon Binary (`cmd/sentinel`)
**Status:** `TODO`  
**Scope:**
- Create `cmd/sentinel/main.go` as the standalone entry point.
- Integrate `setupAndroidDNS()` for Termux DNS resolution.
- Initialize Telegram Bot Poller, Telemetry Collector Worker, and SRE Prober.
- Integrate Alert State Machine for host thresholds (battery, disk) and application downtime.

**Acceptance Criteria:**
- Sentinel Daemon runs completely headless (no public HTTP port listening).
- Daemon starts cleanly, polls Telegram, collects telemetry, and probes port 8080.

**หลักฐานที่บอกว่าเสร็จแล้ว (Definition of Done & Evidence):**
1. Cross-compilation:
   - Command: `CGO_ENABLED=0 GOOS=android GOARCH=arm64 go build -ldflags="-s -w" -o sentinel ./cmd/sentinel`
   - Output: Exit code 0, binary size $\le$ 22MB, `file sentinel` outputs `ELF 64-bit LSB pie executable, ARM aarch64`.
2. Execution test:
   - Start daemon locally: `./sentinel`
   - Log output verifies:
     `[sentinel] 🛡️ SRE Health Prober started (target: http://127.0.0.1:8080/health)`
     `[sentinel] 🤖 Telegram Bot Poller online`
     `[sentinel] 📊 Telemetry Collector running (interval: 60s)`

---

## T004 — Web App Binary Refactoring (`cmd/server`)
**Status:** `TODO`  
**Scope:**
- Create `cmd/server/main.go` (or clean up root `main.go`).
- Retain all web features: Dashboard, Relay (`/tele-bridge`, `/api/relay`), Reminders (`/schedule`), Digest, and Logs.
- Register `GET /health` (prober target returning HTTP 200 JSON `{ "status": "ok" }`).
- Register `GET /api/sre/status` returning current operational state, 24h uptime %, and last incident.
- Remove duplicate telemetry collector and bot poller loops from this binary.

**Acceptance Criteria:**
- Web App binds to `:8080` (or `PORT` env var).
- Passes existing test suite `main_test.go` and module tests.

**หลักฐานที่บอกว่าเสร็จแล้ว (Definition of Done & Evidence):**
1. Unit test suite: `go test -v ./...`
   - Output: 100% `PASS`.
2. Endpoint check:
   - Command: `curl -i http://localhost:8080/health`
   - Output: `HTTP/1.1 200 OK`, `{"status":"ok","uptime":...}`.
3. SRE endpoint check:
   - Command: `curl -i http://localhost:8080/api/sre/status`
   - Output: `HTTP/1.1 200 OK`, JSON containing `availability_pct`, `current_state`, `last_incident`.

---

## T005 — Telegram Bot Router Update & Emergency Controls
**Status:** `TODO`  
**Scope:**
- Update `modules/bot/router.go`:
  - `/reboot`: executes `pm2 restart go-server` to restart only the web app.
  - `/reboot sentinel`: restarts the daemon itself.
  - `/status`: reports both Host metrics and Web App health status (retrieved from SRE prober state).
  - `/exec`: allows safe execution of `pm2 logs go-server --lines 20 --nostream` during downtime.

**Acceptance Criteria:**
- Bot responds to commands even when `go-server` is stopped or crashing.
- Single-user authorization gate (`auth.go`) remains strictly enforced.

**หลักฐานที่บอกว่าเสร็จแล้ว (Definition of Done & Evidence):**
1. Unit test: `go test -v ./modules/bot -run TestRouterReboot`
   - Output: `PASS`.
2. Offline bot test:
   - Command sequence on v11: `pm2 stop go-server` then send `/ping` via Telegram chat.
   - Output: Bot returns `🏓 Pong! (App: Offline, Sentinel: Online)` within 1 second.

---

## T006 — Dashboard SRE Card & Live Telemetry UI
**Status:** `TODO`  
**Scope:**
- Add an Apple HIG SRE Observability Card to `modules/dashboard/dashboard.html`.
- Display live status badge (🟢 Operational / 🟡 Deploying / 🔴 Outage), 24h Uptime %, and last redeploy downtime.
- Implement `static/js/components/sre.js` with Alpine.js (strictly outside reactive proxies per AGENTS.md rules).
- Bump Service Worker cache version in `sw.js` (e.g. `sentinel-shell-v4`).

**Acceptance Criteria:**
- Card updates every 10 seconds without page refresh.
- Zero Alpine.js proxy `TypeError` in browser console.
- Responsive across mobile and desktop.

**หลักฐานที่บอกว่าเสร็จแล้ว (Definition of Done & Evidence):**
1. Console log check: Zero `TypeError: 'get' on proxy` errors when inspecting browser DevTools console.
2. Cache invalidation check:
   - Command: `curl -s http://localhost:8080/sw.js | grep CACHE_NAME`
   - Output: Matches updated version string.
3. UI verification:
   - Status badge and uptime percentage render with Apple action tokens.

---

## T007 — PM2 Orchestration & Deploy Pipeline Update
**Status:** `TODO`  
**Scope:**
- Update `ecosystem.config.js` to manage three processes: `go-server`, `sentinel-daemon`, and `ngrok-tunnel`.
- Update `scripts/deploy.sh`:
  - Cross-compile both `server` and `sentinel` binaries.
  - Default action: batch scp, atomic move, and `pm2 restart go-server` (Sentinel remains running).
  - Add `--all` flag to reload both services.

**Acceptance Criteria:**
- `./scripts/deploy.sh` deploys in under 20 seconds.
- Normal deployment does not interrupt `sentinel-daemon`.

**หลักฐานที่บอกว่าเสร็จแล้ว (Definition of Done & Evidence):**
1. Deploy execution:
   - Command: `./scripts/deploy.sh`
   - Output log shows:
     `[3/4] Reloading PM2 services on v11...`
     `pm2 restart go-server (sentinel-daemon left online)`
     `✅ Service Health Check: PASS (HTTP 200)`
2. Process uptime check:
   - Command: `ssh v11 "pm2 ls"`
   - Output shows `sentinel-daemon` uptime remained unchanged, while `go-server` uptime reset to `0s`.

---

## T008 — End-to-End SRE Verification & Chaos Testing on Device
**Status:** `TODO`  
**Scope:**
- Execute end-to-end chaos testing on vivo V11.
- Validate RAM budget, zero-spam deploy, critical alert dispatch, and recovery.

**Acceptance Criteria:**
- All 4 chaos scenarios pass with concrete logs and timestamps.

**หลักฐานที่บอกว่าเสร็จแล้ว (Definition of Done & Evidence):**
1. **Memory Budget Evidence:**
   - Command: `ssh v11 "pm2 jlist | jq '.[] | {name: .name, memory_mb: (.monit.memory / 1024 / 1024)}'"`
   - Output: `go-server`: $\le$ 18MB, `sentinel-daemon`: $\le$ 12MB. Combined RAM < 30MB (well below 300MB budget).
2. **Redeploy Silence & Metric Evidence:**
   - Command: `./scripts/deploy.sh`
   - Telegram chat: Zero spam messages received.
   - Database query:
     `ssh v11 "sqlite3 /data/data/com.termux/files/home/go-app/data.db 'SELECT started_at, duration_ms, status FROM downtime_incidents ORDER BY id DESC LIMIT 1;'"`
     Output: `2026-11-XX ... | 1850 | recovered`.
3. **Outage Alert Evidence:**
   - Command: `ssh v11 "pm2 stop go-server"`
   - Telegram chat: At T+15s receives `🚨 CRITICAL SRE ALERT: Web App is DOWN! (15s+)`.
4. **Recovery Notification Evidence:**
   - Command: `ssh v11 "pm2 start go-server"`
   - Telegram chat: Receives `✅ Web App RECOVERED! Downtime: XX.X วินาที`.
