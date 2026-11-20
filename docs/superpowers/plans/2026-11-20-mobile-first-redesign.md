# Mobile-First Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every page of Pocket Sentinel phone-first. Every page uses one Sentinel Soft stylesheet. The schedule uses layout B (compact month grid with the selected day's list). The map uses layout A (full-screen map with a bottom sheet).

**Architecture:** The pages stay Go-embedded static HTML with vanilla JS and Alpine, and the API contracts do not change. `styles.css` is rewritten in place as the shared token and component layer. Page CSS (`dashboard.css`, `schedule.css`, `live-map.css`, `relay.css`) is rewritten or created from scratch. Page JS changes only where the layout requires it.

**Tech Stack:** Go `embed` + Gin, HTML/CSS, vanilla JS, Alpine.js 3.14.8, Leaflet 1.9.4, and Chart.js (all existing).

**Spec:** `docs/superpowers/specs/2026-11-20-mobile-first-redesign-design.md`

## Global Constraints

- Sentinel Soft tokens come from `DESIGN.md` (dark is the default; light uses `[data-theme="light"]`). Use the system font stack. Radii are 10–20px and borders are 1px hairline.
- At 360px width nothing may wrap inside a button and nothing may overflow horizontally. Every touch target is at least 44px.
- Content padding-bottom ≥ the tab bar height (56px) + `env(safe-area-inset-bottom)`.
- No Remixicon and no new CDN or dependency. Icons are inline SVG (`<svg class="icon" aria-hidden="true">`).
- Alpine holds primitives only. Leaflet and Chart instances stay outside `Alpine.data`.
- Call `invalidateSize()` or `resize()` after any visibility or size change.
- Bump `CACHE_NAME` in `sw.js`. Every `SHELL_ASSETS` static path must exist.
- Add `prefers-reduced-motion: reduce` to disable transitions. Motion runs 150–250ms with ease-out.
- No Go changes outside `modules/dashboard/dashboard_test.go`.
- Deviation from the spec: the shared layer keeps the filename `styles.css` (instead of `base.css`) to avoid churn in the SW, the tests, and dormant pages.

## Review Focus

- A day with 0 reminders, and a month with no reminders at all: the list shows an empty state and the grid shows no dots. Test this in Task 2 with a manual check using an empty DB.
- A friend location that is missing (null) or stale (older than 10 minutes): the sheet's first line must say "ไม่มีพิกัด" or show the age, and must not render "NaN km". Check in Task 3 with an empty DB.
- The phone keyboard is open in the add-reminder sheet or the Relay composer: the primary button must stay reachable (the sheet scrolls, and the button is not fixed behind the keyboard). Check manually at 390×500 in Tasks 2 and 4.
- Light theme on every page: no white-on-white or invisible borders. Check in every task's screenshot step.
- PIN missing or wrong on save (reminder or geofence): the error stays visible inside the sheet or wizard. Check manually in Tasks 2 and 3.

---

### Task 1: Shared foundation + dashboard migration

**Files:**
- Rewrite: `modules/dashboard/static/css/styles.css` (tokens, reset, type, `.topbar`, `.task-nav`, `.card`, `.btn`, `.btn-primary`, `.btn-ghost`, `.btn-icon`, `.chip`, `.list`, `.list-item`, `.field`, `.sheet`, `.sheet-backdrop`, `.fab`, `.toast`, `.icon`, `.empty`, the reduced-motion guard). Target ≤ 600 lines.
- Create: `modules/dashboard/static/css/dashboard.css` (dashboard-only sections, moved from the old styles.css and restyled).
- Modify: `modules/dashboard/index.html` (link `dashboard.css`, drop Remixicon, use inline SVG icons, use the shared topbar and nav markup, remove inline padding styles).
- Modify: `modules/dashboard/static/js/components/digest.js` (replace the `ri-*` icon strings with inline SVG strings or a status dot span).
- Test: `modules/dashboard/dashboard_test.go`

