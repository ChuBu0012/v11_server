# Map Places & Settings Cleanup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The map sheet shows only people and two shortcuts. Places and Discord settings move behind a ⚙ Manage sheet. Places can be added in one sheet (my spot, friend's spot, long-press, or a pasted link) and edited. The schedule's manual form appears only when AI parsing is unavailable.

**Architecture:** On the Go side, `modules/location` gets three additions: `Store.UpdateGeofence`, `Service.UpdateGeofence`, which re-baselines geofence state silently, and a new `maplink.go` that parses coordinates and resolves short links against an allowlist. Two new routes are mounted in `Module.RegisterRoutes`. The frontend changes `live_map.html`, `live-map.css` and `live-map.js`, plus a small change in `schedule.js`.

**Tech Stack:** Go + Gin + modernc SQLite, Alpine 3.14.8, and Leaflet 1.9.4.

**Spec:** `docs/superpowers/specs/2026-11-20-map-places-design.md`

## Global Constraints

- Geofence routes use `m.requirePIN()`. Pure Go only, with no new dependencies.
- Validation stays the same as `CreateGeofence`: the name is 1–80 runes after trimming, coordinates must pass `ValidCoordinates`, and the radius must satisfy 0 < r ≤ 100000.
- A geofence edit must never emit a `NotificationEvent`.
- The resolver follows short links only over https. Every hop's host must be in {`maps.app.goo.gl`, `goo.gl`, `www.google.com`, `google.com`, `maps.google.com`, `consent.google.com`}. Allow at most 5 hops, a 6 s timeout, and read at most 64 KB of body.
- Leaflet instances stay in `liveMapState` (outside Alpine). Call `invalidateSize` after any change to the map's visible size.
- Bump `sw.js` `CACHE_NAME` to `sentinel-shell-v35`.

## Review Focus

- A Google Maps link that resolves to a URL with no coordinates (a search share) must return 422 with "ลิงก์นี้ไม่มีพิกัด ลองกดค้างบนแผนที่แทน", not a 500 or a 0,0 pin. Task 2 test: `TestResolveLinkWithoutCoordinates`.
- A redirect chain that leaves the allowlist (for example to `evil.example`) must stop before the request is made. Task 2 test: `TestResolveRejectsOffListRedirect`.
- Editing a place while the friend has no point yet must not crash the re-baseline; that device is simply skipped. Task 1 test: `TestUpdateGeofenceRebaselinesWithoutPoints`.
- A long-press while the place sheet is already open moves the point instead of opening a second sheet. Manual browser check in Task 3.
- A pinch-zoom with two fingers must not trigger a long-press. Manual browser check in Task 3, using a pointer simulation with 2 pointers.

---

### Task 1: Update geofence API with silent re-baseline

**Files:**
- Modify: `modules/location/store.go` (add `UpdateGeofence`)
- Modify: `modules/location/service.go` (add `UpdateGeofence`)
- Modify: `modules/location/handlers.go` (route + `updateGeofence` handler)
- Test: `modules/location/store_test.go`, `modules/location/service_test.go`, `modules/location/handlers_test.go`

**Interfaces:**
- Produces:
  - `func (s *Store) UpdateGeofence(ctx context.Context, id int64, input GeofenceInput) (Geofence, bool, error)`. The bool is false when the id is not found. Nil `NotifyEnter`/`NotifyExit` keep their stored values. Bump `updated_at`.
  - `func (s *Service) UpdateGeofence(ctx context.Context, id int64, input GeofenceInput) (Geofence, bool, error)`. It calls the store, then for each device in `store.LatestPoints` writes `GeofenceState{IsInside: DistanceMeters(...) <= Radius, LastTransition: now}`. It never dispatches events.
  - Route: `PUT /api/location/geofences/:id` returns 200 with the `Geofence` JSON, 404 `{"error":"ไม่พบ geofence"}`, or 400. A bad id gets 400 with `{"error":"geofence id ไม่ถูกต้อง"}`.

