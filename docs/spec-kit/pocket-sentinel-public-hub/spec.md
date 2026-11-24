# Pocket Sentinel — Public Hub (CloudSync & Public Web Dashboard) Specification

**Status:** Approved Design Specification  
**Product:** Pocket Sentinel (vivo 1804 Termux ARM64) + Vercel Web Dashboard  
**Target Modules:** `modules/cloudsync`, `web/`, `shared/config`, `main.go`  
**Date:** 2026-11-24  

---

## 1. Intent & Problem Statement

Currently, Pocket Sentinel runs autonomously on a vivo 1804 (V11) Android device via Termux. It provides comprehensive system telemetry, health checks, an embedded dashboard, and a Telegram bot. However, ingress is strictly bound to the Tailscale private mesh (`http://100.92.194.87:8080`).

This introduces key operational limitations:
- **No External Web Access Without Tailscale:** Viewing the system dashboard or health telemetry from an external device (work PC, secondary smartphone, friend's browser) without an active Tailscale connection is impossible.
- **Privacy Trade-offs with Generic Ingress:** Opening a raw public tunnel (like ngrok or Cloudflare Tunnel) to the entire Go server exposes sensitive endpoints: live GPS tracking (Chubu & Furturia on Live Map), detailed AI model token quotas, hardware controllers (torch, media ring), and root shell execution.
- **Asynchronous Only via Telegram:** While Telegram allows commanding the bot from anywhere, it does not provide an open, visual status dashboard that can be bookmarked or shared.

### The Solution:
Create the **Pocket Sentinel Public Hub** using a clean **Decoupled Cloud Buffer (Firestore) + Public Frontend (Vercel)** architecture:
1. **v11 CloudSync Module (`modules/cloudsync`):** A lightweight background worker running on v11 that extracts telemetry, health, and daily digest data, sanitizes it via an immutable whitelist (stripping all GPS, quota, and system credentials), and pushes it to Cloud Firestore.
2. **Cloud Buffer (Firestore):** Stores only non-sensitive public state (`sentinel_public/status`, `sentinel_public/digest`) and buffers incoming safe action triggers (`sentinel_actions`).
3. **Vercel Public Web Dashboard (`web/`):** A standalone web application hosted on Vercel with a public URL, featuring the Sentinel Soft / Apple HIG aesthetic, real-time Firestore listeners, and an automatic **Mock / Preview Mode** when Firebase credentials are not yet configured.

---

## 2. Goals & Non-goals

### Goals
- **Tailscale-Free Public Dashboard:** Accessible from any browser worldwide via a fast Vercel URL with zero Tailscale requirement.
- **Strict Whitelist Privacy Boundary (Zero Leaks):**
  - **HARD BAN on GPS / Live Map:** Location fixes, coordinates, geofences, and map tracks are completely prohibited from leaving v11.
  - **HARD BAN on AI Quotas:** Quota snapshots, tokens, and model usage metrics remain strictly private on SQLite.
  - **HARD BAN on Destructive Shell/Hardware:** Public UI cannot trigger `/exec`, torch, alarm ringing, or process restarts.
- **Graceful Fallback & Dry-Run Mode:** If Firestore credentials are missing, both v11 and Vercel operate smoothly without errors, using mock data and skipping network sync.
- **Sub-Second Real-Time Updates:** Vercel UI updates live metrics (battery, RAM, disk, services) via Firestore `onSnapshot`.
- **Modern Operational Visual Language:** Public and local dashboards share shield/signal branding, restrained indigo status accents, compact cards, clear stale/error states, and responsive desktop/mobile layouts.
- **Action Lifecycle Feedback:** Public actions subscribe to their Firestore action document and show `pending`, `processing`, `completed`, or `failed` instead of reporting success only from document creation.
- **Safe Public Action Triggers:** Allows triggering safe maintenance actions (e.g., refresh daily digest, ping health check) from the web UI.
- **Zero Impact on v11 Memory Budget:** Keep v11 RAM footprint well within budget (< 300MB, targeting < 25MB total).

### Non-goals
- Exposing the full embedded Pocket Sentinel PWA publicly.
- Relational SQLite sync to the cloud (only JSON snapshots of selected public entities are exported).
- Direct HTTP reverse-proxying into v11 (all communication is outbound-initiated from v11 to Firestore).

---

## 3. High-Level Architecture & Data Flow

```text
  [ Vercel Web Dashboard ] (Public URL)
         │           ▲
         │ (Actions) │ (Real-time Snapshot)
         ▼           │
  [ Cloud Firestore (sentinel_public) ]
         ▲           │
         │ (Sync)    │ (Action Listener)
         │           ▼
  [ Pocket Sentinel (v11) ] ──▶ [ Telegram Bot ] (Notifications)
         │
         ▼
  [ SQLite (data.db) ] (Private: Live Map, AI Quota, Auth)
```

1. **Telemetry & Digest Ingestion:**
   - Every 30–60 seconds, `modules/cloudsync` gathers metrics from `modules/telemetry`, `modules/digest`, and service pingers.
   - `sanitizer.go` enforces the whitelist, creating a sanitized public payload.
   - If credentials exist, it updates `sentinel_public/status` and `sentinel_public/digest` in Firestore.
2. **Web Rendering:**
   - Vercel Web App connects to Firestore via Firebase Web SDK.
   - Listens to `sentinel_public/status` and updates the UI instantly.
   - If in Mock Mode, renders bundled mock data with an informative preview badge.
3. **Action Dispatch:**
   - User clicks `[ 🔄 Refresh Digest ]` on the Vercel Web UI.
   - Web App adds a document to `sentinel_actions` with `status: "pending"`.
   - v11 picks up the action, executes `digest.Run()`, updates the action document to `completed`, and broadcasts an HTML summary card to Telegram.

---

## 4. Data Contracts (Firestore Schema)

### Document: `sentinel_public/status`
```json
{
  "updated_at": "2026-11-24T14:40:00Z",
  "device": {
    "battery_level": 84,
    "is_charging": true,
    "ram_used_percent": 62.4,
    "disk_used_percent": 41.8,
    "uptime_hours": 172.5
  },
  "services": {
    "server": "healthy",
    "gatus": "healthy",
    "tailscale": "connected"
  }
}
```

### Document: `sentinel_public/digest`
```json
{
  "updated_at": "2026-11-24T13:00:00Z",
  "gold": {
    "world_spot": 2685.40,
    "bar_buy": 43800,
    "bar_sell": 43900
  },
  "stocks": [
    {
      "title": "SET ปิดบวก 8 จุด แรงซื้อกลุ่มพลังงาน",
      "source": "Kaohoon",
      "url": "https://..."
    }
  ],
  "news": [
    {
      "category": "ai",
      "summary": "Anthropic เปิดตัวโมเดลใหม่ปรับปรุงความแม่นยำ..."
    }
  ]
}
```

### Collection: `sentinel_actions/{actionId}`
```json
{
  "action": "refresh_digest",
  "requested_at": "2026-11-24T14:42:00Z",
  "status": "pending",
  "completed_at": null,
  "result": null
}
```

---

## 5. Security & Isolation Matrix

| Layer | Component | Security Measure |
|---|---|---|
| **v11 Sanitizer** | `modules/cloudsync/sanitizer.go` | Explicit whitelist struct tags; runtime rejection of unlisted keys |
| **Firestore Rules** | Cloud Security Rules | `sentinel_public/*` is `allow read: if true; allow write: if false;` |
| **Firestore Actions** | Cloud Security Rules | `sentinel_actions/*` allows only creation of `status: "pending"` with allowed action names |
| **v11 Action Dispatcher** | `modules/cloudsync/actions.go` | Strict action whitelist (`refresh_digest`, `ping`); ignores unknown actions |
| **Credentials** | `.env` on v11 | File path to `serviceAccountKey.json` outside git tracking |
