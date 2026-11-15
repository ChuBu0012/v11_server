#!/usr/bin/env bash
# ==============================================================================
# Pocket Sentinel — Automated Deploy Script for vivo 1804 (V11) Termux ARM64
#
# Usage:
#   ./scripts/deploy.sh               # Full build: compile Go binary + sync + restart
#   ./scripts/deploy.sh --config-only # Quick sync: only .env / configs (no rebuild)
#   ./scripts/deploy.sh -c            # Shortcut for --config-only
#   ./scripts/deploy.sh --help        # Show usage
# ==============================================================================

set -euo pipefail

REMOTE_HOST="v11"
REMOTE_DIR="/data/data/com.termux/files/home/go-app"
SSH_OPTS=(-o ConnectTimeout=10 -o ServerAliveInterval=5 -o ServerAliveCountMax=2)

MODE="full"
if [[ "${1:-}" == "--config-only" || "${1:-}" == "-c" ]]; then
  MODE="config"
elif [[ "${1:-}" == "--help" || "${1:-}" == "-h" ]]; then
  echo "Usage: $0 [--config-only | -c | --help]"
  echo "  (no flags)     Full deploy: cross-compile ARM64 binary + batch sync + PM2 reload"
  echo "  --config-only  Fast sync: only upload .env, ecosystem.config.js, run-ngrok.sh"
  exit 0
fi

START_TIME=$(date +%s)
echo "=================================================="
echo "🚀 Pocket Sentinel Deploy [mode: ${MODE}]"
echo "   Target: ${REMOTE_HOST}:${REMOTE_DIR}"
echo "=================================================="

# ------------------------------------------------------------------------------
# [1/4] Build Stage (Full mode only)
# ------------------------------------------------------------------------------
if [[ "${MODE}" == "full" ]]; then
  echo "🔨 [1/4] Cross-compiling ARM64 Go binary for Android..."
  CGO_ENABLED=0 GOOS=android GOARCH=arm64 go build -ldflags="-s -w" -o server .
  cp -f server server.new
  CGO_ENABLED=0 GOOS=android GOARCH=arm64 go build -ldflags="-s -w" -o digest ./cmd/digest
  cp -f digest digest.new
else
  echo "⚡ [1/4] Skipping Go build (--config-only mode)..."
fi

# ------------------------------------------------------------------------------
# [2/4] Batch Transfer (Single SCP command to minimize SSH handshakes)
# ------------------------------------------------------------------------------
echo "🔄 [2/4] Preparing batch files for transfer..."
ssh "${SSH_OPTS[@]}" "${REMOTE_HOST}" "mkdir -p ${REMOTE_DIR}"

TRANSFER_FILES=()
if [[ "${MODE}" == "full" ]]; then
  TRANSFER_FILES+=(server.new)
  if [[ -f digest.new ]]; then
    TRANSFER_FILES+=(digest.new)
  fi
fi
if [[ -f ecosystem.config.js ]]; then
  TRANSFER_FILES+=(ecosystem.config.js)
fi
if [[ -f run-ngrok.sh ]]; then
  TRANSFER_FILES+=(run-ngrok.sh)
fi
if [[ -f .env ]]; then
  TRANSFER_FILES+=(.env)
fi

echo "📦 Uploading ${#TRANSFER_FILES[@]} file(s) in a single batch transfer..."
scp "${SSH_OPTS[@]}" "${TRANSFER_FILES[@]}" "${REMOTE_HOST}:${REMOTE_DIR}/"

# Clean up local staging file
rm -f server.new digest.new

# Atomic swap & permissions on remote
if [[ "${MODE}" == "full" ]]; then
  ssh "${SSH_OPTS[@]}" "${REMOTE_HOST}" "cd ${REMOTE_DIR} && mv -f server.new server 2>/dev/null || true; mv -f digest.new digest 2>/dev/null || true; chmod +x server digest run-ngrok.sh 2>/dev/null || true"
else
  ssh "${SSH_OPTS[@]}" "${REMOTE_HOST}" "cd ${REMOTE_DIR} && chmod +x run-ngrok.sh 2>/dev/null || true"
fi

# ------------------------------------------------------------------------------
# [3/4] Process Restart & Health Management
# ------------------------------------------------------------------------------
echo "🔄 [3/4] Reloading PM2 services on ${REMOTE_HOST}..."
ssh "${SSH_OPTS[@]}" "${REMOTE_HOST}" "REMOTE_DIR='${REMOTE_DIR}' bash -s" << 'EOF'
  cd "${REMOTE_DIR}"
  # If ngrok-tunnel is already online, leave it alone to prevent tunnel drop / reconnection delay.
  # Only restart go-server. If ngrok is not online, start/restart ecosystem.
  NGROK_PID=$(pm2 pid ngrok-tunnel 2>/dev/null || true)
  if [ -n "$NGROK_PID" ] && [ "$NGROK_PID" != "0" ]; then
    pm2 restart go-server --update-env > /dev/null
  else
    pkill -9 -x ngrok 2>/dev/null || true
    pkill -9 -x proot 2>/dev/null || true
    pm2 startOrRestart ecosystem.config.js --update-env > /dev/null
  fi
  pm2 save > /dev/null 2>&1 || true
EOF

# ------------------------------------------------------------------------------
# [4/4] Health Verification & Endpoint Status
# ------------------------------------------------------------------------------
echo "🩺 [4/4] Verifying service health..."
ssh "${SSH_OPTS[@]}" "${REMOTE_HOST}" '
  # Poll localhost:8080/health up to 8 seconds
  OK=0
  for i in {1..8}; do
    if curl -s -f --max-time 2 http://localhost:8080/health > /dev/null 2>&1; then
      OK=1
      break
    fi
    sleep 1
  done

  echo ""
  if [ "$OK" -eq 1 ]; then
    echo "✅ Service Health Check: PASS (HTTP 200)"
  else
    echo "⚠️ Service Health Check: Waiting for server to respond..."
  fi

  echo ""
  echo "=== PM2 Processes ==="
  pm2 ls

  echo ""
  echo "=== Public ngrok Endpoint ==="
  ENDPOINT=$(curl -s --max-time 3 http://localhost:4040/api/tunnels 2>/dev/null | grep -oE "https://[a-zA-Z0-9.-]+\.ngrok(-free)?\.(app|dev)" | head -n 1 || true)
  if [ -n "${ENDPOINT}" ]; then
    echo "🌐 Active URL: ${ENDPOINT}"
  else
    echo "ℹ️  ngrok tunnel initializing (check localhost:4040 shortly)"
  fi
'

TOTAL_TIME=$(( $(date +%s) - START_TIME ))
echo ""
echo "✨ Deploy completed successfully in ${TOTAL_TIME}s!"
