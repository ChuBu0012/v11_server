---
name: deploy-v11
description: Build ARM64 Go binary on macOS, sync to vivo 1804 Termux pocket server (v11), reload PM2 process (go-server), and verify Tailscale endpoint.
---

# Deploy & Verify Workflow for `v11`

When the user asks to deploy, test, or inspect the live service on `v11`, follow these steps:

1. **Pre-flight Check & Syntax Validation:**
   - Run `go vet ./...` locally on macOS to catch syntax, type, or lint errors before building.

2. **Execute Deployment Script:**
   - **Full Deploy with Changelog Message (Recommended for Agents):**
     ```bash
     ./scripts/deploy.sh -m "คำอธิบายสิ่งที่อัปเดต / ฟีเจอร์ใหม่"
     ```
   - **Full Deploy (Auto-detects message from latest git commit):**
     ```bash
     ./scripts/deploy.sh
     ```
   - **Fast Config-Only Deploy (No Rebuild / .env only):**
     ```bash
     ./scripts/deploy.sh -c -m "อัปเดตไฟล์ config / .env"
     ```
   - **What this script does:**
     1. Automatically broadcasts Telegram notifications on deploy start, success, and error with the update description (`📦 สิ่งที่อัปเดต:`).
     2. Cross-compiles the ARM64 Android Go standalone binary locally on macOS (`CGO_ENABLED=0 GOOS=android GOARCH=arm64 go build -ldflags="-s -w" -o server .`) to match vivo V11's Android 10/Kernel 4.4 and save phone battery/CPU (skipped in `--config-only` mode).
     3. Transfers all files in a single batch `scp` call to eliminate multiple SSH handshake latencies.
     4. Uses atomic staging (`server.new` -> `mv server.new server`) to prevent ETXTBSY lockouts.
     5. Safely reloads PM2 with `pm2 startOrRestart ecosystem.config.js --update-env`.
     6. Saves PM2 state with `pm2 save` so services persist across Termux reboots.
     7. Polls `http://localhost:8080/health` until HTTP 200 is verified and reports the active Tailscale URL.

3. **Verify Runtime Logs (If Errors Occur):**
   - **CRITICAL:** Always pass `--nostream` when running remote `pm2 logs` via SSH to prevent session hangs:
     ```bash
     ssh v11 "pm2 logs go-server --lines 30 --nostream"
     ```

4. **Health & Resource Telemetry:**
   - Run `./scripts/status.sh` to report PM2 process status, RAM/Swap usage, disk space (`/data`), battery level, and active Tailscale endpoint.
   - Alternatively test HTTP endpoint directly:
     ```bash
     curl -s http://100.92.194.87:8080/health
     ```