- [ ] **Step 1: Write the failing tests.**
  - `TestStoreUpdateGeofence`: create a place, update its name, coordinates and radius, then assert the returned and listed values. A missing id returns `found=false`. An invalid radius of 0 returns an error.
  - `TestUpdateGeofenceRebaselinesSilently`: create a place, have the friend's point inside it with state `IsInside=true`, update the place 5 km away, then process the same point again. Assert 0 events and that the stored state is `IsInside=false`. Do the reverse: move a place onto a point that was outside, then process the point again, and assert no enter event.
  - `TestUpdateGeofenceRebaselinesWithoutPoints`: update a place when there are no points. Expect no error.
  - `TestUpdateGeofenceEndpoint`: wrong PIN gives 401, the happy path gives 200 with the new name in the body, an unknown id gives 404, and a bad JSON body gives 400.
- [ ] **Step 2: Run** `go test ./modules/location/ -run 'UpdateGeofence'`. Expect FAIL (undefined: UpdateGeofence).
- [ ] **Step 3: Implement** the store method, the service method and the handler (route registered after the POST).
- [ ] **Step 4: Run** `go test ./modules/location/`. Expect PASS.
- [ ] **Step 5: Commit** with `feat(location): edit geofences with silent state re-baseline`.

### Task 2: Map link parser and short-link resolver

**Files:**
- Create: `modules/location/maplink.go`, `modules/location/maplink_test.go`
- Modify: `modules/location/handlers.go` (add a `linkResolver *LinkResolver` field, set in `NewModule` with `NewLinkResolver(nil)`, plus route and handler)

**Interfaces:**
- Produces:
  - `func ParseMapCoordinates(raw string) (lat, lng float64, ok bool)`. It recognizes, in order:
    - a plain `lat, lng` pair (comma, optional spaces)
    - `!3dLAT!4dLNG`
    - `@LAT,LNG`
    - the query params `q`, `query`, `ll`, `destination`, `center` holding `lat,lng`
    
    It rejects |lat| > 90 and |lng| > 180.
  - `type LinkResolver struct{ client *http.Client }` and `func NewLinkResolver(client *http.Client) *LinkResolver`. When client is nil, it builds one with a 6 s timeout and a `CheckRedirect` that enforces the allowlist, https, and at most 5 hops.
  - `func (r *LinkResolver) Resolve(ctx context.Context, raw string) (lat, lng float64, err error)`. It tries `ParseMapCoordinates` first. If the raw host is not `maps.app.goo.gl`, or `goo.gl` with a path starting `/maps`, it returns `ErrLinkNotSupported`. Otherwise it issues a GET and parses the final URL, then the first 64 KB of the body (a fallback for `@lat,lng`). If nothing is found it returns `ErrLinkNoCoordinates`.
  - Route: `POST /api/location/resolve-link` (`requirePIN`) with body `{"url": string}` (max 2 KB). It returns 200 `{"latitude","longitude"}`. `ErrLinkNoCoordinates` gives 422 with "ลิงก์นี้ไม่มีพิกัด ลองกดค้างบนแผนที่แทน". `ErrLinkNotSupported` and bad input give 400 with "รองรับเฉพาะลิงก์ Google Maps หรือพิกัด lat, lng". A network error gives 502 with "เปิดลิงก์ไม่สำเร็จ ลองใหม่อีกครั้ง".
  - Tests inject the client through an unexported `allowAnyHostForTest` option, or by building a `LinkResolver` with a client whose transport rewrites requests to an `httptest.Server`. The allowlist check must stay active in the redirect test.

