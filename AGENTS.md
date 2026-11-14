- **SSH Host Alias:** `v11` (configured in `~/.ssh/config` via Tailscale IP `100.92.194.87`, user `u0_a244`, key-based auth enabled — no password prompt).
- **Device & OS:** vivo 1804 (V11), Android 10 (non-rooted), Termux (`android/arm64` / `aarch64`).
- **CPU:** Qualcomm Snapdragon 660 (`SDM660`), 8-core ARM64 architecture (5 active cores visible under normal governor).
- **Memory & Swap:** `5.6Gi` Physical RAM (`~2.8Gi` available) + `3.0Gi` zRAM Swap (`~1.5Gi` free).
  - *Constraint:* Keep active Go services lightweight (`< 300MB` RAM per process) to prevent Android 10 Low Memory Killer (LMK) eviction.
- **Storage / Disk:** `/dev/block/dm-1` (`107G` total, `74G` available at `/data`).
- **Primary Stack:** Go (compiled standalone binary) managed by `PM2` and exposed externally via `cloudflared` Quick Tunnel.
- **Remote Project Path:** `~/go-app` (`/data/data/com.termux/files/home/go-app`).
- **Default Service Port:** `8080` (`http://localhost:8080`).

## 2. Strict OS & Runtime Constraints
- **NO Root / Systemd / Docker:** Never use `sudo`, `systemctl`, `service`, or `docker`.
- **Package Manager:** Always use `pkg install <pkg>` (never `apt-get`).
- **Go Compilation Rules:**
  - Always compile to a binary before running under PM2 (`go build -o server .`).
  - **NEVER** run `pm2 start "go run ..."` (causes orphaned child processes and high memory overhead).
  - If cross-compiling from macOS (ARM64), use `CGO_ENABLED=0 GOOS=linux GOARCH=arm64` or `GOOS=android GOARCH=arm64` with pure-Go libraries (e.g., `modernc.org/sqlite` for SQLite).
- **Android Hardware Integration:** Invoke `termux-*` CLI tools (e.g., `termux-battery-status`, `termux-notification`, `termux-torch`) from Go via `os/exec`.

## 3. Process Management (`PM2`)
- **Active Config:** `ecosystem.config.js` (defines `go-server` and `quick-tunnel`).
- **Persistence:** Always run `pm2 save` after modifying PM2 processes so `~/.bashrc` auto-restores them via `pm2 resurrect`.
- **Log Inspection Rule:** When inspecting PM2 logs remotely, **ALWAYS pass `--nostream`** so the SSH session does not hang:
  ```bash
  ssh v11 "pm2 logs go-server --lines 30 --nostream"