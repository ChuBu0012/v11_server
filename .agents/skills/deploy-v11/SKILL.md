---
name: deploy-v11
description: Build ARM64 Go binary on macOS, sync to vivo 1804 Termux pocket server (v11), reload PM2 processes (go-server + ngrok-tunnel), and inspect active ngrok public endpoint.
---

# Deploy & Verify Workflow for `v11`

When the user asks to deploy, test, or inspect the live service on `v11`, follow these steps:

1. **Pre-flight Check & Syntax Validation:**
   - Run `go vet ./...` locally on macOS to catch syntax, type, or lint errors before building.

2. **Execute Deployment Script:**
   - Run `./scripts/deploy.sh` from the repository root.
   - **What this script does:**
     1. Cross-compiles the ARM64 Android Go standalone binary locally on macOS (`CGO_ENABLED=0 GOOS=android GOARCH=arm64 go build -ldflags="-s -w" -o server .`) to match vivo V11's Android 10/Kernel 4.4 and save phone battery/CPU.
     2. Transfers the `server` binary via atomic staging (`server.new` -> `mv server.new server`), `ecosystem.config.js`, `run-ngrok.sh`, and `.env` via `scp`.
     3. Restarts both `go-server` and `ngrok-tunnel` using `pm2 startOrRestart ecosystem.config.js --update-env`.
     4. Saves PM2 state with `pm2 save` so services persist across Termux reboots.
     5. Queries ngrok local API (`http://localhost:4040/api/tunnels`) and outputs the active public HTTPS URL.

3. **Verify Runtime Logs (If Errors Occur):**
   - **CRITICAL:** Always pass `--nostream` when running remote `pm2 logs` via SSH to prevent session hangs:
     ```bash
     ssh v11 "pm2 logs go-server --lines 30 --nostream"
     ssh v11 "pm2 logs ngrok-tunnel --lines 30 --nostream"
     ```

4. **Health & Resource Telemetry:**
   - Run `./scripts/status.sh` to report PM2 process status, RAM/Swap usage, disk space (`/data`), battery level, and public tunnel URL.
   - Alternatively test HTTP endpoint directly:
     ```bash
     curl -s http://localhost:8080/health  # on remote host
     # or through public ngrok URL:
     curl -s https://<YOUR-NGROK-DOMAIN>.ngrok-free.app/health
     ```