**Interfaces:**
- Produces the shared markup used by Tasks 2–4:
  - topbar: `<header class="topbar"><a class="brand" href="/"><span class="status-dot ok"></span><span class="brand-name">…</span></a><div class="topbar-actions">…<button class="btn-icon theme-toggle-btn" id="theme-toggle">…</button></div></header>`
  - nav: `<nav class="task-nav" aria-label="หน้าหลัก">` containing 4 `a.task-link`, each with a `<svg class="icon">` and a `<span>`. The current page carries `.active` and `aria-current="page"`. The hrefs are `/?tab=overview`, `/schedule`, `/live`, `/relay`.
  - `.sheet` (`position: fixed; bottom: 0`, rounded top, `max-height: 85dvh`, `overflow: auto`, safe-area padding) with `.sheet.open`, and `.sheet-backdrop.open`.
  - `.fab` (56px circle, `bottom: calc(56px + env(safe-area-inset-bottom) + 16px)`, right 16px).

- [ ] **Step 1: Write the failing test** `TestPagesDropIconFontAndShareNav` in `dashboard_test.go`. It requests `/`, `/schedule`, `/relay`, and the live page handler, and asserts that each body does not contain `remixicon` and does contain `class="task-nav"` and `class="topbar"`.
- [ ] **Step 2: Run** `go test ./modules/dashboard/ -run TestPagesDropIconFontAndShareNav`. Expect FAIL (`remixicon` is found).
- [ ] **Step 3: Rewrite `styles.css`, create `dashboard.css`, and migrate `index.html` and `digest.js`.** Keep every Alpine binding and every class or ID used by `app.js`, `charts.js`, `telemetry.js`, `schedule-summary.js`, and `digest.js`. Keep `chart-insight` and `metric-grid` (the existing tests check them). For now, only the topbar and nav markup of the other three pages change, so the test can pass.
- [ ] **Step 4: Run** `go test ./modules/dashboard/`. Expect PASS.
- [ ] **Step 5: Visual check.** Build and run the server on :8091, then screenshot `/` at 390px in dark and light. Expect no wrapped buttons and nothing under the nav. The console must show no errors.
- [ ] **Step 6: Commit** with `feat(ui): shared Sentinel Soft foundation, inline icons, dashboard migration`.

### Task 2: Schedule layout B

**Files:**
- Rewrite: `modules/dashboard/schedule.html`, `modules/dashboard/static/css/schedule.css`
- Modify: `modules/dashboard/static/js/components/schedule.js`
- Test: `modules/dashboard/dashboard_test.go`

**Interfaces:**
- Consumes the Task 1 classes `.topbar`, `.task-nav`, `.sheet`, `.fab`, and `.list-item`.
- DOM contract (these IDs stay): `#month-grid`, `#month-label`, `#previous-month`, `#next-month`, `#jump-today`, `#agenda-list` (now the selected-day list, always visible), `#add-reminder` (now the `.fab`), `#reminder-dialog` (now a bottom sheet `<dialog>` containing the `#nl-*` block above `#reminder-form`), `#pin-open`, `#pin-dialog`, `#telegram-status`, `#schedule-paused`, `#paused-list`, `#delivery-issues`, `#delivery-issue-list`, `#schedule-history`, `#schedule-history-list`, `#schedule-toast`, and the theme toggle.
- These IDs are removed: `#month-view-button`, `#agenda-view-button`, `#next-weekday`, `#next-day`, `#schedule-next-title`, `#schedule-next-description`. Remove `.weekday-row` hiding too.
- New element: `#day-heading` (an `h2` above `#agenda-list`).

