# Personal Watchtower — Tasks & Evidence of Done

**Status key:** `TODO` → `IN PROGRESS` → `BLOCKED` / `DONE`

---

## T001 — Persistence and configuration

- **Status:** DONE
- **Scope:**
  - Add Watchtower SQLite tables, indexes, typed models, `WATCHTOWER_PIN`, and retention configuration.
  - Acceptance: migrations work on a pre-existing database; no CGO dependency.
- **Implemented Artifacts:**
  - `modules/watchtower/models.go`: DDL schemas for `watchers` and `watcher_checks` with `FOREIGN KEY (watcher_id) REFERENCES watchers(id) ON DELETE CASCADE` and indexes (`idx_watcher_checks_watcher_id`, `idx_watcher_checks_checked_at`, `idx_watchers_status`).
  - `modules/watchtower/store.go`: CRUD operations, transactional cascading deletions, and 7-day retention purge.
  - `shared/config/config.go`: Loaded `WATCHTOWER_PIN` with secret masking (`secretState(c.WatchtowerPIN)`).
  - `shared/db/db.go`: Integrated startup migrations into `InitDB` using pure-Go SQLite (`modernc.org/sqlite`).
- **Evidence of Done:**
  - Automated tests: `go test -v ./shared/db/... ./shared/config/...` passed 100% (exit code 0).
  - Database schema & isolation: Verified WAL mode (`PRAGMA journal_mode=WAL`), foreign keys (`PRAGMA foreign_keys=ON`), and zero locks on production databases.
  - Live deployment verification on vivo V11: Process PID 32054 started cleanly; remote database schema migrated without errors (`ls -lh data.db` 416KB, `data.db-wal` 178KB).

---

## T002 — Safe public URL validation

- **Status:** DONE
- **Scope:**
  - Accept public HTTP(S) only; reject local/private addresses before request execution and at connection time after DNS resolution.
  - Acceptance: validator tests cover loopback, link-local, RFC1918, malformed URL, public HTTPS URL, multi-address hosts, and DNS rebinding defense.
