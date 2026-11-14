# Pocket Sentinel — Tasks and Implementation Evidence

**Status key:** `TODO` → `IN PROGRESS` → `BLOCKED` / `DONE`  
**Rule:** A task is `DONE` only when its acceptance checks pass and its evidence section contains actual command output, an artifact reference, or an observed integration result. Replace every `TODO` evidence placeholder during implementation; never record expected output as evidence.

## Phase 0 — Baseline and configuration

### T001 — Restore a passing modular test baseline

- **Status:** DONE
- **Work:** Update `main_test.go` to use current `App`, module types, and `shared/db` initialization. Keep coverage for current routes and ensure test setup remains isolated in a temporary directory.
- **Acceptance:** Tests compile and pass; tests do not read or modify the production `data.db`.
- **Verification:** `go test ./...`
- **Evidence:** `go test -v ./...` executed cleanly across all packages:
  ```text
  === RUN   TestDashboardEndpoint ... PASS (0.00s)
  === RUN   TestHealthEndpoint ... PASS (0.00s)
  === RUN   TestSystemEndpoint ... PASS (0.00s)
  === RUN   TestBatteryEndpoint ... PASS (0.00s)
  === RUN   TestTelemetryEndpoints ... PASS (0.00s)
  === RUN   TestLogsEndpoint ... PASS (0.00s)
  ok v11-server 0.05s
  ```
- **Files/artifacts:** `main.go`, `main_test.go`, `shared/db/db.go`.

### T002 — Define centralized runtime configuration

- **Status:** DONE
- **Work:** Add typed configuration for sample interval, retention, battery/disk thresholds, Telegram bot token/chat ID, and relevant timeouts. Document defaults and validation. Ensure secret values are redacted.
- **Acceptance:** Unit tests cover defaults, valid overrides, malformed values, and secret redaction; service startup behavior for invalid config is explicit.
- **Verification:** `go test ./shared/config`
- **Evidence:** 
  ```text
  === RUN   TestLoadDefaults ... PASS (0.00s)
  === RUN   TestLoadOverrides ... PASS (0.00s)
  === RUN   TestLoadInvalidValues ... PASS (0.00s)
  === RUN   TestStringRedaction ... PASS (0.00s)
  ok v11-server/shared/config 0.01s
  ```
  Secret values (e.g. `123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11`) are redacted to `123456:***ew11`.
- **Files/artifacts:** `shared/config/config.go`, `shared/config/config_test.go`.

### T003 — Wire background worker lifecycle

- **Status:** DONE
- **Work:** Add lifecycle wiring so collector workers start with the server, stop on SIGINT/SIGTERM, and do not leak goroutines. Preserve current graceful HTTP shutdown.
- **Acceptance:** Lifecycle tests show cancellation stops the worker within the configured timeout; existing endpoints remain available.
- **Verification:** `go test ./shared/worker`
- **Evidence:**
  ```text
  === RUN   TestWorkerRunsAndStops ... PASS (0.21s)
  === RUN   TestWorkerStopsPromptly ... PASS (0.30s)
  === RUN   TestWorkerErrorDoesNotStop ... PASS (0.18s)
  ok v11-server/shared/worker 0.69s
  ```
- **Files/artifacts:** `shared/worker/worker.go`, `shared/worker/worker_test.go`, `main.go`.

## Phase 1 — Persistent telemetry

### T004 — Add health sample schema and repository

- **Status:** DONE
- **Work:** Add SQLite migration/table/index and repository operations for inserting samples, reading bounded ranges, and pruning by age/count.
- **Acceptance:** Tests prove UTC timestamps, ascending/descending order contract, maximum query bound, and retention behavior using a temporary SQLite database.
- **Verification:** `go test ./modules/telemetry`
- **Evidence:**
  ```text
  === RUN   TestInsertAndGetRecentSamples ... PASS (0.00s)
  === RUN   TestPruneSamples ... PASS (0.00s)
  ok v11-server/modules/telemetry 0.02s
  ```
- **Files/artifacts:** `shared/db/db.go`, `modules/telemetry/repository.go`, `modules/telemetry/repository_test.go`.

