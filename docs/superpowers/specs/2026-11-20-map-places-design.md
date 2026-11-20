# Map Places & Settings Cleanup — Design

Date: 2026-11-20 · Follows `2026-11-20-mobile-first-redesign-design.md` (map layout A).

## Intent and why

The owner opens the map to check where the friend is. Places (geofences) and Discord settings are edited rarely, but today they take up most of the expanded sheet. Two concrete problems:

- Adding a place is awkward. The wizard has 3 steps and you have to pan the map under a reticle.
- A saved place cannot be edited at all. The server has no update endpoint, so the only way to fix a pin is delete-and-recreate.

The owner adds places in two situations, and both must be quick:

- Standing at the place.
- Remembering a place later and setting it from somewhere else.

They asked for simplicity throughout.

The same round also asks for one schedule change: hide the manual reminder form by default and show it only when AI parsing is unavailable.

## Goals

1. **Sheet shows people only.** It keeps the friend headline, device details, the ping log, and two shortcuts:
   - **Navigate to Furturia** opens Google Maps directions to the friend's latest coordinates.
   - **Copy coordinates** copies the friend's `lat,lng` and shows a toast.
2. **Status bar becomes an icon.** A round 44px glass button with a coloured dot: green when connected, amber while waiting. Tapping it refreshes. The "updated" time already appears in the friend sub-line.
3. **Gear button ⚙** goes in the floating controls column (top-right, above "show both"). It opens a **Manage sheet** with two tabs:
   - **Places:** the list, plus a "+ เพิ่มสถานที่" button.
   - **Notifications:** the three Discord toggles and the Relay PIN field.
4. **One add/edit place sheet** replaces the 3-step wizard (layout sketch below).
5. **Ways to set the point:**
   - **Long-press on the map** (≥ 500 ms touch, or right-click on desktop) opens the add sheet at that point.
   - **"ตรงที่ฉันอยู่"** uses the owner's latest point. **"ตรงที่ Furturia อยู่"** uses the friend's latest point.
   - **Paste a Google Maps link** in the link field. This accepts full URLs, `maps.app.goo.gl` / `goo.gl/maps` short links, and plain `lat, lng` text.
   - **Drag the preview pin** on the map to fine-tune. The map stays visible above the sheet.
6. **Edit place.** Tap a place in the list to open the same sheet with values filled in, plus a "ลบ" button.
7. **Schedule add sheet.** The manual form is hidden by default. If `/api/schedule/parse` returns `fallback: true` or errors, the sheet shows a notice "AI ใช้งานไม่ได้ตอนนี้ — กรอกเองได้เลย" and reveals the manual form, prefilled with the typed text as the title. Editing an existing reminder still opens the manual form directly. The preview's "แก้ไขในแบบฟอร์มปกติ" link stays.

Add/edit place sheet layout:

```
เพิ่มสถานที่ / แก้ไขสถานที่                      ✕
[◎ ตรงที่ฉันอยู่] [◎ ตรงที่ Furturia อยู่]
[ วางลิงก์ Google Maps หรือ lat, lng      ]
พิกัด 13.65160, 100.49390 · ลากหมุดเพื่อปรับ
ชื่อ   [               ]
รัศมี  ──●────  150 ม.   (50–2000, step 25)
แจ้งเมื่อ  ☑ มาถึง  ☑ ออกไป
[ PIN ] (only when no PIN is saved)
[ ลบ ] (edit only)            [   บันทึก   ]
```

## Non-goals

- No place-name search or geocoding service, and no suggestion chips. The 🏠/💼 chips are removed.
- No "Furturia อยู่ที่ <place>" headline.
- No changes to how enter/exit or co-location notifications are decided, apart from the edit rule below.

## Anti-goals

- An edit that triggers a Discord enter/exit notification even though nobody moved.
- The link resolver being usable to make the server fetch arbitrary URLs (SSRF).
- Losing any existing function: create, delete, Discord toggles, Relay PIN, trail, show both.

## Constraints

- Leaflet objects stay outside Alpine. Call `invalidateSize` after any change to the map's visible size.
- Geofence routes keep the existing `requirePIN()` (Relay PIN).
- Pure-Go only, with no new dependencies.
- Bump `sw.js` `CACHE_NAME`.

## Approaches considered

1. **Gear opens a Manage bottom sheet with tabs (chosen by owner).** Rarely used settings stay one tap away, and long-press adds a place without going through the menu.
2. **A separate `/live/settings` page.** This needs token-gated routing and moving between pages.
3. **Collapsible sections inside the existing sheet.** This is the least work, but the sections stay in the way, which is what the owner wants to avoid.

