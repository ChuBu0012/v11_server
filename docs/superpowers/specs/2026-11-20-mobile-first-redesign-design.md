# Pocket Sentinel Mobile-First Redesign — Design

Date: 2026-11-20 · Supersedes the visual parts of `2026-11-20-pocket-sentinel-redesign.md` (RawBlock), which was later replaced by Sentinel Soft in commit `82874c1`.

## Intent and why

The owner is almost the only user and opens the site on a phone. Two pages are broken on a phone today:

- **Schedule (`/schedule`)**: the owner opens it to see **what is on today**.
- **Map (`/live/:token`)**: the owner opens it to check **where the friend is**.

Root cause: two design systems were layered on top of each other on the same day. RawBlock rules (thick white borders, square monospace buttons, uppercase eyebrows) remain in `styles.css` (2,517 lines), `schedule.css`, and `live-map.css` (2,014 lines), and they collide with Sentinel Soft (dark, rounded, gentle contrast). Visible results at 390px width: buttons whose labels wrap ("ก่อน", "ถัดไป", "อัปเดต", "กึ่งกลาง"), map controls that overflow, the map's bottom area hidden behind the tab bar, and a month grid plus several form blocks that all fight for the first screen.

## Goals

1. One design system across all four pages: **Sentinel Soft**, as described in `DESIGN.md` (tokens, system font, rounded cards, subtle borders). The dashboard currently renders closest to this and is the reference.
2. Mobile-first: design for 360–430px widths first. Wider screens get the same layout with a max-width, plus extra columns where it helps.
3. **Schedule = layout B**: a compact month grid at the top, where days with reminders show a dot. Below it is the list for the selected day (today by default). A floating **+** button adds a reminder.
4. **Map = layout A**: a full-screen map, a small status chip at the top, only two floating controls, and a bottom sheet whose first line answers "how far is the friend, and how fresh is that location?"
5. Dashboard and Telegram Relay: move them onto the new shared CSS and fix any mobile layout problems. Their structure stays.
6. Rewrite the CSS rather than patching it. The goal is a much smaller total CSS than today's ~4,800 lines.

## Non-goals

- No changes to Go routes, APIs, auth (Reminder PIN or Relay PIN), the database, or Telegram/Discord behavior.
- No single-page-app merge, no framework, no build step, no new runtime dependency.
- No new features beyond the layouts that were picked.

## Anti-goals (failure even if it "works")

- Any button label that wraps, or any control that overflows at 360px.
- Anything actionable sitting under the bottom tab bar or the iOS home indicator.
- Losing an existing function: add, edit, pause, complete, or delete a reminder; AI natural-language parse; PIN; paused, delivery-issue, and history lists; geofence wizard; Discord notification settings; trail toggle; ping log; relay text, file, and paste; proofreading.
- RawBlock leftovers (thick 2–3px borders, monospace buttons, uppercase English eyebrows) appearing anywhere.

## Constraints (from AGENTS.md)

- Alpine state holds primitives only. Leaflet, Chart.js, and Three.js instances stay outside `Alpine.data`.
- Charts that start hidden must call `resize()` in `$nextTick` after they are revealed. Keep the `loadHistory()` fallback in `init()`.
- Bump `CACHE_NAME` in `sw.js`, and make sure the shell cache lists only the CSS and JS files that still exist.
- Keep the DOM IDs that `schedule.js` binds to (about 40 of them, such as `#month-grid`, `#agenda-list`, `#pin-dialog`, and `#nl-*`). If an ID must change, change it in the JS in the same commit.
- Go hook: none of this touches `.go` files except `dashboard_test.go`, if a test checks the markup.

## Approaches considered

1. **Rewrite the shared CSS as one system and restructure every page (chosen).** Replace `styles.css`, `schedule.css`, and `live-map.css` with a small token and component layer plus page-scoped files. Restructure the schedule and map markup, and adjust only the JS rendering code. This fixes the root cause. It costs the most effort and needs a full click-through of every function.
2. **Rewrite only the schedule and map styles on top of the existing CSS.** This is faster, but the colliding legacy CSS stays and breaks the next page that gets touched.
3. **Merge into a single-page app.** Transitions would be smoother, but the reminder and relay flows would have to move, so regression risk is high, and it does not address the complaint.

The owner chose approach 1.

## Design

### Shared foundation