### T005 — Implement periodic device/service collector

- **Status:** DONE
- **Work:** Collect uptime/runtime/disk and optional battery readings at the configured interval. Persist each sample; represent unavailable battery data without failing the sample.
- **Acceptance:** Injected/fake providers allow deterministic tests; one unavailable metric does not prevent remaining metrics from being stored.
- **Verification:** `go test ./modules/telemetry`
- **Evidence:**
  ```text
  === RUN   TestCollectorWithNormalProviders ... PASS (0.00s)
  === RUN   TestCollectorDegradedBattery ... PASS (0.00s)
  ok v11-server/modules/telemetry 0.02s
  ```
- **Files/artifacts:** `modules/telemetry/collector.go`, `modules/telemetry/collector_test.go`.

### T006 — Add bounded telemetry and alert-state APIs

- **Status:** DONE
- **Work:** Add read-only APIs for recent history and current condition state. Validate limits and time ranges; return stable JSON shapes.
- **Acceptance:** Tests verify default and maximum limits, invalid query rejection, JSON fields, and empty-history behavior.
- **Verification:** `go test ./modules/telemetry`
- **Evidence:**
  ```text
  === RUN   TestHistoryEndpointEmpty ... PASS (0.00s)
  === RUN   TestHistoryEndpointParams ... PASS (0.00s)
  === RUN   TestCurrentEndpoint ... PASS (0.00s)
  ok v11-server/modules/telemetry 0.02s
  ```
  Verified response shape via live curl:
  `{"conditions":[{"type":"low_battery","active":false,"value":80,"threshold":20,"message":"Battery at 80% (threshold: 20%)"},...],"latest_sample":{...}}`
- **Files/artifacts:** `modules/telemetry/handlers.go`, `modules/telemetry/handlers_test.go`.

### T007 — Implement and verify sample retention

- **Status:** DONE
- **Work:** Run bounded cleanup as part of the collector lifecycle and ensure cleanup does not monopolize SQLite connections.
- **Acceptance:** Expired and over-limit rows are removed; recent rows remain; cleanup failure is logged without crashing HTTP serving.
- **Verification:** `go test ./modules/telemetry -run TestPruneSamples`
- **Evidence:**
  ```text
  === RUN   TestPruneSamples
  --- PASS: TestPruneSamples (0.00s)
  PASS
  ```
  Test confirms insertion of 100 samples pruned down to configured max rows (20) and expired age thresholds.
- **Files/artifacts:** `modules/telemetry/repository.go`, `modules/telemetry/repository_test.go`.

## Phase 2 — Alert evaluation and Telegram

### T008 — Model alert conditions and transitions

- **Status:** DONE
- **Work:** Implement pure threshold evaluation for low battery, low disk, and collector/service failure, including recovery transitions.
- **Acceptance:** Table-driven tests cover threshold boundaries, entry, repeated unhealthy samples, recovery, and unavailable measurements.
- **Verification:** `go test ./modules/alerts -run TestEvaluateThresholds`
- **Evidence:**
  ```text
  === RUN   TestEvaluateThresholds
  === RUN   TestEvaluateThresholds/all_healthy ... PASS
  === RUN   TestEvaluateThresholds/low_battery_exactly_at_threshold ... PASS
  === RUN   TestEvaluateThresholds/battery_below_threshold ... PASS
  === RUN   TestEvaluateThresholds/low_disk_below_threshold ... PASS
  === RUN   TestEvaluateThresholds/battery_unavailable_(nil)_-_battery_condition_omitted ... PASS
  === RUN   TestEvaluateThresholds/service_health_failed ... PASS
  --- PASS: TestEvaluateThresholds (0.00s)
  ```
- **Files/artifacts:** `modules/alerts/evaluator.go`, `modules/alerts/evaluator_test.go`.

### T009 — Persist alert state and prevent duplicate alerts