- [ ] **Step 1: Write the failing tests.**
  - `TestParseMapCoordinates` is table-driven. The `ok` cases:
    - `"13.7563, 100.5018"`
    - `https://www.google.com/maps/place/X/@13.7563,100.5018,17z`
    - `.../data=!3d13.7563!4d100.5018`
    - `?q=13.7563,100.5018`
    - `?api=1&destination=13.7563,100.5018`
    
    The not-ok cases: `"hello"`, `"91,10"`, `"10,181"`, `?q=Siam+Paragon`.
  - `TestResolveLinkWithoutCoordinates`: a short link whose final URL is `...?q=Siam+Paragon` returns `ErrLinkNoCoordinates`.
  - `TestResolveRejectsOffListRedirect`: a hop to `https://evil.example/` returns an error, and the evil server records 0 hits.
  - `TestResolveRejectsUnsupportedHost`: `https://example.com/x` returns `ErrLinkNotSupported` without any request being made.
  - `TestResolveLinkEndpoint`: wrong PIN gives 401, plain coordinates give 200, and a non-Google host gives 400.
- [ ] **Step 2: Run** `go test ./modules/location/ -run 'MapCoordinates|Resolve'`. Expect FAIL.
- [ ] **Step 3: Implement** `maplink.go` and the handler.
- [ ] **Step 4: Run** `go test ./modules/location/ && go vet ./modules/location/`. Expect PASS.
- [ ] **Step 5: Commit** with `feat(location): resolve Google Maps links to coordinates`.

### Task 3: Map UI — controls column, Manage sheet, place sheet, shortcuts

**Files:**
- Modify: `modules/dashboard/live_map.html`, `modules/dashboard/static/css/live-map.css`, `modules/dashboard/static/js/components/live-map.js`
- Test: `modules/dashboard/dashboard_test.go` (`TestLiveMapPageIsEmbeddedAndResponsive`)

**Interfaces:**
- Consumes `PUT /api/location/geofences/:id` and `POST /api/location/resolve-link` from Tasks 1–2, using the same `X-Relay-PIN` header that `createGeofence` uses today.
- New Alpine state, all primitives:
  - `manageOpen: false`, `manageTab: 'places'`
  - `placeOpen: false`, `placeEditId: null`
  - `linkInput: ''`, `linkBusy: false`
- `newZone` keeps its shape.
- Removed: `pickingLocation`, `pickerStep`, `pickerError` (→ `placeError`), `nextPickerStep`, `previousPickerStep`, `pickerStepTitle`, `placeSuggestions`, `setPlaceSuggestion`, `startMapPicker`, `cancelMapPicker`, `toggleMapPicker`.
- New methods:
  - `openManage(tab)`, `closeManage()`
  - `openPlace(zone = null, latLng = null)`, `closePlace()`
  - `setPlacePoint(lat, lng)`: updates `newZone`, then moves the preview marker and circle
  - `useDevicePoint('me'|'friend')`, `applyLink()`
  - `savePlace()`: POST or PUT, then `refresh()`
  - `removePlace()`: reuses `deleteGeofence`
  - `navigateToFriend()`, `copyFriendCoords()`
- Long-press is wired in `init()` once the map exists. On the container: `pointerdown` starts a 500 ms timer when it is the only active pointer. `pointermove` with more than 10 px of travel, `pointerup`, `pointercancel`, or a second `pointerdown` clears the timer. When it fires, call `openPlace(null, latlng)`, or `setPlacePoint` if `placeOpen`. Also `map.on('contextmenu', e => …)` does the same.
- The preview marker is `liveMapState.placeMarker`, an `L.marker` with `draggable: true`. Its `dragend` calls `setPlacePoint`. The radius circle is `liveMapState.placeCircle`. Both live in `pickerLayer`.
- `body.is-picking` binds to `placeOpen`. CSS keeps the shrink-map rule and removes the reticle element.

- [ ] **Step 1: Update the test.**
  - The want list adds `class="manage-sheet`, `class="place-sheet`, `map-live-btn`, `navigateToFriend()`, `copyFriendCoords()`, `applyLink()` and `theme-toggle-btn`.
  - The must-not list adds `picker-step-list`, `picker-suggestion-chips`, `class="map-status"` and `picker-reticle`.
