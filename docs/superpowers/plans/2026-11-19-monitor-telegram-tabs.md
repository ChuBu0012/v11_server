# Monitor and Telegram Tabs Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a Monitor tab with useful deterministic and AI-generated health insight, and make Telegram work inside the shared Dashboard shell.

**Architecture:** Extend the current Dashboard HTML and Alpine components with URL-backed in-page navigation. Keep the existing Watchtower and Schedule pages intact. Add a bounded backend insight endpoint that uses the existing OpenRouter client and accepts only structured health evidence.

**Tech Stack:** Go, Gin, existing `shared/ai` OpenRouter client, HTML, Alpine.js, existing CSS.

**Spec:** `docs/superpowers/specs/2026-11-19-monitor-telegram-tabs-design.md`

## Global Constraints

- SQLite stays on the pure-Go `modernc.org/sqlite` driver; do not add CGO.
- OpenRouter calls use the existing `openrouter/free` restriction and server-side client.
- Do not put complex external library instances in Alpine data.
- Bump service worker `CACHE_NAME` when frontend assets change.
- Preserve Telegram PIN protection and existing text, file, clipboard, and proofreading behavior.

## Review Focus

- Missing or incomplete telemetry history: Monitor shows available current data and a useful empty state.
- Missing OpenRouter API key or provider failure: deterministic insight remains visible and the AI error is clear.
- Oversized or malformed AI input: endpoint returns a client error without calling the provider.
- Browser back/forward and direct `?tab=` entry: selected view stays synchronized with the URL.
- Telegram legacy URLs: `/tele-bridge` and `/relay` still reach a working Telegram experience.

## File Structure

- Modify `modules/dashboard/index.html` to add the Monitor and Telegram navigation/views and reuse the existing shared shell.
- Modify `modules/dashboard/static/js/app.js` to manage the selected view and URL/history synchronization.
- Create `modules/dashboard/static/js/components/monitor.js` for telemetry loading, deterministic insight, and AI request state.
- Modify `modules/dashboard/static/js/components/relay.js` only as needed to keep the shared Telegram composer behavior intact.
- Modify `modules/dashboard/static/css/styles.css` for Monitor cards and responsive layout.
- Modify `modules/dashboard/static/js/components/telemetry.js` only if shared metric formatting is needed.
- Modify `modules/dashboard/tele-bridge.html` or `modules/dashboard/dashboard.go` so the old endpoint opens the Telegram tab in the shared Dashboard.
- Modify `modules/dashboard/sw.js` to precache any new assets and increment the cache name.
- Create `shared/ai/monitor_insight.go` for a constrained Thai insight prompt and structured result/fallback.
- Modify `main.go` to mount `POST /api/monitor/insight` and validate structured evidence before calling AI.

## Tasks

### Task 1: Add bounded server-side Monitor insight

**Interfaces:** `POST /api/monitor/insight`, JSON request with structured current metrics, up to 24 trend points, and bounded Watchtower summary; JSON response with `insight` and fallback metadata.

- [x] Add request validation for required metrics, maximum 24 trend points, and a bounded Watchtower summary; malformed or oversized payload returns HTTP 400.
- [x] Add `shared/ai.SummarizeMonitorInsight(ctx, client, evidence)` returning concise Thai facts and next checks, with deterministic fallback when OpenRouter is not configured or fails.
- [x] Mount PIN-protected endpoint in `main.go`; keep API credentials server-side and do not accept free-form prompt text.
- [x] Review that AI output is advisory and cannot affect health status or alerts.

### Task 2: Build the Monitor tab

**Consumes:** `POST /api/monitor/insight` from Task 1 and existing telemetry/Watchtower read APIs.

- [x] Add `monitorComponent()` in `modules/dashboard/static/js/components/monitor.js`; load current telemetry, recent history, and Watchtower summary on initialization.
- [x] Show current health, key metric readings, recent trend observations, and active conditions using text bindings (no unsafe HTML rendering).
- [x] Add an explicit AI analysis action; submit only a summarized bounded evidence object and show loading, success, fallback, and error states.
- [x] Add Monitor view markup and responsive styling to `index.html` and `styles.css`.

### Task 3: Add shared in-page navigation and Telegram view

- [x] Add URL-backed `overview`, `monitor`, and `telegram` selection in `app.js`; use `history.pushState` and handle `popstate`.
- [x] Render the new shared views with Alpine `x-show`, preserving Dashboard shell and avoiding reload when switching among these tabs.
- [x] Move/reuse Telegram composer markup in the Telegram view with text/file tabs, PIN setup, and proofread preview intact; preserve clipboard paste behavior.
- [x] Keep dedicated Watchtower and Schedule links functioning as before.
- [x] Keep legacy `/tele-bridge` and `/relay` entry points working; the Dashboard navigation opens the in-shell Telegram tab.
- [x] Update `sw.js` cache version and shell assets for the new Monitor component.

### Task 4: Review the finished changes

- [x] Inspect the diff for OpenRouter key exposure, prompt size bounds, PIN authorization, unsafe HTML, and SPA route regressions.
- [x] Complete independent code review, including route registration, navigation state transitions, and mobile layout code paths. The review's public AI endpoint finding was addressed by requiring Relay PIN. Do not deploy or restart the service as part of this task.
