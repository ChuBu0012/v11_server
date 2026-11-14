# Pocket Sentinel — Product Specification

**Status:** Draft for implementation  
**Version:** 1.0  
**Target:** vivo V11 / Android 10 / Termux

## Summary

Turn the existing pocket server into a lightweight personal monitoring service. It collects device and service health, stores a history locally, presents that history in a small web dashboard, and sends Telegram alerts when configured thresholds are crossed. The phone acts as the always-on collector and relay; optional AI-assisted summaries are a later extension and are not required for the first release.

## Problem

The server already exposes current health, runtime, disk, battery, and event-log endpoints, but it does not retain time-series health readings, make them easy to inspect, or notify the owner when a problem occurs. The owner needs a low-maintenance way to see whether the phone-hosted service is healthy and get timely alerts without running a heavy stack on Android.

## Goals

- Provide a compact dashboard showing current server/device status and recent history.
- Collect health snapshots on a configurable interval and retain a bounded history in SQLite.
- Send Telegram alerts for actionable conditions: service health failure, low battery, and low free disk space.
- Avoid repeated alerts for the same ongoing condition; send a recovery notification when it clears.
- Keep the Go service suitable for Termux and below the existing 300 MB per-process memory limit.
- Make setup possible through environment configuration without committing secrets.

## Non-goals for the first release

- Running an AI model on the phone or making an external AI provider mandatory.
- General purpose home automation or disruptive hardware control.
- Multi-user accounts, a public SaaS, or a full observability stack.
- Replacing PM2, SQLite, or the existing Cloudflare Quick Tunnel deployment.
- Sending notifications through LINE or Discord in the first iteration. Telegram is the initial channel behind a notifier interface so more channels can be added later.

## Users and primary flow

The primary user is the owner of the V11. They open the private dashboard to check the server, battery, disk, and recent alerts. The collector checks configured thresholds in the background; on a state transition it records an event and sends a Telegram message. After recovery, it records and sends a recovery message.

## Functional requirements

### Dashboard

- `GET /` serves a responsive, static dashboard suitable for phone and desktop browsers.
- Dashboard shows last successful collection time, service uptime, Go runtime memory, disk free/total, battery state when Termux API is available, and current alert conditions.
- Dashboard shows recent health history and recent event logs, with clear empty, loading, and unavailable states.
- Dashboard fetches data from the service API; it does not require a frontend build system or a database connection from the browser.

### Collection and history

- A background collector samples local system/device status at a configurable interval (default: 60 seconds).
- Each sample stores a UTC timestamp and the supported measurements in SQLite.
- Battery collection failure is represented as unavailable and does not stop other measurements.
- Retention is bounded by configurable age and/or row count, with safe defaults suitable for a small SQLite file.
- Collection starts and stops with the Go process and shuts down cleanly.

### Alerts

- Telegram integration is enabled only when bot token and chat ID are configured.
- Configurable thresholds include low battery percentage and low disk free space.
- Health-check failure is detected by the collector itself; it must not recursively depend on calling the public tunnel.
- Alert state is edge-triggered: send once when entering a problem state, suppress repeats while it persists, and send one recovery message when it clears.
- Alert attempts and outcomes are written to event logs without recording bot tokens or other secrets.
- Telegram API timeouts and errors do not block sampling or HTTP request handling; failed delivery is logged and retried on a later collection only when policy permits.

### API and configuration

- Keep existing routes compatible: `/health`, `/api/system`, `/api/device/battery`, `/api/logs`.
- Add a read-only health-history API with bounded query parameters and a bounded response size.
- Add a current alert-state API for the dashboard.
- Configuration comes from environment variables with documented defaults. Invalid values fail clearly at startup or fall back only where safe and explicitly documented.
- Secrets are supplied at runtime and excluded from source control, API responses, logs, and dashboard assets.

## Quality attributes and constraints

- Runtime remains a standalone pure-Go binary compatible with the repository's Android ARM64 deployment flow.
- Keep idle resource use low; no external database, container, or heavyweight frontend runtime.
- SQLite writes must tolerate concurrent API and collector access using the existing database setup and bounded connection pool.
- Use request and outbound-call timeouts, bounded history queries, and graceful shutdown.
- Public tunnel exposure is a security boundary: dashboard/history are read-only; log creation and other state-changing routes must not be exposed without a configured access control mechanism. Decide and document the access policy before enabling public write routes.
- Never include Telegram credentials in logs or client-visible data.

## Acceptance criteria

1. A deployed-compatible build serves the dashboard and existing endpoints.
2. At least two collector samples can be queried in chronological order, and retention removes expired/excess samples.
3. Threshold transitions produce one Telegram alert and one recovery alert, with repeated samples during the same state producing no duplicate alert.
4. Missing Termux battery support or Telegram configuration does not crash the service or prevent other features.
5. Automated tests cover storage, threshold state transitions, API bounds, and dashboard/API integration.
6. Implementation evidence is recorded for every task in `tasks.md`; final verification includes the actual command and result, not a planned command.

## Decisions and open items

- Initial notifier: Telegram Bot API.
- Initial dashboard: server-rendered/static assets shipped with the Go binary or embedded in the repository build; avoid runtime Node dependencies.
- Default collection interval: 60 seconds; make configurable.
- Dashboard access policy and whether an auth token is needed for read access remain a deployment decision. Write routes must be protected before public exposure.
- AI summaries, LINE, and Discord are follow-on capabilities after the monitoring MVP is stable.

## Current project context

The service currently uses Gin and `modernc.org/sqlite`; routes live under `modules/system` and `modules/logs`, database initialization is in `shared/db`, and PM2 deploy configuration targets port 8080 and `data.db`. Reconcile the existing `main_test.go` with the current modular code as part of implementation planning; it appears to reference legacy type/function names.
