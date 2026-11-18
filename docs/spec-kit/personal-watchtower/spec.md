# Pocket Sentinel — Personal Watchtower Specification

**Status:** Draft for approval  
**Target:** vivo V11 / Android 10 / Termux ARM64  
**Primary target:** `Todo FM` — `https://todo-fm-d.vercel.app/`

## Intent

Personal Watchtower continuously verifies that public services owned by the device owner remain reachable from the Pocket Sentinel server. It records evidence, alerts Telegram only after a sustained outage, and sends a recovery notice when the service returns.

The first release is deliberately narrow: public HTTP(S) availability checks. It must be reliable, inexpensive to run, and simple to manage from the Dashboard.

## User decisions

- Monitor public services first; internal/LAN targets are deferred to v2.
- Add and manage watchers through the Dashboard.
- Avoid false alerts: check every 5 minutes; use a 10-second timeout, up to 3 retries per check, and alert only after 3 failed check cycles.
- A paused watcher stays paused until explicitly resumed.
- Use a separate `WATCHTOWER_PIN` for create, edit, enable, pause, and delete actions. Read-only status remains visible.
- Retain detailed check history for 7 days.
- A new watcher is persisted, checked immediately, and enabled even if that first check fails.
- Telegram messages include watcher name, URL, reason, outage start or downtime, and a Dashboard link; recovery messages are sent.

## Goals

- Let the owner add a public URL in one Dashboard form: name, URL, optional expected HTTP status.
- Show one Watchtower summary in the main Dashboard plus a prominent link to a management page.
- Immediately check a target after creation and check enabled targets every 5 minutes thereafter.
- Track `up`, `down`, and `paused` state across process restarts.
- Send Telegram only for `up → down` and `down → up` transitions.
- Persist check history with timestamp, result, status code, latency, error summary, and failure streak.
- Keep baseline memory use small and use the existing SQLite, event log, background worker lifecycle, and Telegram notifier.

## Non-goals

- Private IP, localhost, LAN, DNS, TCP port, ICMP, cron heartbeat, authentication, or browser-rendered DOM checks.
- Validating SPA behavior beyond public HTTP availability. Todo FM initially passes when its homepage returns HTTP 200.
- Multiple notification destinations, alert acknowledgements, escalation schedules, or maintenance windows.
- Automatically unpausing a watcher.

## Approaches considered

1. **Public HTTP(S) Watchtower — selected.** Small HTTP client, persisted state, retries, and Telegram transitions. It covers the immediate Todo FM need within the server’s memory budget.
2. **Headless-browser monitoring.** Can inspect SPA-rendered DOM, but is too memory-heavy and operationally complex for vivo V11.
3. **Broad multi-protocol monitoring from the start.** Adds Ping, TCP, DNS, LAN, and heartbeat checks, but complicates validation, security, and the Dashboard before the core flow is proven.

## Functional design

### Watcher configuration

Each watcher has:

| Field | Rules |
|---|---|
| Name | Required, 1–80 characters, shown in UI and notifications. |
| URL | Required public `http` or `https` URL. Reject localhost, loopback, link-local, RFC1918/private, and resolved private IP addresses. |
| Expected status | Defaults to `200`; valid range 100–599. |
| Enabled state | `enabled` or `paused`. New watchers are enabled. |

The first record is prefilled by the owner through the Dashboard: `Todo FM`, `https://todo-fm-d.vercel.app/`, expected status `200`.

### Check policy

1. Every enabled watcher is checked every 5 minutes.
2. A check has a 10-second end-to-end deadline.
3. A failed attempt is retried up to 3 times with bounded backoff inside the check cycle.
4. The cycle is successful when any attempt returns the expected status; its latency is recorded.
5. A watcher becomes `down` after 3 failed cycles in a row. This normally means 10–15 minutes before the first alert.
6. A single successful cycle returns a `down` watcher to `up` and emits recovery.
7. Paused watchers are neither checked nor notified. Their existing history remains visible.
8. **Check now** records a diagnostic result but does not change the scheduled failure streak or emit a Telegram transition. It exists for configuration validation.
9. Editing a URL or expected status resets the watcher to an unknown/up baseline, clears any active outage metadata, and immediately performs a diagnostic check. Configuration edits never generate recovery messages.
10. On process startup, enabled watchers are scheduled once within a random 0–60 second window, then use the normal five-minute cadence. Persisted `down` state remains intact.

