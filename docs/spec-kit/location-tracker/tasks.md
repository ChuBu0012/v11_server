# Location & Rendezvous Tracker — Tasks & Evidence of Done

**Status key:** `TODO` → `IN PROGRESS` → `BLOCKED` / `DONE`

---

## T001 — Database schemas, models, and configuration

- **Status:** DONE
- **Evidence:** `GOCACHE=/tmp/v11-gocache go test ./modules/location ./shared/config ./shared/db` → PASS; new database schema test confirms location tables and blank token columns rather than predictable defaults.
- **Scope:**
  - Define Go structs for OwnTracks payload (`LocationReport`), DB entities (`LocationPoint`, `Geofence`, `GeofenceState`, `CoLocationState`, `LocationSettings`).
  - Add `LOCATION_SECRET_TOKEN` and `LOCATION_VIEWER_TOKEN` to `shared/config/config.go` with secret masking.
  - Implement SQLite migrations in `shared/db/db.go` for `location_points`, `location_geofences`, `location_geofence_states`, `location_colocation_state`, and `location_settings`.
- **Target Artifacts:**
  - `modules/location/models.go`
  - `shared/config/config.go`
  - `shared/db/db.go`
  - `.env.example`
- **Acceptance Criteria:**
  - `InitDB()` runs migrations successfully on existing SQLite without table collision or CGO dependencies.
  - Configuration loads tokens from environment variables with fallback defaults.
  - Unit tests in `shared/db` and `shared/config` pass 100%.

---

## T002 — Spatial calculation math (Haversine engine)

- **Status:** DONE
- **Evidence:** Haversine, coordinate boundaries (including NaN/Inf), formatting tests pass; benchmark on Apple M4: `42.06 ns/op` (`go test -run '^$' -bench BenchmarkDistanceMeters -benchtime=1s ./modules/location`).
- **Scope:**
  - Implement pure Go Haversine formula calculation for geodesic distance between coordinates $(lat_1, lon_1)$ and $(lat_2, lon_2)$ in meters.
  - Implement coordinate boundary validation (latitude $[-90, 90]$, longitude $[-180, 180]$).
  - Implement bearing / heading calculation if needed.
- **Target Artifacts:**
  - `modules/location/math.go`
  - `modules/location/math_test.go`
- **Acceptance Criteria:**
  - Precision within 0.1% of official geodesic reference distances.
  - Benchmark runs in sub-microsecond speeds per calculation (< 500ns).
  - Comprehensive unit test coverage covering poles, equator, prime meridian, and anti-meridian.

---

## T003 — SQLite persistence store & retention lifecycle

- **Status:** DONE
- **Evidence:** `GOCACHE=/tmp/v11-gocache go test ./modules/location ./shared/db` → PASS; tests cover credentials, point latest/breadcrumbs/validation, geofence/state CRUD, settings patch, and pruning path.
- **Scope:**
  - CRUD operations for location points, geofences, and state persistence.
  - Query to fetch last-known location per device (`me` and `friend`).
  - Query to fetch 24-hour historical breadcrumb points for trail rendering.
  - Automated 7-day retention cleanup job to prevent storage bloat on Android.
- **Target Artifacts:**
  - `modules/location/store.go`
  - `modules/location/store_test.go`
- **Acceptance Criteria:**
  - Insertion and query methods execute within bounded timeouts.
  - 7-day cleanup prunes old records without table locks in WAL mode.

---

## T004 — Co-location & Geofence evaluation engine

- **Status:** DONE
- **Evidence:** `GOCACHE=/tmp/v11-gocache go test ./modules/location` → PASS; meeting/separation thresholds, GPS jitter, cooldown, geofence enter/exit, battery cooldown and recovery reset are exercised.
- **Scope:**
  - Implement stateful spatial engine in `service.go`.
  - Hysteresis logic: Enter co-location at $\le 80$m; Exit co-location at $\ge 250$m.
  - Cooldown logic: 5-minute suppression window between transitions to eliminate GPS jitter.
  - Geofence enter/exit transition detection per device.
  - Battery critical evaluation ($\le 15\%$ and unplugged).
- **Target Artifacts:**
  - `modules/location/service.go`
  - `modules/location/service_test.go`
- **Acceptance Criteria:**
  - Test suite simulates approaching, jittering around threshold, departing, and cooldown timing.
  - Zero duplicate alerts during rapid coordinate jitter.

---

## T005 — Discord notification dispatcher

