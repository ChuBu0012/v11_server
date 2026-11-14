#!/usr/bin/env bash
set -euo pipefail

REMOTE_HOST="v11"
REMOTE_DIR="/data/data/com.termux/files/home/go-app"

echo "🔨 [1/4] Cross-compiling ARM64 Go binary for Android (vivo V11)..."
CGO_ENABLED=0 GOOS=android GOARCH=arm64 go build -ldflags="-s -w" -o server .

echo "🔄 [2/4] Copying binary (via atomic staging), config, and env to ${REMOTE_HOST}..."
ssh "${REMOTE_HOST}" "mkdir -p ${REMOTE_DIR}"
# Upload to staging file first to prevent 'ETXTBSY: Text file busy' if server is actively running
scp server "${REMOTE_HOST}:${REMOTE_DIR}/server.new"
scp ecosystem.config.js "${REMOTE_HOST}:${REMOTE_DIR}/"
if [ -f run-ngrok.sh ]; then
  scp run-ngrok.sh "${REMOTE_HOST}:${REMOTE_DIR}/"
fi
if [ -f .env ]; then
  scp .env "${REMOTE_HOST}:${REMOTE_DIR}/.env"
fi
ssh "${REMOTE_HOST}" "cd ${REMOTE_DIR} && mv -f server.new server && chmod +x server run-ngrok.sh 2>/dev/null || true"

echo "🚀 [3/4] Restarting PM2 ecosystem on ${REMOTE_HOST}..."
ssh "${REMOTE_HOST}" "cd ${REMOTE_DIR} && pm2 startOrRestart ecosystem.config.js --update-env && pm2 save > /dev/null"

sleep 3

echo "🌐 [4/4] Active Status & Public Endpoint:"
ssh "${REMOTE_HOST}" "pm2 ls && echo '--- Public Endpoint (ngrok) ---' && (
  ENDPOINT=\$(curl -s http://localhost:4040/api/tunnels 2>/dev/null | grep -oE 'https://[a-zA-Z0-9.-]+\.ngrok(-free)?\.(app|dev)' | head -n 1 || true)
  if [ -n \"\${ENDPOINT}\" ]; then
    echo \"Active URL: \${ENDPOINT}\"
  else
    echo 'Notice: ngrok tunnel is not active yet. Ensure NGROK_AUTHTOKEN is set in .env'
  fi
)"
