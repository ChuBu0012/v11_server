# Pocket Sentinel Web Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Redesign every active Pocket Sentinel web page for mobile-first RawBlock usability, informative telemetry, smooth motion, and faster initial loading.

**Architecture:** Keep the embedded Go pages and existing API contracts. Share a small CSS design system and task navigation across active pages; retain existing page-specific JavaScript while replacing markup/styles and lazily loading heavy page-only assets.

**Tech Stack:** Go `embed`, HTML, CSS, vanilla JavaScript, Alpine.js, Chart.js (existing only).

**Spec:** `docs/superpowers/specs/2026-11-20-pocket-sentinel-redesign.md`

## Global Constraints

- Follow `DESIGN.md` RawBlock colors, square corners, thick borders, and typography.
- Mobile-first; no legacy top bar + horizontal navigation pattern.
- No new runtime dependencies, build step, sessions, or session tracking.
- Preserve active API contracts and reminder/relay/location workflows.
- Remove Monitor/Watchtower from active UI while leaving backend services untouched.
- Maintain Alpine proxy guards, chart resize lifecycle, direct history fallback, and PWA cache invalidation.
- Use reduced-motion behavior for nonessential animation.

## Review Focus

- Empty or sparse telemetry history must explain that insight is unavailable rather than inventing a trend; keep explicit no-data UI in chart insight generation.
- One data point must render without a broken/hidden canvas; preserve the sample count check and resize canvas after reveal.
- On narrow screens and safe-area devices, the task bar must not cover actionable controls; apply bottom safe-area padding.
- PIN protected schedule and relay actions must retain their current authorization semantics; keep PIN entry and error feedback in the redesign.
- Offline and stale telemetry states must remain distinguishable from healthy state; preserve status derivation from collector time/error and conditions.

---

### Task 1: Shared RawBlock foundation and task navigation

**Files:**
- Modify: `modules/dashboard/static/css/styles.css`
- Modify: `modules/dashboard/index.html`
- Modify: `modules/dashboard/schedule.html`
- Modify: `modules/dashboard/tele-bridge.html`
- Modify: `modules/dashboard/live_map.html`
- Modify: `modules/dashboard/static/css/schedule.css`
- Modify: `modules/dashboard/static/css/live-map.css`

**Interfaces:** Shared CSS tokens and `.task-nav` markup links Overview, Schedule, Map, and Telegram across active pages.

- [x] Replace legacy shared palette, rounded cards, and horizontal `.sentinel-nav` treatment with RawBlock tokens, square bordered components, focus-visible states, and mobile-first layout rules.
- [x] Replace top/horizontal navigation on each active page with a task-oriented bottom navigation on mobile and a compact non-topbar arrangement at wider widths.
- [x] Add safe-area padding, keyboard focus, touch target sizing, and reduced-motion rules.
- [x] Remove eager icon-font usage from shared controls and use readable text controls instead.
- [x] Confirm all four active pages link to the correct existing route or tab.

### Task 2: Dashboard hierarchy, chart insights, and loading path

**Files:**
- Modify: `modules/dashboard/index.html`
- Modify: `modules/dashboard/static/js/app.js`
- Modify: `modules/dashboard/static/js/components/charts.js`
- Modify: `modules/dashboard/static/js/components/three-core.js` (only if teardown is needed)
- Modify: `modules/dashboard/sw.js`

**Interfaces:** Chart insight view-models are derived from history samples in `charts.js` and displayed with the current selected time range. `app.js` only accepts active tabs `overview`, `map`, and `telegram`.