### Dashboard flow

The main Dashboard gets a compact Watchtower summary: number up, down, and paused; a red state when any watcher is down; and an obvious **Manage watchers** call to action.

The management page has a watcher list showing name, URL, current state, status/latency, last check, and action buttons. A Dashboard form provides Add, Edit, Pause, Resume, Delete, and Check now. Mutation actions request `WATCHTOWER_PIN`; the client may keep it only in local storage on that device, matching the existing relay pattern.

Saving a watcher immediately runs a check and shows its result. A failed initial result is saved as evidence and does not prevent monitoring.

V1 supports at most **25 watchers**. The scheduler permits at most **three network checks at once** to preserve the phone's memory, sockets, and battery.

### Notifications and logs

- On sustained outage, Telegram includes the watcher, URL, final check error or status, and outage start time.
- On recovery, Telegram includes watcher, URL, and measured downtime.
- Each transition and configuration mutation is written to `event_logs`.
- Repeated down cycles do not create Telegram messages or repeated event entries.

## Data model

New SQLite tables:

```text
watchers
  id, name, url, expected_status, enabled,
  state, consecutive_failures, down_since,
  last_checked_at, last_status_code, last_latency_ms, last_error,
  created_at, updated_at

watcher_checks
  id, watcher_id, checked_at, success,
  status_code, latency_ms, error, attempt_count
```

`watcher_checks` is purged after seven days. Deleting a watcher also deletes its history. This destructive Dashboard action must be explicit and PIN-protected.

## Interfaces

| Route | Purpose | Auth |
|---|---|---|
| `GET /api/watchtower/summary` | Aggregate states for Dashboard | public read-only |
| `GET /api/watchtower/watchers` | List watchers and latest state | public read-only |
| `GET /api/watchtower/watchers/:id/checks` | Seven-day bounded history | public read-only |
| `POST /api/watchtower/watchers` | Create and immediately check | `WATCHTOWER_PIN` |
| `PATCH /api/watchtower/watchers/:id` | Edit name, URL, expected status | `WATCHTOWER_PIN` |
| `POST /api/watchtower/watchers/:id/check` | Run one manual check | `WATCHTOWER_PIN` |
| `POST /api/watchtower/watchers/:id/pause` | Pause indefinitely | `WATCHTOWER_PIN` |
| `POST /api/watchtower/watchers/:id/resume` | Re-enable and immediately check | `WATCHTOWER_PIN` |
| `DELETE /api/watchtower/watchers/:id` | Delete watcher and history | `WATCHTOWER_PIN` |

## Failure handling

- Invalid URL or an address that resolves to a private network is rejected before storage and before any request is sent. Every resolved address must be public.
- At connection time, the checker resolves and validates the hostname again, then dials only a validated address while preserving the original hostname for HTTP Host and TLS SNI. This blocks DNS rebinding into private networks.
- DNS, TLS, timeout, redirect, malformed response, and unexpected status are retained as a concise error result.
- Redirects are not followed in v1; a redirect is an unexpected response unless its status is explicitly configured.
- A failed Telegram delivery is logged but does not block state persistence or later recovery detection.
- SQLite errors are surfaced in the management UI and logged. A check must never crash the server worker.

## Acceptance criteria

- Owner creates Todo FM via Dashboard PIN and sees an immediate HTTP 200 result.
- A configured unexpected HTTP response must not alert before three failed cycles.
- Exactly one down Telegram notification is emitted on the third failed cycle.
- Exactly one recovery message is emitted after a succeeding cycle.
- Pause suppresses checks and alerts until Resume; Resume checks immediately.
- Private, localhost, and private-resolving hostnames are rejected.
- Seven-day check history is displayed and purged without affecting watcher configuration.
- Existing telemetry collection, relay uploads, digest, and Telegram bot behavior remain intact.