- [ ] **Step 1: Write the failing test** `TestScheduleUsesMonthPlusDayLayout`. It GETs `/schedule` and asserts the body contains `id="day-heading"`, `class="fab"`, and every `nl-*` ID already checked. It also asserts the body does not contain `agenda-view-button` or `schedule-next-title`.
- [ ] **Step 2: Run** `go test ./modules/dashboard/ -run TestScheduleUsesMonthPlusDayLayout`. Expect FAIL.
- [ ] **Step 3: Rewrite the markup and CSS.**
  - Month header: one row, `‹ label ›` with a "วันนี้" `.btn-ghost`.
  - Grid: 7 columns, cells `aspect-ratio: 1 / 0.9`, max 44px tall, no cell borders. `.today` gets a ring, `.selected` gets a blue fill, and `.has-events::after` adds a 4px green dot.
  - `#day-heading`, then `#agenda-list` as `.list-item` rows with the time in blue, the title and details, and one `⋯` button per row.
- [ ] **Step 4: Change `schedule.js`.**
  - Delete `setView`, the view buttons, and the `matchMedia` agenda switch.
  - `renderAgenda()` renders only `state.selected` and sets `#day-heading` to `วันนี้ · N รายการ` when the selected day is today, otherwise `${dateFormatter.format(day)} · N รายการ`.
  - A grid click sets `state.selected`, then calls `renderMonth(); renderAgenda()`.
  - Month arrows: if `state.selected` is not in the new month, set it to the 1st of that month.
  - `renderTop()` writes only `#telegram-status`.
  - Row `⋯` toggles an inline `.row-actions` strip (edit, pause, done, delete) that reuses the existing `data-action` buttons and `act()`. Only one row is open at a time.
  - `openForm()` opens the sheet dialog with `showModal()`. NL parse and preview stay inside it.
- [ ] **Step 5: Run** `go test ./modules/dashboard/`. Expect PASS.
- [ ] **Step 6: Visual and functional check** at 360, 390, and 430px in dark and light.
  - With an empty DB: the empty state shows and there are no dots.
  - Set a PIN (the `REMINDER_PIN` from `.env` on the local server), add a reminder through the form, and confirm the dot and the list row appear. Then edit it, mark it done, and delete it. A wrong PIN shows its error.
  - At 390×500 the save button is reachable by scrolling.
  - The console shows no errors.
- [ ] **Step 7: Commit** with `feat(schedule): mobile month + selected-day layout with add sheet`.

### Task 3: Map layout A

**Files:**
- Rewrite: `modules/dashboard/live_map.html`, `modules/dashboard/static/css/live-map.css` (target ≤ 700 lines)
- Modify: `modules/dashboard/static/js/components/live-map.js`
- Test: `modules/dashboard/dashboard_test.go`

**Interfaces:**
- Consumes the Task 1 `.sheet`, `.btn-icon`, and `.chip`. The theme toggle keeps `theme-toggle-btn` (an existing test needs it).
- New Alpine state: `sheetExpanded: false` (boolean).
- New methods: `toggleSheet(): void`, and `friendHeadline(): string`, which returns `"Furturia ห่าง ${distanceText}"`, or `proximityText` when colocated, or `"ยังไม่มีพิกัดของ Furturia"` when `friend` is null.
- After any change to `sheetExpanded` or `pickingLocation`, call `liveMapState.map?.invalidateSize({ pan: false })` in `$nextTick`.
- Keep `centerOnDevices`, `toggleTrail`, `zoomIn`, `zoomOut`, `refresh`, `startMapPicker`, the picker methods, and the geofence and notification methods with unchanged names.