- [x] Recompose the dashboard around operational state, current resources, upcoming reminder, telemetry history, digest, location shortcut, and recent events; remove Monitor and Watchtower sections/views.
- [x] Replace the 3D core in the initial dashboard with a lightweight status presentation and remove eager Three.js/OrbitControls loading.
- [x] Derive per-chart insight text from actual valid samples, including direction/period comparison and an insufficient-data state; do not label raw values as insights.
- [x] Preserve history fallback loading, event updates, and Chart.js instances outside Alpine proxies; resize hidden charts after reveal.
- [x] Lazy-load Chart.js near the chart section; load Leaflet only on the separate map route.
- [x] Remove Monitor/Watchtower scripts and assets from active shell caching, increment `CACHE_NAME`, and keep the dashboard, schedule, and Relay shell assets available offline; the tokenized live map remains network-only.

### Task 3: Agenda-first schedule

**Files:**
- Modify: `modules/dashboard/schedule.html`
- Modify: `modules/dashboard/static/css/schedule.css`
- Modify: `modules/dashboard/static/js/components/schedule.js` only where needed to support view behavior.

**Interfaces:** Keep existing reminder REST endpoints, DOM IDs used by `schedule.js`, PIN flow, and Telegram delivery status.

- [x] Reorder the mobile schedule around the next reminder and an agenda list; make month browsing a secondary view.
- [x] Restyle reminder creation, natural language preview, reminder dialog, PIN dialog, paused reminders, delivery issues, and history to RawBlock.
- [x] Keep create/edit/pause/resume/complete/delete and month/agenda switching usable by touch and keyboard.
- [x] Ensure form errors and AI ambiguity messages remain visible and actionable.

### Task 4: Focused Telegram Relay compose page

**Files:**
- Modify: `modules/dashboard/tele-bridge.html`
- Modify: `modules/dashboard/static/js/components/relay.js` only where needed for redesigned controls.
- Modify: `modules/dashboard/static/css/styles.css`

**Interfaces:** Keep `/api/relay/status`, `/api/relay/send`, current PIN header behavior, proofreading endpoint, and clipboard paste/drop handling.

- [x] Recompose the page as a focused message/file composer with clear text and file modes and one prominent send action.
- [x] Preserve PIN setup, AI proofreading preview, recipient/status context, file preview/removal, 20 MB guidance, upload progress, and send feedback.
- [x] Ensure mobile file picking and keyboard paste work without relying on drag and drop.

### Task 5: Focused live map and route cleanup

**Files:**
- Modify: `modules/dashboard/live_map.html`
- Modify: `modules/dashboard/static/css/live-map.css`
- Modify: `modules/dashboard/static/js/components/live-map.js`
- Modify: `modules/dashboard/dashboard.go`
- Modify: `modules/dashboard/dashboard_test.go` only to remove expectations for retired Monitor/Watchtower pages/assets and replace them with active route expectations.

**Interfaces:** Preserve live map authorization/query behavior and active map APIs; remove the Watchtower page route and redirect stale `/watchtower` links to `/` or return not found consistently.

- [x] Redesign the live map page with task navigation, readable location/update status, map-first mobile sizing, and safe-area support.
- [x] Load Leaflet CSS/script and map component only on the map page.
- [x] Remove Watchtower route embedding and Monitor/Watchtower active-page references, unmount their APIs and Watchtower scheduler, and leave backend packages and dormant source files available for later reintroduction.
- [x] Update existing dashboard assertions that encode retired UI behavior.

### Task 6: PWA cache and final source audit

**Files:**
- Modify: `modules/dashboard/sw.js`
- Modify: `modules/dashboard/index.html`
- Modify: `modules/dashboard/schedule.html`
- Modify: `modules/dashboard/tele-bridge.html`
- Modify: `modules/dashboard/live_map.html`
- Modify: `modules/dashboard/dashboard.go`

- [x] Confirm the service worker cache version has been incremented after frontend edits and shell assets contain only active pages/assets.
- [x] Search active page sources for legacy Monitor/Watchtower UI and old shared navigation, and remove remaining visible references.
- [x] Perform source-level checks for active API endpoint preservation, script loading order, reduced-motion rules, accessibility labels, and absence of session writes.
- [x] Do not add or run automated tests unless the user requests verification.
