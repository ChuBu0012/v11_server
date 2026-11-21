#!/usr/bin/env bash
# ==============================================================================
# Pocket Sentinel — Automated Deploy Script for vivo 1804 (V11) Termux ARM64
#
# Usage:
#   ./scripts/deploy.sh                           # Full build with auto git commit message
#   ./scripts/deploy.sh -m "สิ่งที่อัปเดต"        # Full build with custom deploy message
#   ./scripts/deploy.sh -c                        # Quick sync: only .env / configs (no rebuild)
#   ./scripts/deploy.sh -c -m "อัปเดต .env"       # Quick sync with custom deploy message
#   ./scripts/deploy.sh --help                    # Show usage
# ==============================================================================

set -euo pipefail

# If running natively inside Termux on vivo V11, delegate directly to on-device deploy
if [ -d "/data/data/com.termux" ] || [ -n "${TERMUX_VERSION:-}" ]; then
  SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
  exec "${SCRIPT_DIR}/deploy-v11.sh" "$@"
fi

REMOTE_HOST="v11"
REMOTE_DIR="/data/data/com.termux/files/home/go-app"
SSH_OPTS=(-o ConnectTimeout=10 -o ServerAliveInterval=5 -o ServerAliveCountMax=2)

escape_html() {
  local s="$1"
  s="${s//&/&amp;}"
  s="${s//</&lt;}"
  s="${s//>/&gt;}"
  echo "$s"
}

notify_telegram() {
  local msg="$1"
  if [[ -f .env ]]; then
    local token=$(grep -E "^TELEGRAM_BOT_TOKEN=" .env | cut -d '=' -f2- | tr -d '"'\'' ' || true)
    local chat_id=$(grep -E "^TELEGRAM_CHAT_ID=" .env | cut -d '=' -f2- | tr -d '"'\'' ' || true)
    if [[ -n "${token}" && -n "${chat_id}" ]]; then
      # Primary: send directly from local macOS using IPv4 and HTTP/1.1
      if ! curl --http1.1 -4 -s --connect-timeout 4 --max-time 8 -X POST "https://api.telegram.org/bot${token}/sendMessage" \
        -d "chat_id=${chat_id}" \
        --data-urlencode "text=${msg}" \
        -d "parse_mode=HTML" > /dev/null 2>&1; then
        # Fallback: execute via remote host (v11) if local network times out
        ssh "${SSH_OPTS[@]}" "${REMOTE_HOST}" "curl --http1.1 -4 -s --connect-timeout 4 --max-time 8 -X POST 'https://api.telegram.org/bot${token}/sendMessage' -d 'chat_id=${chat_id}' --data-urlencode 'text=${msg}' -d 'parse_mode=HTML' > /dev/null 2>&1" || true
      fi
    fi
  fi
}

on_error() {
  local line="$1"
  echo "❌ Deploy failed on line ${line}"
  local err_msg="❌ <b>การ Deploy ล้มเหลว!</b> (เกิดข้อผิดพลาดที่บรรทัด ${line} ของ deploy script)"
  if [[ -n "${DEPLOY_MSG:-}" ]]; then
    err_msg+=$'\n'
    err_msg+="📦 <b>งานที่พยายาม Deploy:</b> $(escape_html "${DEPLOY_MSG}")"
  fi
  notify_telegram "${err_msg}"
}
trap 'on_error $LINENO' ERR

MODE="full"
DEPLOY_MSG="${DEPLOY_MSG:-${DEPLOY_MESSAGE:-}}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    -m|--message|--msg)
      DEPLOY_MSG="${2:-}"
      shift 2
      ;;
    -c|--config-only)
      MODE="config"
      shift
      ;;
    -h|--help)
      echo "Usage: $0 [-m \"message\"] [--config-only | -c] [--help]"
      echo "  -m, --message      Specify what is being deployed (included in Telegram notifications)"
      echo "  -c, --config-only  Fast sync: only upload .env, ecosystem.config.js, run-ngrok.sh"
      echo "  -h, --help         Show this help message"
      exit 0
      ;;
    *)
      if [[ -z "${DEPLOY_MSG}" && "$1" != -* ]]; then
        DEPLOY_MSG="$1"
      fi
      shift
      ;;
  esac
done