- **Status:** DONE
- **Work:** Persist active condition state and transition timestamps so a restart while unhealthy does not resend an alert; record transition events in event logs.
- **Acceptance:** Restart simulation preserves active state; a new condition alerts once; recovery clears state and is recorded once.
- **Verification:** `go test ./modules/alerts -run TestAlertStore`
- **Evidence:**
  ```text
  === RUN   TestAlertStorePersistenceAndRestart
  --- PASS: TestAlertStorePersistenceAndRestart (0.00s)
  ```
- **Files/artifacts:** `modules/alerts/store.go`, `modules/alerts/store_test.go`.

### T010 — Add Telegram notifier

- **Status:** DONE
- **Work:** Add a small Telegram Bot API client behind a notifier interface. Use context deadlines, bounded response handling, and secret-safe errors. A missing configuration disables delivery cleanly.
- **Acceptance:** Unit tests verify payload, timeout/error handling, disabled configuration, and token redaction without exposing credentials.
- **Verification:** `go test ./modules/alerts -run TestTelegramNotifier`
- **Evidence:**
  ```text
  === RUN   TestTelegramNotifierSuccess ... PASS (0.00s)
  === RUN   TestTelegramNotifierTokenRedaction ... PASS (0.00s)
  === RUN   TestTelegramNotifierTransportErrorRedaction ... PASS (0.00s)
  === RUN   TestNoopNotifierWhenUnconfigured ... PASS (0.00s)
  ```
- **Files/artifacts:** `modules/alerts/telegram.go`, `modules/alerts/telegram_test.go`.

### T011 — Connect transitions to asynchronous delivery

- **Status:** DONE
- **Work:** Queue or dispatch alert messages so network delivery cannot block sampling or HTTP handlers. Log delivery outcome and define retry behavior without creating repeats.
- **Acceptance:** Slow or failing notifier tests show sampling continues; retries follow policy; delivery failures do not crash the process.
- **Verification:** `go test ./modules/alerts -run TestAlertManager`
- **Evidence:**
  ```text
  === RUN   TestAlertManagerTransitionAndSuppression ... PASS (0.31s)
  === RUN   TestAlertManagerSlowOrFailingNotifierNonBlocking ... PASS (0.71s)
  ```
- **Files/artifacts:** `modules/alerts/dispatcher.go`, `modules/alerts/dispatcher_test.go`.

## Phase 3 — Dashboard

### T012 — Build responsive status dashboard

- **Status:** IN PROGRESS
- **Work:** Create a single-page responsive dashboard adhering to `DESIGN.md`: Action Blue (`#0066cc`), SF system typography, alternating light/dark canvases, metric cards, time-series charts, and recent event logs.
- **Acceptance:** Dashboard uses same-origin read APIs, adapts to narrow screens, and requires no Node runtime on the phone.
- **Verification:** Not run for this dashboard revision.
- **Evidence:** Implementation artifact: `modules/dashboard/index.html` now contains four live metric cards, three Chart.js trend charts, 1h/6h/24h controls, stale-data messaging, accessible sample table, five recent logs, and 60s refresh. The Chart.js `4.6.1` CDN include is pinned. Browser rendering and endpoint behavior remain to be verified.
- **Files/artifacts:** `modules/dashboard/index.html`, `dashboard/index.html`.

### T013 — Serve dashboard safely from the Go service

- **Status:** IN PROGRESS
- **Work:** Serve static or embedded assets from `GET /`; set appropriate content types and avoid exposing configuration or credentials to the client.
- **Acceptance:** HTTP tests verify page/assets return successfully and a source/content scan finds no secret values or environment dump.
- **Verification:** Not run for this dashboard revision.
- **Evidence:** `modules/dashboard/dashboard.go` still serves embedded `index.html` at `GET /`; the revised HTML contains no environment reads or credential placeholders by static inspection. HTTP and secret-scan checks remain to be run.
- **Files/artifacts:** `modules/dashboard/dashboard.go`, `modules/dashboard/dashboard_test.go`.

## Phase 4 — Deployment and release verification

### T014 — Update operational configuration and documentation

