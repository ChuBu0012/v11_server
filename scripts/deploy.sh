#!/usr/bin/env bash
set -euo pipefail

REMOTE_HOST="v11"
REMOTE_DIR="~/go-app"

echo "🔄 [1/4] Syncing source files to ${REMOTE_HOST}:${REMOTE_DIR}..."
rsync -avz --delete 

--exclude '.git/' 

--exclude '.agent/' 

--exclude 'scripts/' 

--exclude 'AGENTS.md' 

--exclude 'CLAUDE.md' 

--exclude 'server' 

--exclude '.env' 

./ "${REMOTE_HOST}:${REMOTE_DIR}/"

echo "🔨 [2/4] Building Go binary on ${REMOTE_HOST}..."
ssh "${REMOTE_HOST}" "cd ${REMOTE_DIR} && go build -o server ."

echo "🚀 [3/4] Restarting PM2 ecosystem..."
ssh "${REMOTE_HOST}" "cd ${REMOTE_DIR} && pm2 startOrRestart ecosystem.config.js --update-env && pm2 save > /dev/null"

sleep 3

echo "🌐 [4/4] Active Status & Public Endpoint:"
ssh "${REMOTE_HOST}" "pm2 ls && echo '--- Public URL ---' && grep -oE 'https://[a-zA-Z0-9-]+.trycloudflare.com' ~/.pm2/logs/quick-tunnel-error.log | tail -n 1"
