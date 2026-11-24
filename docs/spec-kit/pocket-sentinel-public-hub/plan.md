# Pocket Sentinel — Public Hub (CloudSync & Public Web Dashboard) Implementation Plan

**Target Modules:** `modules/cloudsync`, `web/`, `shared/config`, `main.go`  
**Spec Reference:** `docs/spec-kit/pocket-sentinel-public-hub/spec.md`  

---

## 1. Phases Overview

```text
  Phase 1: Go CloudSync Module (modules/cloudsync)
  ├── Data models, Whitelist sanitizer, and Unit Tests
  └── Firestore REST/Admin client with Dry-run & Mock fallback

  Phase 2: Lifecycle & Server Integration
  ├── Wire into shared/config (.env FIRESTORE_CREDENTIALS)
  └── Register worker in main.go with graceful cancellation

  Phase 3: Public Web Dashboard Scaffold (web/)
  ├── Setup Vite + React/TypeScript + Tailwind Monorepo subfolder
  ├── Apple HIG / Sentinel Soft UI Theme (Dark/Light toggle)
  └── Firebase Web SDK integration + Offline Mock Data Switch

  Phase 4: Component Implementation
  ├── Device Status & Telemetry Card (Battery, RAM, Disk, Uptime)
  ├── Daily Digest Card (Gold, Stocks, AI News)
  └── Safe Interactive Action Button with Optimistic UI

  Phase 5: Verification & Production Readiness
  ├── Full Go test suite execution (go test ./...)
  ├── Production build verification (web build & ARM64 cross-compile)
  └── Vercel Deployment configuration verification
```

---

## 2. Implementation Strategy & Dependencies

### Go CloudSync (`modules/cloudsync`)
- To prevent heavy CGO dependencies and conserve Termux ARM64 memory, we implement a lightweight, pure-Go REST-based or Admin SDK client.
- **Dry-run Mode:** When `FIRESTORE_CREDENTIALS` is empty, the module initializes in `ModeDryRun`, emitting a clean informational log on boot and operating as a no-op ticker without panic or error.
- **Unit Testing:** 100% test coverage using standard `httptest.Server` to verify payload serialization, sanitization, and error handling.

### Web Dashboard (`web/`)
- Located in subfolder `web/` inside the repository.
- Built with Vite + React + TypeScript + Tailwind CSS for instant hot reload and ultra-small bundle size (< 80KB gzip).
- Configured with `vercel.json` so Vercel builds from `web/` seamlessly.
- Includes `web/src/lib/mock.ts` containing realistic snapshot data, enabling complete local preview and UI testing before connecting to Firebase.

---

## 3. Risks & Mitigations

| Risk | Mitigation |
|---|---|
| **Accidental Data Leak (GPS/Quota)** | Whitelist sanitizer uses explicit copy-by-value structs, completely disconnected from `modules/location` models. |
| **Missing Credentials on Boot** | Explicit guard in `NewWorker`: if unconfigured, logs `[cloudsync] running in dry-run mode` and skips network calls. |
| **Firebase Free Tier Exhaustion** | 60-second debounce on sync ticker + smart stationary check; max 1,440 writes/day (quota is 20,000/day). |
| **Android Kernel 4.4 Incompatibility** | Pure-Go implementation with `CGO_ENABLED=0 GOOS=android GOARCH=arm64`. |