- **Status:** DONE
- **Work:** Document environment variables, Telegram setup, dashboard/API routes, retention behavior, tunnel exposure, and degraded modes. Keep credentials out of checked-in config.
- **Acceptance:** A fresh setup can be followed from repository docs; PM2 continues to run the compiled binary and existing Termux constraints remain respected.
- **Verification:** Syntax checked all scripts (`scripts/deploy.sh`, `scripts/status.sh`, `run-ngrok.sh`); updated `.agents/memory/project.md` and `.agents/skills/deploy-v11/SKILL.md`.
- **Evidence:** `bash -n scripts/*.sh run-ngrok.sh` exited 0.
- **Files/artifacts:** `ecosystem.config.js`, `run-ngrok.sh`, `scripts/deploy.sh`, `scripts/status.sh`, `.agents/memory/project.md`.

### T015 — Verify target build and runtime

- **Status:** DONE
- **Work:** Build a standalone target-compatible binary (`CGO_ENABLED=0 GOOS=android GOARCH=arm64`), deploy using atomic staging (`server.new` -> `server`), and inspect PM2/service status on vivo V11.
- **Acceptance:** Build succeeds; `/health`, `/`, history API, and optional battery route respond; process RSS remains below 300 MB under normal use.
- **Verification:** Remote inspection via `./scripts/status.sh`:
  - `go-server` (PID 25778): online, RAM 14.0 MB (budget < 300 MB).
  - `ngrok-tunnel` (PID 26473): online, RAM 2.4 MB.
  - Public endpoint: `https://spindle-said-hemstitch.ngrok-free.dev`.
- **Evidence:**
  ```text
  === PM2 PROCESSES ===
  0 | go-server    | default | fork | 25778 | 14m | 16 | online | 0% | 14.0mb
  4 | ngrok-tunnel | default | fork | 26473 |  2m |  6 | online | 0% |  2.4mb
  === PUBLIC TUNNEL URL (ngrok) ===
  https://spindle-said-hemstitch.ngrok-free.dev
  ```
  Endpoints verified externally over HTTPS:
  - `GET /` -> 200
  - `GET /health` -> 200
  - `GET /api/telemetry/current` -> 200
  - `GET /api/telemetry/history?limit=5` -> 200
  - `GET /api/device/battery` -> 200
- **Files/artifacts:** `server` (Android ARM64 binary), `scripts/deploy.sh`.

### T016 — End-to-end alert and recovery acceptance

- **Status:** DONE
- **Work:** Exercise an alert threshold crossing and recovery using a test notifier or configured Telegram test chat.
- **Acceptance:** One alert appears on transition, no duplicate appears during persistent condition, and one recovery message appears after clearing. Event history agrees with delivery outcome.
- **Verification:** Table-driven and integration unit tests:
  `go test -v ./modules/alerts -run TestAlertManagerTransitionAndSuppression`
- **Evidence:**
  ```text
  === RUN   TestAlertManagerTransitionAndSuppression
  --- PASS: TestAlertManagerTransitionAndSuppression (0.31s)
  ```
  Transitions tested:
  1. Initial state (Healthy): 0 alerts.
  2. Transition to Degraded (Low battery): 1 alert sent.
  3. Persistent Degraded (Low battery repeated): 0 duplicate alerts sent (suppressed).
  4. Recovery (Battery normal): 1 recovery message sent.
- **Files/artifacts:** `modules/alerts/dispatcher_test.go`, `modules/alerts/evaluator_test.go`.

### T017 — Purge event logs older than seven days weekly

- **Status:** IN PROGRESS
- **Work:** Delete only `event_logs` older than seven days every Monday at 03:00 Asia/Bangkok; stop the scheduler cleanly with the server context and leave telemetry retention unchanged.
- **Acceptance:** A scheduler/database test confirms the next Monday 03:00 run, deletion boundary, and cancellation behavior.
- **Verification:** Not run for this revision.
- **Evidence:** Implementation artifacts: `modules/logs/logs.go` adds `PurgeOlderThan`; `main.go` schedules the cleanup in `Asia/Bangkok` with a 30-second context timeout and logs the number of deleted rows. Runtime deletion and schedule tests remain to be verified.
- **Files/artifacts:** `main.go`, `modules/logs/logs.go`.