## Design — server

- **`PUT /api/location/geofences/:id`** (`requirePIN`). It takes the same `GeofenceInput` body and validation as create, and returns `200` with the updated `Geofence`, `404` if the place is not found, and `400` on bad input.
  - New `Store.UpdateGeofence(ctx, id, input) (Geofence, bool, error)`.
  - After a successful update, the service **re-baselines silently**. For each device with a latest point, it recomputes `inside` against the new centre and radius, then writes `GeofenceState` with `LastTransition = now`. It emits **no** notification events. (Claude's call, prevents edit-triggered alerts.)
- **`POST /api/location/resolve-link`** (`requirePIN`), body `{"url": "..."}`, returns `{latitude, longitude}` or a Thai error message.
  1. Parse locally first: `@lat,lng`, `!3dLAT!4dLNG`, `q=`/`query=`/`ll=`/`destination=lat,lng`, or plain `lat, lng`.
  2. Only if that fails and the host is exactly `maps.app.goo.gl` or `goo.gl` (path `/maps…`) over https, follow redirects. Allow at most 5 hops, and every hop's host must be one of `maps.app.goo.gl`, `goo.gl`, `www.google.com`, `google.com`, `maps.google.com`, or `consent.google.com`. Use a 6 s timeout and read the body only up to 64 KB, then parse the final URL with the same patterns.
  3. Reject latitudes outside ±90 and longitudes outside ±180.
  4. Some share links (search results) contain only a place name and no coordinates. In that case, return `422` with "ลิงก์นี้ไม่มีพิกัด ลองกดค้างบนแผนที่แทน". (Claude's call.)
  - Plain `lat, lng` and full links are also parsed client-side, so they never hit the server. Only short links call the resolver.

## Design — map UI

- Remove the `.map-status` pill. Add `.map-live-btn` (a dot icon) as the first item of the top-right control column, with the theme toggle next to it in a small top row. Leaflet attribution goes back to bottom-left above the sheet.
- Control column order: live/refresh, ⚙ manage, show both, trail, then zoom (desktop only).
- **Manage sheet:** `aside.manage-sheet` (Alpine flag `manageOpen`) uses a segmented control with `manageTab` set to `'places'` or `'notify'`. It replaces the people sheet while open and closes with ✕ or Esc.
- **Place sheet:** `aside.place-sheet` (flags `placeOpen` and `placeEditId`, where null means add). While it is open, the map canvas shrinks above it (reusing `body.is-picking`). A draggable Leaflet marker together with the radius circle previews the point, and dragging updates `newZone.latitude` and `newZone.longitude`.
- **Long-press:** iOS Safari does not fire `contextmenu` on long-press, so use our own pointer-event timer on the map container. It fires after 500 ms, is cancelled if the pointer moves more than 10 px, on `pointerup`, or when a second pointer touches (pinch), and converts the point with `map.containerPointToLatLng`. Also listen for Leaflet's `contextmenu` event for desktop right-click. While the place sheet is open, a long-press moves the point instead of opening a new sheet. (Claude's call.)
- The people sheet adds a shortcut row with two buttons, "นำทาง" and "คัดลอกพิกัด". Both are disabled while there is no friend point. Navigate opens `https://www.google.com/maps/dir/?api=1&destination=LAT,LNG` in a new tab.
- Delete the old wizard state: `pickerStep`, `nextPickerStep`, `previousPickerStep`, `pickerStepTitle`, and `placeSuggestions`.

## Left to the builder

- Whether the Manage sheet keeps its last tab between openings (default: Places).

## Verification

- Go tests:
  - `PUT` happy path, plus 404, 400, and wrong PIN.
  - Re-baseline produces no events: create a place, record a point inside it, move the place away, record the same point again, and expect no exit event. Also the reverse case.
  - The link parser handles each URL pattern.
  - The resolver rejects non-Google hosts and does not follow a redirect to an off-list host. Use a test server with an injected HTTP client.
- Browser at 390px, dark and light:
  - Add a place via "ตรงที่ฉันอยู่", via long-press, and via a pasted full link.
  - Edit name, radius, and point by dragging, then save. The list updates.
  - Delete a place.
  - Turn the Discord toggles on and off.
  - Navigate and copy shortcuts work. The console shows no errors.
- Schedule: AI fallback reveals the manual form prefilled. With the AI preview path, the manual form stays hidden.