- **`static/css/base.css` (new):** design tokens for dark and light themes from `DESIGN.md`, plus reset, type scale, and components: `.card`, `.btn` (primary/secondary/ghost/icon, with a minimum height of 44px and `white-space: nowrap`), `.chip`, `.list-item`, `.sheet` (bottom sheet), `.fab`, `.dialog` (full-height bottom sheet on mobile, centered on ≥640px), form fields, `.toast`, `.task-nav`, the top bar, focus-visible rings, and a reduced-motion guard. It replaces `styles.css`. The dashboard-only rules move to `static/css/dashboard.css`.
- **Top bar:** one compact row (status dot, page title, theme toggle) that is the same on all pages. Page-specific actions go into the page body, not the top bar. On the map, the top bar is replaced by the floating status chip.
- **Bottom tab bar (`.task-nav`):** it keeps its 4 items and hrefs. Its height is 56px plus `env(safe-area-inset-bottom)`. Page content gets matching bottom padding. On ≥900px it can stay at the bottom, centered with a max-width (builder's call). It must not become a top nav again.
- **Icons:** replace the Remixicon CDN font with a small inline SVG set for the roughly 10 icons actually used: nav ×4, theme, plus, chevrons, locate, people, close. This drops a blocking cross-origin stylesheet and font download. (Claude's call; revert if any icon is missing.)

### Schedule (layout B)

Top to bottom on mobile:

1. Top bar. The PIN button becomes a small lock icon button, and the "เวลาไทย" chip is removed (the timezone is stated once in the footer or form hint).
2. **Month header row:** `‹  พฤศจิกายน 2569  ›` with a "วันนี้" text button. Everything stays on one line.
3. **Compact month grid:** 7 columns, each cell about 40px tall, with no borders on individual cells. Today gets a ring, the selected day is filled with blue, and days with reminders get a green dot underneath. It shows 6 weeks at most, and outside-month days are muted.
4. **Selected-day list:** a heading such as "วันนี้ · 3 รายการ", or "ส. 21 พ.ย. · 1 รายการ". Each reminder is a list item: time in blue, then title and details. Row actions (edit, pause, done, delete) move from inline buttons into a tap-to-open action sheet, or into one "⋯" button per row, so rows never wrap. An empty day shows a friendly empty state.
5. The paused list, delivery issues, and history (a collapsible `<details>`) stay below in the same list style.
6. **FAB (+):** sits bottom-right above the tab bar and opens an "add reminder" bottom sheet. The **AI natural-language input sits at the top of that sheet**, with the regular form fields below it. This removes the always-visible AI bar from the first screen. The AI preview card and ambiguity alert render inside the sheet.
7. Tapping a day only changes the selection. It no longer switches to a separate "agenda view". The month/agenda toggle and the forced mobile agenda switch in `DOMContentLoaded` are removed. The selected-day list replaces both.

The separate "UP NEXT" card (`#next-weekday`, `#next-day`, `#schedule-next-title`) is removed, because the selected-day list for today already answers that question. `renderTop()` keeps only the Telegram status line, which becomes a single muted line under the month header and appears only when Telegram is not connected. (Claude's call.)

JS changes in `schedule.js`: `renderAgenda()` always renders the selected day only. Remove `setView` and the view buttons. The grid click sets `state.selected` and re-renders. The month arrows keep the selection if it stays inside the month, and otherwise select the 1st. "วันนี้" resets to today. Row actions use the existing `act(button)` path. The NL creator moves into `#reminder-dialog`.

### Map (layout A)

- `#live-map-canvas` fills the viewport (`100dvh`) behind everything. The page does not scroll in its default state.
- **Status chip (top, floating):** a connection dot, "อัปเดตสด" or "รอเชื่อมต่อ", the time since the last update, and a theme toggle. A tap on it calls refresh. It replaces the header bar and the summary row.
- **Two floating controls (right):** "show both" (`centerOnDevices`) and "trail" (`toggleTrail`). Zoom uses pinch, and the +/− buttons are removed on touch (they may stay on desktop via `@media (hover: hover)`).
- **Bottom sheet** above the tab bar, with two snap states:
  - **Collapsed (default):** the friend's avatar, then **"Furturia ห่าง 2.4 กม."** (or the colocated text), then "กำลังเคลื่อนที่ · 1 นาทีก่อน · แบต 64%". On the right is a small chip for the owner's own status. This answers the owner's question without any interaction.
  - **Expanded** (tap the handle or swipe up, up to about 75% of the height, scrolling inside the sheet): both device cards (battery, speed, accuracy), places (geofences) with "+ เพิ่มสถานที่", Discord notification settings, and the ping log (collapsed by default).
- **Place picker wizard:** opening it collapses the sheet and replaces it with the wizard (same three steps and same Alpine state), so the reticle on the map stays visible. Wizard footer buttons sit above the safe area.
- Leaflet's own attribution stays, moved to the top-left under the chip or kept small, so it does not collide with the sheet.
- Alpine gets only primitive flags (`sheetExpanded`); the Leaflet map stays outside the proxy, as it does today. After the sheet or wizard changes the visible map size, call `map.invalidateSize()`.

### Dashboard and Telegram Relay

- Same sections as now. Swap them onto `base.css` components and fix anything that wraps or overflows at 360px. Remove inline `style="padding-inline…"` hacks.
- Relay keeps its current flow. The send button stays visible above the keyboard and tab bar on mobile.

### Motion

- 150–250ms ease-out for sheets, dialogs, and the selection change. Nothing animates on load beyond a short fade. All of it is disabled under `prefers-reduced-motion`.

## Left to the builder

- Exact pixel values within the `DESIGN.md` token scale.
- How the sheet drag is implemented: plain pointer events, with a tap-to-toggle fallback. No library.
- Desktop (≥900px) arrangement, as long as it is not a top nav and the mobile layout is not compromised.
- Whether `dashboard.css` and `relay.css` are separate files or sections inside `base.css`.

## Verification

- Load each page at 360×740 and 430×932 in both themes, using a local server (`PORT=8091`). There must be no wrapped labels, no overflow, and nothing hidden under the tab bar.
- Click through every function listed under the anti-goals.
- Run `go test ./modules/dashboard/...` and `go build`.
- Check the browser console for errors on each page.
