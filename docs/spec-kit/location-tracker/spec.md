# Pocket Sentinel — Location & Rendezvous Tracker Specification

**Status:** Draft for approval  
**Target:** vivo V11 / Android 10 / Termux ARM64  
**Primary Clients:** 2 Android devices running OwnTracks (HTTP Mode)  
**Primary Notification Channel:** Discord Webhook  
**Public Map View:** `/live/:token` (Responsive Leaflet Map)

---

## 1. Intent

Location & Rendezvous Tracker provides private, continuous 24/7 background location tracking and proximity awareness between two specific individuals (Owner and Friend), reminiscent of Life360 but completely self-hosted, privacy-first, and without recurring subscription fees or third-party cloud data harvesting.

The system ingests location beacons from the free, open-source **OwnTracks** Android app via HTTP, evaluates spatial relationships (Geofences and Co-location distance), detects critical battery levels, dispatches actionable notifications to **Discord**, and provides a responsive, beautiful real-time Web Map Viewer accessible via a secure Public Link.

---

## 2. User Decisions & Requirements

- **Client Application:** Both individuals run **OwnTracks** on Android in HTTP mode with significant motion tracking to preserve device battery life 24/7.
- **Device Separation:** Exactly two tracked devices: `me` (Owner) and `friend` (Friend).
- **Ingress Protocol:** HTTP POST with JSON payload conforming to OwnTracks location report format (`_type: "location"`).
- **Proximity / Co-location Detection:**
  - **Enter Threshold ("พบกันแล้ว"):** Triggers when distance between both devices $\le 80$ meters.
  - **Exit Threshold ("แยกย้ายแล้ว"):** Triggers when distance $\ge 250$ meters (Hysteresis prevents spam and GPS jitter while sitting in the same venue).
  - **Cooldown:** At least 5 minutes between state transitions.
  - **Contextual Place Enrichment:** If co-location occurs within a registered Geofence, the alert names the place (e.g. "สยามพารากอน"); otherwise, it provides reverse geocode or concise coordinates.
- **Geofencing:**
  - Configurable circular zones (latitude, longitude, radius in meters).
  - Triggers enter/exit notifications per device to Discord.
- **Battery Monitoring:**
  - Alerts Discord when any device battery drops $\le 15\%$ with charging state indicator, throttled to once per 2 hours unless charged.
- **Notification Destination:** **Discord Webhook** (using Discord Markdown, emojis, and embedded status badges).
- **Notification Toggles:** Independent on/off switches for Co-location, Geofence, and Battery alerts manageable via Dashboard.
- **Public Viewer:**
  - Web-based map accessible via `/live/:token` without requiring login credentials.
  - Displays both pins, dynamic distance connecting line, battery status, speed, last seen timestamp, active geofence circles, and breadcrumb path history.

---

## 3. Goals

- **Zero App Development:** Leverage existing open-source Android clients (OwnTracks) to achieve true 24/7 background location updates without writing or maintaining custom native Android apps.
- **Low Power & Resource Footprint:** Run seamlessly on the vivo V11 pocket server within the project's < 300MB RAM budget (pure Go, SQLite WAL mode, < 15MB active footprint).
- **Robust Math & Spatial Logic:** Deterministic Haversine distance calculations in Go with microsecond execution times.
- **Noise-Free Notifications:** Anti-spam hysteresis and cooldowns guarantee Discord receives only meaningful, actionable life events.
- **Secure Public Link:** Tokenized URL prevents unauthorized enumeration while granting immediate browser-based visibility to authorized parties.
- **Data Retention & Hygiene:** Automated pruning of raw location pings older than 7 days to preserve SQLite storage on the pocket device.

---

## 4. Non-Goals

- General multi-tenant SaaS tracking or fleet management (strictly optimized for 2 devices: `me` and `friend`).
- Driving turn-by-turn navigation or routing directions.
- Video, audio, or crash sensor streaming.
- Native mobile app build/compilation requirements (relies exclusively on standard OwnTracks client and Web UI).

---

## 5. Approaches Considered & Trade-offs

| Approach | Feasibility on Mobile OS | Pros | Cons / Verdict |
|---|---|---|---|
| **1. Pure Web/PWA Geolocation via Public Link** | ❌ Fails on background | Zero app installation required. | **Rejected:** Android/iOS kills background JavaScript when the screen is locked or browser is minimized. Cannot deliver Life360 24/7 background tracking. |
| **2. Custom Native App (Flutter/React Native)** | ⚠️ High cost | Full visual control over mobile UI. | **Rejected:** High maintenance, Play Store developer fee/review, complex background service tuning for battery life. Overkill for 2 users. |
| **3. OwnTracks + Pocket Sentinel + Discord (Selected)** | ✅ 100% Proven | 24/7 background tracking, zero custom app code, battery optimized, private SQLite storage, rich Discord notifications. | **Selected:** Optimal balance of reliability, speed to deploy, and user experience. |