# Auto-detect deploy message from git commit if not explicitly provided
if [[ -z "${DEPLOY_MSG}" ]] && command -v git >/dev/null 2>&1 && git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  DEPLOY_MSG=$(git log -1 --pretty=format:"%s" 2>/dev/null || true)
fi
if [[ -z "${DEPLOY_MSG}" ]]; then
  DEPLOY_MSG="อัปเดตระบบทั่วไป"
fi

ESCAPED_DEPLOY_MSG=$(escape_html "${DEPLOY_MSG}")

START_TIME=$(date +%s)
echo "=================================================="
echo "🚀 Pocket Sentinel Deploy [mode: ${MODE}]"
echo "   Target:  ${REMOTE_HOST}:${REMOTE_DIR}"
echo "   Message: ${DEPLOY_MSG}"
echo "=================================================="

START_NOTI="🚀 <b>เริ่มกระบวนการ Deploy บน vivo v11</b> [mode: ${MODE}]"
START_NOTI+=$'\n'
START_NOTI+="📦 <b>สิ่งที่อัปเดต:</b> ${ESCAPED_DEPLOY_MSG}"
notify_telegram "${START_NOTI}"

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
if [[ -f .env ]]; then
  TRANSFER_FILES+=(.env)
fi

echo "📦 Uploading ${#TRANSFER_FILES[@]} file(s) in a single batch transfer..."
scp "${SSH_OPTS[@]}" "${TRANSFER_FILES[@]}" "${REMOTE_HOST}:${REMOTE_DIR}/"

# Clean up local staging file
rm -f server.new digest.new

# Atomic swap & permissions on remote
if [[ "${MODE}" == "full" ]]; then
  echo "💾 Creating automatic remote backup (server -> server.bak)..."
  ssh "${SSH_OPTS[@]}" "${REMOTE_HOST}" "cd ${REMOTE_DIR} && if [ -f server ]; then cp -f server server.bak; fi && mv -f server.new server 2>/dev/null || true; mv -f digest.new digest 2>/dev/null || true; chmod +x server digest 2>/dev/null || true"
fi

# ------------------------------------------------------------------------------
# [3/4] Process Restart & Health Management
# ------------------------------------------------------------------------------
echo "🔄 [3/4] Reloading PM2 services on ${REMOTE_HOST}..."
ssh "${SSH_OPTS[@]}" "${REMOTE_HOST}" "REMOTE_DIR='${REMOTE_DIR}' bash -s" << 'EOF'
  cd "${REMOTE_DIR}"
  pkill -9 -x ngrok 2>/dev/null || true
  pkill -9 -x proot 2>/dev/null || true
  pm2 delete ngrok-tunnel > /dev/null 2>&1 || true
  pm2 startOrRestart ecosystem.config.js --update-env > /dev/null
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
    echo "❌ Service Health Check: Server failed to respond on :8080 within 8s"
    echo "⚠️ Initiating automated rollback to server.bak..."
    if [ -f server.bak ]; then
      cp -f server.bak server
      chmod +x server
      pm2 restart go-server --update-env > /dev/null 2>&1 || true
      echo "🔄 Restored server.bak and reloaded go-server via PM2."
      echo "📋 Diagnostics: Recent PM2 logs (nostream):"
      pm2 logs go-server --lines 30 --nostream
    else
      echo "⚠️ No server.bak found on remote host; unable to restore previous binary."
    fi
    exit 1
  fi

  echo ""
  echo "=== PM2 Processes ==="
  pm2 ls

  echo ""
  echo "=== PM2 Logs: go-server (last 20 lines) ==="
  pm2 logs go-server --lines 20 --nostream

  echo ""
  echo "=== Active Endpoint (Tailscale) ==="
  echo "🌐 URL: http://100.92.194.87:8080"
'

TOTAL_TIME=$(( $(date +%s) - START_TIME ))
echo ""
echo "✨ Deploy completed successfully in ${TOTAL_TIME}s!"

ENDPOINT="http://100.92.194.87:8080"
SUCCESS_MSG="✅ <b>Deploy สำเร็จสมบูรณ์</b> ใน ${TOTAL_TIME}s! บริการพร้อมใช้งาน"
SUCCESS_MSG+=$'\n'
SUCCESS_MSG+="📦 <b>สิ่งที่อัปเดต:</b> ${ESCAPED_DEPLOY_MSG}"
if [[ -n "${ENDPOINT}" ]]; then
  SUCCESS_MSG+=$'\n'
  SUCCESS_MSG+="🌐 <b>URL:</b> ${ENDPOINT}"
fi
notify_telegram "${SUCCESS_MSG}"