- **Status:** DONE
- **Evidence:** `GOCACHE=/tmp/v11-gocache go test ./modules/location` → PASS; all five templates, ICT timestamps, Markdown escaping, mention suppression, and async delivery are covered.
- **Scope:**
  - Build formatted Discord notifications with emojis, bold headers, and Bangkok ICT local timestamps.
  - Support Co-location alert (with geofence place name if inside one).
  - Support Geofence enter/exit alert.
  - Support Critical battery warning.
  - Respect user toggle switches in `location_settings`.
- **Target Artifacts:**
  - `modules/location/notifier.go`
  - `modules/location/notifier_test.go`
- **Acceptance Criteria:**
  - Renders valid Discord Markdown (< 1950 chars).
  - Dispatches asynchronously without blocking the location ingress HTTP response.

---

## T006 — Ingress and management API endpoints

- **Status:** DONE
- **Evidence:** `GOCACHE=/tmp/v11-gocache go test ./modules/location ./...` → PASS; handler tests cover OwnTracks `200 []` ACK, bad credentials/payload, future timestamp rejection, viewer token isolation, PIN-protected management, and main route registration. Service ignores duplicate/out-of-order samples so queued reports cannot roll back geofence state.
- **Scope:**
  - `POST /api/location/report`: Ingests OwnTracks HTTP JSON payload, validates token and device ID (`me` | `friend`), saves point, triggers service evaluation.
  - `GET /api/location/live`: Returns latest points, distance, geofences, and settings (token-guarded).
  - `GET /api/location/geofences`, `POST /api/location/geofences`, `DELETE /api/location/geofences/:id`: Geofence management (PIN-guarded).
  - `PATCH /api/location/settings`: Notification toggle switches (PIN-guarded).
  - Route registration in `main.go`.
- **Target Artifacts:**
  - `modules/location/handlers.go`
  - `modules/location/handlers_test.go`
  - `main.go`
- **Acceptance Criteria:**
  - OwnTracks HTTP client receives HTTP 200 `[]` response (standard OwnTracks empty ACK).
  - Unauthorized requests return HTTP 401.

---

## T007 — Responsive Public Web Map Viewer (`/live/:token`)

- **Status:** DONE
- **Evidence:** Viewer, Leaflet state outside Alpine, polling, geofences, trails, settings drawer, no-store responses, and cache v17 are implemented; dashboard tests, `node --check modules/dashboard/static/js/components/live-map.js`, and `git diff --check` → PASS. Device/browser visual verification remains part of T008 rollout.
- **Scope:**
  - Create standalone mobile-friendly Web Viewer at `/live/:token`.
  - Leaflet.js interactive map with CartoDB Dark Matter / OSM tiles.
  - Render dual pins (`me` in blue, `friend` in emerald) with distance connector line.
  - Live telemetry floating card: speed, battery %, last updated relative time.
  - Geofence circular overlays on map.
  - Breadcrumb historical trail toggle.
  - Bump Service Worker cache version in `modules/dashboard/sw.js`.
- **Target Artifacts:**
  - `modules/dashboard/live_map.html`
  - `modules/dashboard/static/js/components/live-map.js`
  - `modules/dashboard/sw.js`
- **Acceptance Criteria:**
  - Responsive on Android and iOS mobile viewports.
  - Leaflet instance strictly maintained outside Alpine reactive Proxy.
  - Auto-polls live data every 10–15 seconds smoothly.

---

## T008 — End-to-end verification and on-device deployment

- **Status:** TODO
- **Evidence:** Local full suite `GOCACHE=/tmp/v11-gocache go test ./...`, `go build ./...`, `go vet ./...`, and `CGO_ENABLED=0 GOOS=android GOARCH=arm64 GOCACHE=/tmp/v11-gocache go build -o /tmp/v11-location-server .` → PASS. Physical deployment, Discord delivery, and live public-link verification are not run from this implementation pass.
- **Scope:**
  - Run full test suite: `go test ./...`.
  - Cross-compile ARM64 Android Go binary.
  - Deploy to vivo V11 via `deploy-v11.sh` and reload PM2.
  - Send mock OwnTracks location payload for both devices via `curl` to verify Co-location trigger and Discord notification.
  - Verify public web link `/live/:viewer_token` on public ngrok domain.
- **Acceptance Criteria:**
  - Memory consumption on vivo V11 remains well within < 300MB RAM budget.
  - Discord receives test meeting alert with distance and battery metrics.
  - Web viewer displays real-time pins accurately.
