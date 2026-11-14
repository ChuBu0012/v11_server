## name: deploy-v11
description: Sync Go source code and ecosystem config to the vivo 1804 Termux pocket server (v11), compile the ARM64 Go binary, reload PM2 processes, and return the active Cloudflare Quick Tunnel URL.

# Deploy & Verify Workflow for `v11`

When the user asks to deploy, test, or check the live server on `v11`, follow these exact steps:

1. **Pre-flight Check (Optional Local Syntax Check):**
* Run `go vet ./...` locally on macOS before syncing to catch syntax or type errors early.


2. **Execute Deployment Script:**
* Run `./scripts/deploy.sh` from the repository root.
* This syncs source files via `rsync`, compiles `./server` on `v11`, runs `pm2 startOrRestart ecosystem.config.js --update-env`, saves the PM2 dump, and outputs the active `https://*.trycloudflare.com` URL.


3. **Verify Runtime Logs (If Errors Occur):**
* Never run `pm2 logs` without `--nostream`.
* Use: `ssh v11 "pm2 logs go-server --lines 25 --nostream"`


4. **Health & Resource Inspection:**
* Run `./scripts/status.sh` to report PM2 status, available RAM/Swap, disk space, battery status, and the current public HTTPS endpoint.