---

## 6. Functional Design

### 6.1 OwnTracks Ingress Endpoint

- **Endpoint:** `POST /api/location/report`
- **Query Params / Headers:**
  - Query: `?device=me|friend&token=LOCATION_SECRET_TOKEN`
  - Or Header: `X-Location-Token: LOCATION_SECRET_TOKEN`
- **Payload (OwnTracks Location Report):**
  ```json
  {
    "_type": "location",
    "lat": 13.7462,
    "lon": 100.5348,
    "acc": 15,
    "tst": 1700394000,
    "batt": 45,
    "bs": 1,
    "vel": 24,
    "alt": 18,
    "cog": 180
  }
  ```
- **Field Definitions:**
  - `lat`, `lon`: Latitude and Longitude (WGS84).
  - `acc`: Accuracy in meters (values > 100m are flagged or filtered for co-location calculation).
  - `tst`: Unix epoch timestamp when location was recorded by device.
  - `batt`: Battery percentage (0–100).
  - `bs`: Battery status (0=unknown, 1=unplugged, 2=charging, 3=full).
  - `vel`: Velocity / speed in km/h.
  - `alt`: Altitude in meters.
  - `cog`: Course over ground (bearing 0–359°).

### 6.2 Spatial Evaluation & State Machine

#### A. Co-location & Rendezvous State Machine
- Calculates distance $D = \text{Haversine}(\text{loc}_{\text{me}}, \text{loc}_{\text{friend}})$.
- State: `is_co_located` (boolean), `last_transition_at` (time), `last_notified_at` (time).
- **Rule 1 (Meeting):** If `!is_co_located` AND $D \le 80\text{m}$ AND $(\text{now} - \text{last\_transition\_at} \ge 300\text{s})$:
  - Update `is_co_located = true`.
  - Check if location matches any active Geofence.
  - Dispatch Discord meeting notification.
- **Rule 2 (Separation):** If `is_co_located` AND $D \ge 250\text{m}$ AND $(\text{now} - \text{last\_transition\_at} \ge 300\text{s})$:
  - Update `is_co_located = false`.
  - Dispatch Discord separation notification.

#### B. Geofence Enter / Exit State Machine
- For each active Geofence $G$:
  - Calculate distance $d = \text{Haversine}(\text{loc}_{\text{device}}, G_{\text{center}})$.
  - Current condition: $\text{inside} = (d \le G_{\text{radius}})$.
  - If previous state was outside and now inside $\rightarrow$ Trigger **Enter Alert**.
  - If previous state was inside and now outside $\rightarrow$ Trigger **Exit Alert**.

#### C. Battery Critical State Machine
- Triggered when `batt <= 15` AND previous alert was sent $> 2$ hours ago (or state reset after battery recovers $> 25\%$).

---

### 6.3 Discord Message Templates

All messages format through `modules/alerts/discord.go` with emojis, bold headers, and timestamps in Bangkok ICT (UTC+7):

1. **Co-location Meeting Alert:**
   ```markdown
   🤝 **พบกันแล้ว!**
   • **สถานที่:** สยามพารากอน (ระยะห่าง 35 ม.)
   • **พิกัด:** `13.7462, 100.5348`
   • **เวลา:** 14:15:20 ICT
   • **แบตเตอรี่:** คุณ 85% ⚡ · เพื่อน 62%
   ```

2. **Co-location Separation Alert:**
   ```markdown
   👋 **แยกย้ายกันแล้ว**
   • **ระยะห่างล่าสุด:** 310 เมตร
   • **เวลา:** 16:45:10 ICT
   ```

3. **Geofence Enter Alert:**
   ```markdown
   🟢 **ถึงที่หมายแล้ว**
   • **บุคคล:** เพื่อน
   • **สถานที่:** บ้านเพื่อน (รัศมี 150 ม.)
   • **เวลา:** 18:30:15 ICT
   • **แบตเตอรี่:** 48%
   ```

4. **Geofence Exit Alert:**
   ```markdown
   🟠 **ออกจากสถานที่**
   • **บุคคล:** เพื่อน
   • **สถานที่:** ที่ทำงาน
   • **ความเร็ว:** 42 km/h
   • **เวลา:** 17:15:00 ICT
   ```

