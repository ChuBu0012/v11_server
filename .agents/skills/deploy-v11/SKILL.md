---
name: deploy-v11
description: Build ARM64 Go binary on macOS, sync to vivo 1804 Termux pocket server (v11), reload PM2 processes (go-server + ngrok-tunnel), and inspect active ngrok public endpoint.
---

# Deploy & Verify Workflow for `v11`

When the user asks to deploy, test, or inspect the live service on `v11`, follow these steps:

1. **Pre-flight Check & Syntax Validation:**
   - Run `go vet ./...` locally on macOS to catch syntax, type, or lint errors before building.

2. **Execute Deployment Script:**
   - **Full Deploy (Binary + Configs):**
     ```bash
     ./scripts/deploy.sh
     ```
   - **Fast Config-Only Deploy (No Rebuild / .env only):**
     ```bash
     ./scripts/deploy.sh -c   # or ./scripts/deploy.sh --config-only
     ```
   - **What this script does:**
     1. Cross-compiles the ARM64 Android Go standalone binary locally on macOS (`CGO_ENABLED=0 GOOS=android GOARCH=arm64 go build -ldflags="-s -w" -o server .`) to match vivo V11's Android 10/Kernel 4.4 and save phone battery/CPU (skipped in `--config-only` mode).
     2. Transfers all files in a single batch `scp` call to eliminate multiple SSH handshake latencies.
     3. Uses atomic staging (`server.new` -> `mv server.new server`) to prevent ETXTBSY lockouts.
     4. Safely terminates stale `proot`/`ngrok` processes (`pkill -9 -x`) and reloads PM2 with `pm2 startOrRestart ecosystem.config.js --update-env`.
     5. Saves PM2 state with `pm2 save` so services persist across Termux reboots.
     6. Polls `http://localhost:8080/health` until HTTP 200 is verified and reports the active ngrok URL.

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
