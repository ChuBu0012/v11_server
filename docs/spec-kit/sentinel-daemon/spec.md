# Pocket Sentinel — Decoupled SRE Sentinel Daemon Specification

**Status:** Draft for approval  
**Product name:** Decoupled SRE Sentinel (Sentinel Daemon + Independent Web App)  
**Target Environment:** vivo 1804 (V11) Android 10 (Termux ARM64)  
**Ingress:** Single ngrok tunnel (`http://localhost:8080`)  

---

## 1. Intent & Problem Statement

Currently, Pocket Sentinel runs as a single monolithic Go binary (`server`) managed by PM2 on port 8080. It acts as both the **Observer** (telemetry collector, health monitors, Telegram bot, alert engine) and the **Observed** (web dashboard, photo/file relay API, reminder calendar, AI digest).

In SRE (Site Reliability Engineering), this violates the fundamental principle of **Failure Domain Isolation (Blast Radius Separation)**:
- Whenever the application is redeployed or encounters a runtime panic/crash, the monitor and Telegram bot crash along with it.
- Downtime during deployment cannot be measured, logged, or observed from within the system.
- Users receive no diagnostic insight or emergency bot access during an outage.

**Decoupled SRE Sentinel** splits the system into two lightweight, independent processes that share a local SQLite database in WAL mode:
1. **Sentinel Daemon (`sentinel-daemon`):** An always-on, autonomous watchdog that runs the Telegram Bot, collects device telemetry, probes the Web App's health endpoint every 3 seconds, and tracks downtime duration.
2. **Web App (`go-server`):** The user-facing Gin HTTP server on port 8080 hosting the Dashboard, Relay, Reminders, and APIs, directly exposed through the single public ngrok tunnel.

---

## 2. Goals & Non-goals

### Goals
- **Zero Fate-Sharing:** Redeploying or crashing `go-server` must NOT disrupt `sentinel-daemon` or the Telegram Bot.
- **Accurate Downtime Measurement:** Sentinel probes `http://localhost:8080/health` every 3 seconds and records state transitions (UP → DOWN → UP) with duration in milliseconds in `downtime_incidents`.
- **Intelligent Alert Policy:**
  - Expected deployments (< 15s downtime): Recorded silently into SQLite to eliminate chat spam.
  - Critical outages (> 15s downtime): Immediate 🚨 Critical Telegram alert with diagnostics.
- **Always-Available Emergency Bot:** Telegram commands (`/status`, `/exec`, `/ping`, `/torch`, `/task`, `/reboot`) stay 100% operational during web deployments.
- **Controlled Reboots:** `/reboot` restarts `go-server` by default; `/reboot sentinel` restarts the daemon.
- **Dashboard SRE Observability:** Dashboard displays a live SRE Status Card showing 24h Availability %, current status, and last redeploy downtime duration.
- **Strict Memory Budget:** Both Go processes combined must consume < 30MB RAM on vivo V11 (budget ceiling: 300MB).
- **Single Public Tunnel Compliance:** ngrok continues pointing exclusively to `http://localhost:8080` without any reverse proxy overhead.

### Non-goals
- Multi-region or distributed consensus monitoring.
- High-frequency sub-second HTTP probing that drains battery or creates excessive CPU wake locks.
- Public web UI for the Sentinel Daemon itself (all Sentinel administrative interaction occurs via Telegram).
- Complex blue/green socket inheritance (`SO_REUSEPORT`) which is unstable on Android Kernel 4.4.

---

## 3. User Experience & Scenarios

### Scenario A: Normal Redeployment
1. Developer runs `./scripts/deploy.sh` on macOS.
2. PM2 restarts `go-server`. `sentinel-daemon` remains online.
3. Sentinel notices `http://localhost:8080/health` is down at `T+0s`.
4. `go-server` comes back up at `T+2.1s`.
5. Sentinel logs the incident: `duration_ms=2100`, `reason="redeploy_restart"`.
6. No spam message is sent to Telegram because downtime was < 15s.
7. Next time the user opens the Dashboard, the SRE card shows: `Last redeploy: 2.1s ago (Downtime: 2.1s)`.

### Scenario B: Deployment Failure or Application Panic
1. A new build contains a configuration error or panics on startup.
2. `go-server` crashes and PM2 restarts it repeatedly, failing the health check.
3. At `T+15s`, Sentinel detects that downtime exceeded the 15-second threshold.
4. Sentinel sends an urgent alert to Telegram:
   ```html
   🚨 <b>CRITICAL SRE ALERT: Web App is DOWN!</b>
   ⏱️ <b>Downtime:</b> 15+ วินาที
   🔍 <b>Target:</b> <code>http://localhost:8080/health</code>
   💡 <i>Sentinel ยังออนไลน์ พิมพ์ /exec pm2 logs go-server หรือ /reboot เพื่อแก้ไข</i>
   ```
5. The user types `/exec pm2 logs go-server --lines 15 --nostream` in Telegram and immediately inspects the crash log from their phone.