5. **Battery Critical Alert:**
   ```markdown
   🪫 **แจ้งเตือนแบตเตอรี่ต่ำ**
   • **บุคคล:** เพื่อน
   • **ระดับแบตเตอรี่:** `12%` (ยังไม่ได้เสียบชาร์จ)
   • **เวลา:** 21:05:30 ICT
   ```

---

### 6.4 Public Web Map Viewer (`/live/:token`)

- **Routing:** `GET /live/:token` serves the responsive standalone viewer.
- **API Backend:** `GET /api/location/live?token=:token` returns latest coordinates of both devices, geofences, and recent breadcrumb points.
- **Frontend Components:**
  - **Map View:** Leaflet.js with CartoDB Dark Matter / OSM tiles.
  - **Pins:**
    - Blue pulse pin for `me` (คุณ).
    - Violet/Emerald pin for `friend` (เพื่อน).
    - Custom connecting dashed polyline showing live distance in meters or kilometers.
    - Geofence transparent circular overlays.
  - **Live Status Header / Floating Bar:**
    - Speed (km/h) with moving/stationary badge.
    - Battery percentage badge with charging lightning icon.
    - Relative time ("อัปเดตเมื่อ 15 วินาทีที่แล้ว").
    - Relative proximity status ("กำลังอยู่ด้วยกัน" หรือ "ห่างกัน 1.2 กม.").
  - **Settings Drawer (Protected by PIN):**
    - Notification toggle switches.
    - Geofence management (Add/Delete/Adjust radius).

---

## 7. Data Models & Database Schemas (SQLite)

```sql
-- 1. Historical location pings (7-day retention)
CREATE TABLE IF NOT EXISTS location_points (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    device_id TEXT NOT NULL, -- 'me' or 'friend'
    latitude REAL NOT NULL,
    longitude REAL NOT NULL,
    accuracy REAL NOT NULL DEFAULT 0,
    speed REAL NOT NULL DEFAULT 0,
    altitude REAL NOT NULL DEFAULT 0,
    course REAL NOT NULL DEFAULT 0,
    battery_level INTEGER NOT NULL DEFAULT 0,
    battery_status INTEGER NOT NULL DEFAULT 0,
    recorded_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_location_points_device_recorded 
ON location_points(device_id, recorded_at DESC);

-- 2. Circular Geofences
CREATE TABLE IF NOT EXISTS location_geofences (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    latitude REAL NOT NULL,
    longitude REAL NOT NULL,
    radius_meters REAL NOT NULL DEFAULT 150.0,
    notify_enter BOOLEAN NOT NULL DEFAULT 1,
    notify_exit BOOLEAN NOT NULL DEFAULT 1,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 3. Device Geofence State Tracking
CREATE TABLE IF NOT EXISTS location_geofence_states (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    device_id TEXT NOT NULL,
    geofence_id INTEGER NOT NULL,
    is_inside BOOLEAN NOT NULL DEFAULT 0,
    last_transition_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(device_id, geofence_id),
    FOREIGN KEY (geofence_id) REFERENCES location_geofences(id) ON DELETE CASCADE
);

-- 4. Co-location State (Singleton row for 'me' vs 'friend')
CREATE TABLE IF NOT EXISTS location_colocation_state (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    is_colocated BOOLEAN NOT NULL DEFAULT 0,
    last_distance_meters REAL NOT NULL DEFAULT 0,
    last_transition_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_notified_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 5. Location Notification Settings
CREATE TABLE IF NOT EXISTS location_settings (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    secret_token TEXT NOT NULL,
    viewer_token TEXT NOT NULL,
    notify_colocation BOOLEAN NOT NULL DEFAULT 1,
    notify_geofence BOOLEAN NOT NULL DEFAULT 1,
    notify_battery BOOLEAN NOT NULL DEFAULT 1,
    colocation_enter_meters REAL NOT NULL DEFAULT 80.0,
    colocation_exit_meters REAL NOT NULL DEFAULT 250.0,
    cooldown_seconds INTEGER NOT NULL DEFAULT 300,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

---

## 8. Security, Privacy & Integrity

1. **Ingress Authentication:** Ingestion requests must match `LOCATION_SECRET_TOKEN` (configured in `.env` and `location_settings`). Unauthorized pings return HTTP 401 with security audit logging.
2. **Viewer Isolation:** The public viewer requires `LOCATION_VIEWER_TOKEN` in the route `/live/:token`. Random enumeration is mitigated by 32-character cryptographically random tokens.
3. **Data Retention Lifecycle:** A daily background job deletes location pings older than 7 days to guarantee `data.db` remains compact on Android flash storage.
4. **GPS Anti-Spoofing & Sanity Checking:** Locations with impossible coordinates (latitude $\notin [-90, 90]$, longitude $\notin [-180, 180]$) or zero accuracy are rejected.
