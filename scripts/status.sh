#!/usr/bin/env bash
set -euo pipefail

REMOTE_HOST="v11"

ssh "${REMOTE_HOST}" '
echo "=== PM2 PROCESSES ==="
pm2 ls

echo "=== MEMORY & SWAP ==="
free -h

echo "=== STORAGE (/data) ==="
df -h /data

echo "=== BATTERY ==="
termux-battery-status 2>/dev/null || echo "Termux:API unavailable"

echo "=== PUBLIC TUNNEL URL (ngrok) ==="
curl -s http://localhost:4040/api/tunnels 2>/dev/null | grep -oE "https://[a-zA-Z0-9.-]+\.ngrok(-free)?\.(app|dev)" | head -n 1 || echo "ngrok tunnel inactive"
'
