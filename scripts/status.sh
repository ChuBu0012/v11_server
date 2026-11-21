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

echo "=== ACTIVE ENDPOINT (Tailscale) ==="
echo "http://100.92.194.87:8080"
'
