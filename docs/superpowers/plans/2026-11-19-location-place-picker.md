# Location Place Picker UX Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make creating and finding geofences clear and comfortable on desktop and mobile, with a guided map picker and adjustable radius.

**Architecture:** Keep the feature in the existing standalone live-map page. Alpine owns only primitive/form state; Leaflet layers and handlers remain in `liveMapState`. Reuse the existing live location and PIN-protected geofence endpoints, adding no geocoding provider or API.

**Tech Stack:** Embedded HTML/CSS/JavaScript, Alpine.js, Leaflet 1.9.4, existing Gin APIs.

**Spec:** `docs/superpowers/specs/2026-11-19-location-place-picker-design.md`

## Global Constraints

- Keep Leaflet map, marker, layer, and event objects outside `Alpine.data`.
- Bump the service worker `CACHE_NAME` for every dashboard frontend change.
- Preserve existing `/api/location/live` and PIN-protected geofence API contracts.
- Do not send place names or coordinates to an external geocoding/search service.
- Preserve the current mobile-first dark dashboard style and safe-area padding.

## Review Focus

- Missing device fixes: device shortcuts are disabled with clear guidance; map-center placement remains available.
- Map pan/zoom while creating: the selected point and radius preview follow the map center without interfering with ordinary map navigation outside selection mode.
- Radius slider and numeric entry: both stay synchronized, valid, keyboard operable, and show the same preview.
- Mobile and keyboard use: panel content scrolls, action buttons remain reachable, labels/focus are clear, and closing restores map interaction.
- API failures and deletion: preserve form values after a failed save, show useful feedback, and require confirmation before deleting a saved place.

---

### Task 1: Guided map-first place picker

**Files:**
- Modify: `modules/dashboard/live_map.html`
- Modify: `modules/dashboard/static/js/components/live-map.js`
- Modify: `modules/dashboard/static/css/live-map.css`

**Interfaces:**
- Consumes: `this.me`, `this.friend`, `this.newZone`, `this.pin`, `this.createGeofence()`, and `liveMapState.map` / `liveMapState.pickerLayer`.
- Produces: a three-step picker (`place`, `details`, `alerts`), map-center selection, `useDeviceLocation(device)`, synchronized slider/number radius controls, and live circle preview.

- [x] Replace the overlay form with a responsive side panel on desktop and a full-width bottom sheet on mobile. Keep the map visible and add step labels, back/next actions, close/cancel, and focusable form labels.
- [x] In selection mode, render a fixed center reticle and update `newZone.latitude`/`longitude` plus preview on map movement; outside selection mode, preserve existing map click behavior.
- [x] Add Chubu/Furturia shortcuts using latest live fixes. Disable unavailable fixes with explanatory text. Center the map at the chosen fix and update the preview.
- [x] Add a radius range input (50–2,000m, 25m steps), exact numeric input (25–100,000m), and common-value shortcuts (50, 100, 250, 500, 1,000m). Keep all controls synchronized and render slider changes on the existing Leaflet circle.
- [x] Keep name and notification choices in the guided sequence; retain the PIN and existing save API. Show saving, success, and inline error states without clearing input on failure.
- [ ] Manually verify selection, save, and cancel interactions at desktop/mobile sizes; viewport QA was not performed in this pass.

### Task 2: Scannable saved places and direct map focus

**Files:**
- Modify: `modules/dashboard/live_map.html`
- Modify: `modules/dashboard/static/js/components/live-map.js`
- Modify: `modules/dashboard/static/css/live-map.css`

**Interfaces:**
- Consumes: `this.geofences`, `liveMapState.map`, `this.deleteGeofence(id)`, and existing rendered geofence circles.
- Produces: ordered place cards and `focusGeofence(zone)` that centers/zooms the map to a selected place.

- [x] Present saved places as ordered cards with name, radius, enabled arrival/departure alerts, and coordinates; retain server alphabetical order and add an explicit empty state.
- [x] Make a card’s primary action focus its circle on the map, close any active picker, and keep the card’s delete action separate.
- [x] Require confirmation before delete and display API failure feedback inline without silently dropping the item.
- [ ] Manually verify empty, one-item, and multiple-item lists; card focus; delete confirm/cancel; and failed deletion feedback; browser QA was not performed in this pass.

### Task 3: Cache, documentation, and delivery checks

**Files:**
- Modify: `modules/dashboard/sw.js`
- Modify: `docs/spec-kit/location-tracker/setup-th.md`

**Interfaces:**
- Consumes: final picker UI and existing static asset paths.
- Produces: cache version invalidation and updated Thai usage instructions.

- [x] Bump `CACHE_NAME` in `modules/dashboard/sw.js` and keep the live-map assets in `SHELL_ASSETS`.
- [x] Update setup instructions to describe the three picker steps, device shortcuts, map-center selection, slider/number radius, and how to focus a saved place.
- [x] Run `node --check modules/dashboard/static/js/components/live-map.js` and `git diff --check`; confirm the new cache version and assets in `sw.js`. `go build ./...` also passes.
- [ ] Manually inspect the picker at 375px mobile width and desktop width, including scrolling, safe-area padding, focus order, reduced-motion behavior, and map visibility; browser QA was not performed in this pass.
