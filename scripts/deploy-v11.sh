#!/usr/bin/env bash
# ==============================================================================
# Pocket Sentinel — Fast On-Device Deploy Script for Termux on vivo 1804 (V11)
#
# Usage (run directly in Termux on v11):
#   ./scripts/deploy-v11.sh
#   or from any directory:
#   cd ~/go-app && ./scripts/deploy-v11.sh
# ==============================================================================

set -euo pipefail

APP_DIR="${HOME}/go-app"
if [ ! -d "${APP_DIR}" ]; then
  APP_DIR="$(cd "$(dirname "$0")/.." && pwd)"
fi
cd "${APP_DIR}"

echo "=================================================="
echo "📱 Pocket Sentinel — Deploying natively on vivo V11"
echo "   Directory: ${APP_DIR}"
echo "=================================================="

# 1. Pull latest code from GitHub
if [ -d .git ]; then
  echo "📥 [1/4] Pulling latest changes from git..."
  git pull origin main || echo "⚠️ Git pull reported warnings, proceeding with local working tree..."
else
  echo "⚡ [1/4] Skipping git pull (no .git directory)..."
fi

# 2. Build Go binary natively
echo "🔨 [2/4] Building ARM64 Go binary with stripped debug symbols (-s -w)..."
if [ -f server ]; then
  cp -f server server.bak
  echo "💾 Backed up existing server binary -> server.bak"
fi

go build -ldflags="-s -w" -o server .
if [ -d ./cmd/digest ]; then
  go build -ldflags="-s -w" -o digest ./cmd/digest || true
fi
chmod +x server digest 2>/dev/null || true

# 3. Reload PM2 with latest environment variables from ecosystem.config.js & .env
echo "🔄 [3/4] Reloading PM2 go-server with --update-env..."
pm2 restart ecosystem.config.js --only go-server --update-env || pm2 restart go-server --update-env

# 4. Verify health & display recent logs
echo "🩺 [4/4] Verifying service health..."
sleep 1
OK=0
for i in {1..8}; do
  if curl -s -f --max-time 2 http://localhost:8080/health > /dev/null 2>&1; then
    OK=1
    break
  fi
  sleep 1
done

if [ "$OK" -eq 1 ]; then
  echo "✅ Service Health Check: PASS (HTTP 200)"
else
  echo "❌ Service Health Check failed! Initiating rollback to server.bak..."
  if [ -f server.bak ]; then
    cp -f server.bak server
    pm2 restart ecosystem.config.js --only go-server --update-env > /dev/null 2>&1 || true
    echo "🔄 Rolled back to server.bak successfully."
  fi
fi

echo ""
echo "=== PM2 Logs: go-server (last 20 lines) ==="
pm2 logs go-server --lines 20 --nostream
