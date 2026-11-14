# Pocket Sentinel — Implementation Plan

**Status:** Proposed  
**Source:** [spec.md](./spec.md)  
**Task checklist and evidence:** [tasks.md](./tasks.md)

## Approach

Implement in small vertical slices. First restore a reliable baseline around the current modular server, then add persistent samples and a bounded read API, then add alert state and Telegram delivery, and finally deliver the dashboard and verify the deployment artifact. Keep collection and notification logic independent of Gin handlers so failures and transitions can be tested without a running phone.

## Phases

### 0. Baseline and configuration

- Inspect and align current tests with the modular implementation.
- Add centralized configuration for collector, retention, alert thresholds, and Telegram credentials.
- Add dependency injection/startup wiring and graceful lifecycle for background work.

**Exit:** existing routes have passing tests; configuration has documented defaults and secrets are not logged.

### 1. Persistent telemetry

- Add SQLite schema and repository methods for health samples and retention.
- Implement collector sampling on a configurable ticker.
- Expose bounded history and current-state APIs.

**Exit:** deterministic tests prove sample persistence, ordering, bounds, retention, and battery-unavailable behavior.

### 2. Alert evaluation and Telegram

- Model alert conditions and transitions independently from the transport.
- Persist enough alert state to avoid duplicate notifications across process restarts.
- Add Telegram sender with timeout, redacted errors, and configuration validation.

**Exit:** tests prove problem/repeat/recovery behavior; a configured integration check confirms Telegram delivery without exposing credentials.

### 3. Dashboard

- Add a responsive page that reads the APIs and renders current status, history, and alert states.
- Serve assets from the Go binary/repository with no Node process required on the phone.
- Provide unavailable/empty states for optional hardware and notification integrations.

**Exit:** dashboard can be served locally, renders API data, and does not expose secrets or mutation controls.

### 4. Deployment and release verification

- Update operational configuration/documentation and deployment as needed.
- Build for the target Android ARM64 environment and verify the service health and dashboard endpoints.
- Record deployment/runtime evidence and known limitations.

**Exit:** target build starts under the established PM2 binary workflow, endpoints respond, and memory is within the project limit.

## Work sequencing

Complete tasks in `tasks.md` in order within each phase. Tasks may be split further during implementation, but their acceptance criteria and evidence fields must remain attached to the resulting work. Do not mark a task complete from code inspection alone: execute its stated verification and paste concise actual output or link to the artifact/log.

## Risk controls

- Keep collector intervals and API result sizes bounded.
- Ensure Telegram network calls use short timeouts and do not run on HTTP request paths.
- Persist alert state so restarts do not create notification storms.
- Treat absent Termux API and absent Telegram configuration as supported degraded modes.
- Review public tunnel exposure before enabling any write or secret-bearing endpoint.
- Preserve Termux operating constraints documented in the repository `AGENTS.md`.