- [ ] **Step 1: Extend the test** `TestLiveMapPageIsEmbeddedAndResponsive`. Its want list adds `class="map-sheet` and `friendHeadline()`. Add a must-not list: `map-controls-group`, `live-summary`.
- [ ] **Step 2: Run** `go test ./modules/dashboard/ -run TestLiveMap`. Expect FAIL.
- [ ] **Step 3: Rewrite the markup and CSS.**
  - Layout: `body` is `100dvh` with no scroll. The canvas is fixed at `inset: 0`.
  - `.map-status` is a floating chip at the top (connection dot, updated text, theme button). Tapping it calls `refresh()`.
  - `.map-fabs` sits on the right with 2 `.btn-icon` buttons (show both, trail). Zoom buttons are wrapped in `@media (hover: hover)`.
  - `aside.map-sheet` sits above the nav and is `:class` bound to `is-expanded`. Collapsed it shows the handle, the friend row (`friendHeadline()`, then `deviceAge(friend)` · speed state · battery), and an own-status chip. Expanded, at max-height 75dvh, it adds both device cards, the geofence list, Discord settings, and the ping log (collapsed, `showLogs` default false).
  - The picker panel replaces the sheet content while `pickingLocation` is true.
  - Move Leaflet attribution to `topleft` below the chip.
- [ ] **Step 4: Change `live-map.js`.** Add `sheetExpanded`, `toggleSheet`, and `friendHeadline`. Set `showLogs: false`. Implement handle drag with pointer events: a drag greater than 40px up expands the sheet and greater than 40px down collapses it, and a tap toggles it. Run `invalidateSize` on both changes.
- [ ] **Step 5: Run** `go test ./modules/dashboard/`. Expect PASS.
- [ ] **Step 6: Visual and functional check** at 360, 390, and 430px in dark and light, with `/map` on the local server.
  - With an empty DB the headline reads "ยังไม่มีพิกัดของ Furturia" (no NaN).
  - The sheet expands and collapses.
  - The picker opens with the reticle visible, and a wrong PIN on save shows an error.
  - Trail and show-both work.
  - The console shows no errors.
- [ ] **Step 7: Commit** with `feat(map): full-screen map with friend bottom sheet`.

### Task 4: Telegram Relay migration

**Files:**
- Modify: `modules/dashboard/tele-bridge.html`, `modules/dashboard/static/js/components/relay.js` (icon strings only, if any)
- Create: `modules/dashboard/static/css/relay.css` (relay-only rules moved out of the old styles.css)

**Interfaces:** Consumes the Task 1 classes. Keep `relayComponent`, `proofread-btn`, `proofread-diff-card`, the "Telegram Bridge" text, and all Alpine bindings.

- [ ] **Step 1: Migrate** the markup to the shared topbar, nav, `.card`, `.btn`, and `.field`. The send button must not wrap, and at 390×500 the composer scrolls with Send reachable.
- [ ] **Step 2: Run** `go test ./modules/dashboard/`. Expect PASS (the existing relay assertions stay green).
- [ ] **Step 3: Visual check** at 390 in dark and light. Text mode and file mode both switch, the file picker opens, and the console shows no errors. Do not actually send.
- [ ] **Step 4: Commit** with `feat(relay): migrate Telegram Relay to shared mobile styles`.

### Task 5: Service worker, cleanup, verification, ship

**Files:**
- Modify: `modules/dashboard/sw.js`, `modules/dashboard/dashboard_test.go`

- [ ] **Step 1: Write the failing test** `TestShellAssetsAllExist`. It parses `SHELL_ASSETS` from the embedded `sw.js` with a regexp on `'/static/[^']+'`, and asserts that a GET of each returns 200 with a non-empty body.
- [ ] **Step 2: Update `sw.js`.** Set `CACHE_NAME = 'sentinel-shell-v33'`, and add `/static/css/dashboard.css` and `/static/css/relay.css`. Do not add `live-map.css`; the map stays network-only, as it is today.
- [ ] **Step 3: Run** `go test ./... && go vet ./... && go build -o /dev/null .`. Expect all to pass.
- [ ] **Step 4: Final sweep.** Grep the active pages and CSS for `ri-`, `remixicon`, `border: 2px`, `border: 3px`, and `font-family: .*mono` on buttons. Expect no hits outside data or mono readouts.
- [ ] **Step 5: Commit** with `chore(pwa): bump shell cache v33 and verify shell assets`, then run `git push origin main` (AGENTS.md protocol).