- [ ] **Step 2: Run** `go test ./modules/dashboard/ -run TestLiveMap`. Expect FAIL.
- [ ] **Step 3: Implement the markup, CSS and JS** as described in the spec's "Design — map UI" section.
  - Top-right column order: live dot button, theme, ⚙, show both, trail, then zoom inside `.hover-only`.
  - Attribution moves to bottomleft with CSS `bottom: calc(var(--sheet-peek) + var(--nav-h) + 8px)`.
  - The people sheet gets a shortcut row with `[นำทาง] [คัดลอกพิกัด]`, both disabled when `!friend`.
  - Navigate URL: `https://www.google.com/maps/dir/?api=1&destination=${lat},${lng}`, opened with `window.open(url, '_blank', 'noopener')`.
  - Copy uses `navigator.clipboard.writeText`, falling back to a hidden textarea and `execCommand('copy')`. Show the toast "คัดลอกพิกัดแล้ว".
- [ ] **Step 4: Run** `go test ./modules/dashboard/`. Expect PASS.
- [ ] **Step 5: Browser check** at 390px, dark and light, against the local server on :8091 with the Relay PIN from `.env` typed into the PIN field.
  - Open ⚙. Tabs switch, toggles work.
  - Add a place via "ตรงที่ฉันอยู่", after injecting `me` via Alpine.
  - Add via a long-press simulation, and via a pasted full link.
  - Drag the marker, then save.
  - Edit an existing place's name, radius and point.
  - Delete a place.
  - A wrong PIN shows its error inside the sheet.
  - A long-press while the sheet is open moves the point.
  - A two-pointer simulation does not open anything.
  - Navigate and copy are disabled while there is no friend point, and work after injecting `friend`.
  - The console shows no errors.
- [ ] **Step 6: Commit** with `feat(map): manage sheet, one-step place editor, long-press and link input`.

### Task 4: Schedule — manual form only when AI is unavailable

**Files:**
- Modify: `modules/dashboard/schedule.html`, `modules/dashboard/static/js/components/schedule.js`, `modules/dashboard/static/css/schedule.css`
- Test: `modules/dashboard/dashboard_test.go` (`TestScheduleUsesMonthPlusDayLayout`)

**Interfaces:**
- New element: `<p id="nl-fallback-note" class="alert" hidden>AI ใช้งานไม่ได้ตอนนี้ — กรอกเองได้เลย</p>`, placed directly above `#reminder-form` in the sheet.
- `#nl-or` is removed.
- `openForm(null)`: shows the NL block, hides `#reminder-form` and `#nl-fallback-note`.
- `openForm(reminder)`: hides the NL block and shows the form.
- `parseNLPrompt`:
  - If `data.fallback === true`, or an error is caught, call `showManualFallback(prompt)`. That shows the note and the form, sets `#reminder-title` to the prompt, and focuses it.
  - Otherwise show the preview card as today.
- `openManualFormFromNL`: shows the form without the note.
- `showManualForm(show)` is replaced by `setManualVisible(show: boolean, withNote: boolean)`.

- [ ] **Step 1: Update the test.** Want `id="nl-fallback-note"`; must-not `id="nl-or"`.
- [ ] **Step 2: Run** `go test ./modules/dashboard/ -run TestScheduleUsesMonthPlusDayLayout`. Expect FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `go test ./modules/dashboard/`. Expect PASS.
- [ ] **Step 5: Browser check.**
  - Opening + shows only the AI input.
  - Parsing with no OpenRouter key locally (fallback) reveals the note and the form, with the title prefilled.
  - Editing an existing reminder shows the form directly.
  - Saving works with the local test PIN.
  - The console shows no errors.
- [ ] **Step 6: Commit** with `feat(schedule): show manual reminder form only when AI parsing is unavailable`.

### Task 5: Cache bump, full verification, push

- [ ] **Step 1: Bump** `modules/dashboard/sw.js` to `sentinel-shell-v35`.
- [ ] **Step 2: Run** `go test ./... && go vet ./... && go build -o /dev/null .`. Expect all to pass.
- [ ] **Step 3: Commit** with `chore(pwa): bump shell cache v35`, then run `git push origin main`.
