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

START_TIME=$(date +%s)

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
      curl -s -X POST "https://api.telegram.org/bot${token}/sendMessage" \
        -d "chat_id=${chat_id}" \
        --data-urlencode "text=${msg}" \
        -d "parse_mode=HTML" > /dev/null 2>&1 || true
    fi
  fi
}

on_error() {
  local line="$1"
  echo "❌ Deploy failed on line ${line}"
  local err_msg="❌ <b>การ Deploy ล้มเหลว!</b> (เกิดข้อผิดพลาดที่บรรทัด ${line} ของ deploy-v11.sh)"
  if [[ -n "${DEPLOY_MSG:-}" ]]; then
    err_msg+=$'\n'
    err_msg+="📦 <b>งานที่พยายาม Deploy:</b> $(escape_html "${DEPLOY_MSG}")"
  fi
  notify_telegram "${err_msg}"
}
trap 'on_error $LINENO' ERR

DEPLOY_MSG="${DEPLOY_MSG:-${DEPLOY_MESSAGE:-}}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    -m|--message|--msg)
      DEPLOY_MSG="${2:-}"
      shift 2
      ;;
    -h|--help)
      echo "Usage: $0 [-m \"message\"] [--help]"
      echo "  -m, --message  Specify what is being deployed (included in Telegram notifications)"
      echo "  -h, --help     Show this help message"
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

echo "=================================================="
echo "📱 Pocket Sentinel — Deploying natively on vivo V11"
echo "   Directory: ${APP_DIR}"
echo "   Message:   ${DEPLOY_MSG:-(from latest commit after pull)}"
echo "=================================================="

START_NOTI="🚀 <b>เริ่มกระบวนการ Deploy บน vivo v11</b>"
START_NOTI+=$'\n'
if [[ -n "${DEPLOY_MSG}" ]]; then
  START_NOTI+="📦 <b>สิ่งที่อัปเดต:</b> $(escape_html "${DEPLOY_MSG}")"
else
  START_NOTI+="📥 กำลังดึงโค้ดล่าสุดจาก GitHub…"
fi
notify_telegram "${START_NOTI}"

# 1. Pull latest code from GitHub. A failed pull aborts the deploy: rebuilding
#    the old tree and reporting "success" would be misleading.
if [ -d .git ]; then
  echo "📥 [1/4] Pulling latest code from origin/main..."
  BEFORE_REV=$(git rev-parse --short HEAD 2>/dev/null || echo "?")
  if ! PULL_OUT=$(git pull --ff-only origin main 2>&1); then
    trap - ERR
    echo "${PULL_OUT}"
    echo "❌ git pull failed — deploy aborted (nothing was built or restarted)."
    PULL_TAIL=$(printf '%s' "${PULL_OUT}" | tail -n 3)
    notify_telegram "❌ <b>Deploy ยกเลิก:</b> git pull ไม่สำเร็จ (ยังไม่ได้ build หรือ restart)"$'\n'"<pre>$(escape_html "${PULL_TAIL}")</pre>"
    exit 1
  fi
  echo "${PULL_OUT}"
  AFTER_REV=$(git rev-parse --short HEAD 2>/dev/null || echo "?")
else
  echo "⚡ [1/4] Skipping git pull (no .git directory)..."
  BEFORE_REV="?"; AFTER_REV="?"
fi

# Describe what was actually deployed (the pulled commit), unless a message was given.
if [[ -z "${DEPLOY_MSG}" ]] && git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  DEPLOY_MSG=$(git log -1 --pretty=format:"%s" 2>/dev/null || true)
fi
if [[ -z "${DEPLOY_MSG}" ]]; then
  DEPLOY_MSG="อัปเดตระบบทั่วไปบนอุปกรณ์"
fi
if [[ "${BEFORE_REV}" != "?" && "${BEFORE_REV}" != "${AFTER_REV}" ]]; then
  DEPLOY_MSG="${DEPLOY_MSG} (${BEFORE_REV} → ${AFTER_REV})"
elif [[ "${BEFORE_REV}" != "?" ]]; then
  DEPLOY_MSG="${DEPLOY_MSG} (ไม่มี commit ใหม่ · ${AFTER_REV})"
fi
ESCAPED_DEPLOY_MSG=$(escape_html "${DEPLOY_MSG}")

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
  TOTAL_TIME=$(( $(date +%s) - START_TIME ))
  ENDPOINT=$(curl -s --max-time 3 http://localhost:4040/api/tunnels 2>/dev/null | grep -oE "https://[a-zA-Z0-9.-]+\.ngrok(-free)?\.(app|dev)" | head -n 1 || true)
  if [ -z "${ENDPOINT}" ] && [ -f .env ]; then
    ENDPOINT=$(grep -E "^NGROK_DOMAIN=" .env | cut -d '=' -f2- | tr -d '"'\'' ' || true)
  fi

  SUCCESS_MSG="✅ <b>Deploy บน vivo v11 สำเร็จสมบูรณ์</b> ใน ${TOTAL_TIME}s! บริการพร้อมใช้งาน"
  SUCCESS_MSG+=$'\n'
  SUCCESS_MSG+="📦 <b>สิ่งที่อัปเดต:</b> ${ESCAPED_DEPLOY_MSG}"
  if [[ -n "${ENDPOINT}" ]]; then
    SUCCESS_MSG+=$'\n'
    SUCCESS_MSG+="🌐 <b>URL:</b> ${ENDPOINT}"
  fi
  notify_telegram "${SUCCESS_MSG}"
else
  echo "❌ Service Health Check failed! Initiating rollback to server.bak..."
  notify_telegram "❌ <b>Service Health Check ล้มเหลว!</b> กำลัง rollback กลับเป็น server.bak..."
  if [ -f server.bak ]; then
    cp -f server.bak server
    pm2 restart ecosystem.config.js --only go-server --update-env > /dev/null 2>&1 || true
    echo "🔄 Rolled back to server.bak successfully."
  fi
  exit 1
fi

echo ""
echo "=== PM2 Logs: go-server (last 20 lines) ==="
pm2 logs go-server --lines 20 --nostream