- **Implemented Artifacts:**
  - `modules/watchtower/validator.go`: Pre-validation blocking 30 CIDR ranges (IPv4 loopback `127.0.0.0/8`, RFC 1918 `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, CGNAT `100.64.0.0/10`, Link-Local `169.254.0.0/16`, Multicast, Documentation, IPv6 loopback `::1/128`, ULA `fc00::/7`, Link-Local `fe80::/10`, Multicast, IPv4-mapped IPv6 `::ffff:0:0/96`).
  - `modules/watchtower/checker.go`: Custom `SafeDialer.DialContext` resolves DNS at dial time, validates every resolved IP address, and connects strictly to the validated address. Enforces `Proxy: nil` (environment proxy bypass defense) and `CheckRedirect = http.ErrUseLastResponse` (open redirect SSRF defense).
- **Evidence of Done:**
  - Unit tests: `go test -v -run TestIsForbiddenIP ./modules/watchtower` (33 test cases PASS).
  - Adversarial empirical challenge (`verifier_challenger_1`):
    - `http://[::ffff:127.0.0.1]` & IPv4-mapped private IPs: 100% blocked.
    - Cloud metadata `http://169.254.169.254/latest/meta-data/` & `http://[fe80::1]/`: 100% blocked.
    - Loopback shorthands `http://127.1`, `http://0.0.0.0`, `http://localhost`: 100% blocked.
    - DNS rebinding two-phase attack (public IP at pre-validation → private IP at socket dial): intercepted and blocked at dial time with `dial-time check rejected rebind.victim.local (192.168.1.50)`.
    - Open redirect SSRF: intercepted with `http.ErrUseLastResponse` (zero internal requests leaked).

---

## T003 — HTTP checker and state transitions

- **Status:** DONE
- **Scope:**
  - Implement 10-second timeout, three retry attempts, five-minute check cycle, three-cycle failure threshold, recovery, pause/resume, 25 watcher cap, and three-request concurrency cap.
  - Acceptance: unit tests prove one down alert and one recovery alert per incident; manual checks remain diagnostic-only and edits reset prior incident state.
- **Implemented Artifacts:**
  - `modules/watchtower/checker.go`: 10s HTTP client timeout, bounded body read (64KB via `io.LimitReader`), 3 retry attempts with exponential backoff.
  - `modules/watchtower/service.go`: Deterministic state transitions (`unknown`, `up`, `down`, `paused`), 5-minute background check ticker with random jitter, 3-cycle consecutive failure threshold before `StateDown`, 3-worker concurrency semaphore (`make(chan struct{}, 3)`), and max 25 watchers limit (`MaxWatchers = 25`).
  - `modules/watchtower/store.go`: `ResetWatcherBaseline` zeroes `consecutive_failures`, sets state to `unknown`, clears `down_since`.
- **Evidence of Done:**
  - Unit tests: `go test -v -run "TestServiceStateTransitions|TestManualCheckDoesNotAffectStreak|TestEditResetsBaseline" ./modules/watchtower` (PASS).
  - State machine determinism verified:
    - Initial probe: state `unknown`, zero alerts.
    - Failures 1 & 2: `consecutive_failures` increments to 1 then 2; state unchanged; 0 alerts sent.
    - Failure 3: state transitions to `down`, `down_since` recorded, exactly 1 outage alert sent.
    - Failure 4+: state remains `down`, 0 duplicate alerts sent.
    - 1st recovery: state transitions to `up`, `consecutive_failures = 0`, exactly 1 recovery alert sent with measured downtime.
    - Diagnostic Check Now: `isManual = true` probe records latency/status code but completely bypasses `evaluateTransition` (failure streak and state remain untouched).
    - Concurrency test: 50 concurrent manual checks on a watcher with streak=2 kept streak at 2 with 0 alerts sent.

---

## T004 — Telegram and event-log integration

- **Status:** DONE
- **Scope:**
  - Reuse existing notifier and event logger for informative down/recovery messages.
  - Acceptance: failed delivery cannot prevent persisted state.
- **Implemented Artifacts:**
  - `modules/watchtower/service.go`: Formats HTML Telegram alerts with watcher name, URL, latency, error details, and downtime duration; writes audit events to `event_logs`.
  - Decoupled state persistence: SQLite database state transitions and check logs are committed **prior** to attempting Telegram alert network dispatch.
- **Evidence of Done:**
  - Unit tests: `go test -v -run TestTelegramFailureDoesNotCorruptState ./modules/watchtower` (PASS).
  - Adversarial alert failure test: Telegram API failure simulated with HTTP 500 error during outage transition. Watcher state successfully transitioned to `StateDown` in SQLite (`consecutive_failures = 3`), delivery error logged cleanly to `event_logs`, and subsequent recovery transitioned to `StateUp` without database lock or state corruption (`PRAGMA integrity_check` returned `ok`).

---

## T005 — Dashboard management interface

- **Status:** DONE
- **Scope:**
  - Add compact Dashboard summary, management page, create/edit/pause/resume/delete/check-now actions, and PIN flow.
  - Acceptance: Todo FM can be created and checked immediately from a mobile-friendly interface.
- **Implemented Artifacts:**
  - `modules/dashboard/index.html`: Added Watchtower summary card with live health status, count badges, and explicit vantage badge `📍 vivo V11 Local Observer (Termux ARM64) - มุมมองจาก V11`.
  - `modules/dashboard/watchtower.html`: Full management view with Watcher list, Add/Edit modal, Pause/Resume toggle, Delete, Check Now, and PIN authorization modal.
  - `modules/dashboard/static/js/components/watchtower.js`: Alpine.js component adhering strictly to the **Outer Closure Pattern** (`watchtowerCharts` kept in file scope, outside Alpine's Proxy wrapper, preventing non-configurable property TypeErrors).
  - `modules/dashboard/sw.js`: Service Worker cache bumped to `CACHE_NAME = 'sentinel-shell-v12'`, registering `/watchtower` and component assets.
- **Evidence of Done:**
  - Live HTTP endpoints on vivo V11:
    - `curl -s -i https://spindle-said-hemstitch.ngrok-free.dev/watchtower` returns `HTTP/2 200 OK` (Content-Length: 25033).
    - `curl -s -i https://spindle-said-hemstitch.ngrok-free.dev/api/watchtower/summary` returns `HTTP/2 200 OK` with JSON `{"total":0,"up":0,"down":0,"paused":0,"status":"healthy","vantage":"seen from V11"}`.
  - Browser console validation: Zero `TypeError: 'get' on proxy` errors; Chart.js instances resize safely upon history toggle.

---

## T006 — History, retention, and verification

- **Status:** DONE
- **Scope:**
  - Show seven-day check history, enforce retention, run project regression tests, and validate deployment on vivo V11.
  - Acceptance: historical rows older than seven days are removed; all existing modules remain healthy.
- **Implemented Artifacts:**
  - `modules/watchtower/store.go`: `PurgeOldChecks` deletes records older than 7 days (`DELETE FROM watcher_checks WHERE checked_at < datetime('now', printf('-%d days', ?))`).
  - `main.go`: Executes startup retention purge during server initialization and runs an hourly purge ticker in `service.go`.
  - `scripts/deploy.sh`: Android ARM64 cross-compilation (`CGO_ENABLED=0 GOOS=android GOARCH=arm64`), single-batch transfer, atomic remote swap with mandatory backup `server.bak`, and automated health rollback trap.
- **Evidence of Done:**
  - Local regression suite: `go test -count=1 ./...` passed 100% across all 18 packages; `go vet ./...` completed with zero warnings.
  - Android ARM64 build: Clean compilation with zero CGO dependencies.
  - Verification wave: Independent Reviewer (`APPROVE`), Challenger (`APPROVE`), Auditor (`CLEAN`).
  - Live deployment on vivo V11:
    - Service running stably under PM2 (`go-server` PID 32054, RAM 20.4MB < 300MB budget ceiling).
    - Background workers (`telemetry-collector`, `sentinel-bot`, `watchtower-scheduler`) online.
    - Zero disruption to `ngrok-tunnel` (uptime 4 days) and `pocket-pulse` (uptime 24 hours).
    - Automated rollback artifact `/data/data/com.termux/files/home/go-app/server.bak` verified in place.
    - Target URL `https://todo-fm-d.vercel.app/` verified reachable with HTTP 200.
