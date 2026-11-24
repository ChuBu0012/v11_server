# Pocket Sentinel — Public Hub (CloudSync & Public Web Dashboard) Task List

**Spec Reference:** `docs/spec-kit/pocket-sentinel-public-hub/spec.md`  
**Plan Reference:** `docs/spec-kit/pocket-sentinel-public-hub/plan.md`  

---

## Task Matrix

- [x] **T001: Data Models & Whitelist Contracts**
  - Create `modules/cloudsync/types.go` with `PublicStatusPayload`, `PublicDigestPayload`, `PublicActionRequest`.
  - Ensure zero field overlaps with GPS, location, or private credentials.

- [x] **T002: Whitelist Sanitizer & Unit Tests**
  - Create `modules/cloudsync/sanitizer.go`.
  - Write `modules/cloudsync/sanitizer_test.go` ensuring GPS and internal quota fields are strictly stripped.

- [x] **T003: Firestore Client with Dry-run Fallback**
  - Create `modules/cloudsync/client.go` supporting live sync and no-op dry-run mode when unconfigured.
  - Write `modules/cloudsync/client_test.go` with mock HTTP server.

- [x] **T004: Background Sync Worker & Action Listener**
  - Create `modules/cloudsync/worker.go` with configurable ticker (default 60s) and context cancellation.
  - Implement action poll/listener for safe actions (`refresh_digest`, `ping`).

- [x] **T005: Integration into Config and Main Lifecycle**
  - Update `shared/config/config.go` to add `FIRESTORE_CREDENTIALS` and `CLOUDSYNC_INTERVAL`.
  - Update `main.go` to initialize and register `cloudsync.Worker`.
  - Verify `go test ./...` passes 100%.

- [x] **T006: Web App Monorepo Scaffold (`web/`)**
  - Initialize `web/` with Vite, React, TypeScript, and Tailwind CSS.
  - Add Apple HIG / Sentinel Soft typography and color tokens.
  - Add `vercel.json` for one-click deployment.

- [x] **T007: Mock Data & Firebase SDK Layer**
  - Create `web/src/lib/mock-data.ts` containing realistic offline snapshot.
  - Create `web/src/lib/firebase.ts` with auto-detection of mock mode vs live Firestore.

- [x] **T008: Public Dashboard UI Components**
  - Implement Header with Live Status Indicator and Theme Toggle (Dark/Light).
  - Implement Device Status Card (Battery, RAM, Disk, Uptime).
  - Implement Daily Digest Card (Gold, Stocks, AI News).

- [x] **T009: Interactive Action Trigger Component**
  - Implement `[ 🔄 Refresh Digest ]` button with loading state and optimistic feedback.
  - Wire to `sentinel_actions` collection or mock handler.

- [x] **T010: End-to-End Build & Verification**
  - Run `go test ./...` and `go build ./...`.
  - Run `cd web && npm run build` to verify production bundle.
  - Cross-compile ARM64 binary (`CGO_ENABLED=0 GOOS=android GOARCH=arm64 go build -o server .`).