---

## 4. Architecture & Component Contracts

```text
┌─────────────────────────────────────────────────────────────┐
│                      ngrok Ingress                          │
│        https://spindle-said-hemstitch.ngrok-free.dev        │
└──────────────────────────────┬──────────────────────────────┘
                               │
                               ▼
┌─────────────────────────────────────────────────────────────┐
│  Process 1: go-server (Port 8080)                           │
│  - Gin HTTP Framework (ReleaseMode)                         │
│  - GET /               (Embedded Dashboard Web UI)          │
│  - GET /health         (Prober target, returns HTTP 200)    │
│  - GET /api/sre/status (Uptime %, recent incidents)         │
│  - Modules: Dashboard, Relay, Reminders, Digest, Logs       │
└──────────────────────────────┬──────────────────────────────┘
                               ▲
                               │ HTTP Probe every 3s
                               │ (HealthProber)
┌──────────────────────────────┴──────────────────────────────┐
│  Process 2: sentinel-daemon                                 │
│  - Background Daemon (No public HTTP listener)              │
│  - SRE HealthProber: Checks localhost:8080/health          │
│  - DowntimeTracker: Measures duration, records incidents    │
│  - Telegram Bot Poller: Long Polling 24/7                   │
│  - Telemetry Collector: Battery, Disk, RAM, Uptime          │
│  - Alert State Machine: Battery/Disk thresholds + Outage   │
└──────────────────────────────┬──────────────────────────────┘
                               │
                               ▼
┌─────────────────────────────────────────────────────────────┐
│  Shared Storage: SQLite WAL Mode (data.db)                  │
│  - health_samples (Sentinel writes, Dashboard reads)        │
│  - downtime_incidents (Sentinel writes, Dashboard reads)    │
│  - event_logs, reminders, digest, alert_states              │
└─────────────────────────────────────────────────────────────┘
```

---

## 5. Data Models & SQLite Schema

### Table: `downtime_incidents`
```sql
CREATE TABLE IF NOT EXISTS downtime_incidents (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    started_at DATETIME NOT NULL,
    recovered_at DATETIME,
    duration_ms INTEGER,
    status TEXT NOT NULL,         -- 'active', 'recovered', 'degraded'
    reason TEXT,                  -- 'http_unreachable', 'http_5xx', 'redeploy'
    alert_sent BOOLEAN DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_downtime_started_at ON downtime_incidents(started_at DESC);
```

### Table: `health_samples` (Existing, written by Sentinel)
Contains host metrics (battery, memory, disk). `go-server` queries this table read-only for Dashboard telemetry charts.

---

## 6. SRE Metrics & Calculations

1. **Uptime / Availability % (Last 24 Hours):**
   $$\text{Availability} = \frac{86,400,000 - \sum \text{Downtime (ms)}}{86,400,000} \times 100\%$$
2. **Current State:**
   - `OPERATIONAL` (Last probe = 200 OK)
   - `REDEPLOYING` (Down for < 15 seconds)
   - `CRITICAL_OUTAGE` (Down for $\ge$ 15 seconds)
3. **Probe Schedule:**
   - Interval: 3 seconds.
   - Timeout: 1.5 seconds.
   - Target: `http://127.0.0.1:8080/health`.

---

## 7. Security & Environmental Constraints

- **Pure Go Only:** `modernc.org/sqlite` without CGO; no gcc/clang runtime on Termux.
- **DNS Resolution:** Must preserve `setupAndroidDNS()` on Android Termux to resolve `api.telegram.org`.
- **Concurrency Safety:** SQLite opened with `PRAGMA journal_mode=WAL;` and `PRAGMA busy_timeout=5000;` in both binaries.
- **Process Memory Limits:**
  - `go-server`: `max_memory_restart: "200M"` in PM2 (typically ~14MB).
  - `sentinel-daemon`: `max_memory_restart: "100M"` in PM2 (typically ~8MB).

---

## 8. Acceptance Criteria

1. **Isolation Proof:** Killing `go-server` with `pm2 stop go-server` leaves `sentinel-daemon` fully running. Sending `/ping` to Telegram returns `Pong` within 1 second.
2. **Accurate Downtime Tracking:** Restarting `go-server` records an incident in `downtime_incidents` with `duration_ms` between 500ms and 4000ms.
3. **Threshold-Gated Alerting:**
   - Normal restart (< 15s): Zero alert messages in Telegram.
   - Simulated outage (stop > 15s): Exactly 1 Critical Alert sent at T+15s. When restarted, 1 Recovery message sent.
4. **Dashboard Integration:** Dashboard renders the SRE Card with live availability %, last downtime duration, and status indicator.
5. **Clean PM2 Operation:** `pm2 ls` displays both `go-server` and `sentinel-daemon` running under 30MB combined RAM.
6. **Backward Compatibility:** All existing features (Relay `/tele-bridge`, Reminders `/schedule`, Digest `/digest`, Agent `/task`) remain 100% operational.
