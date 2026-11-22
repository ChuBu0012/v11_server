#!/usr/bin/env bash
# ==============================================================================
# install-notify.sh — Install v11-notify CLI into system PATH
# ==============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NOTIFY_SRC="${SCRIPT_DIR}/v11-notify"

if [[ ! -f "${NOTIFY_SRC}" ]]; then
  echo "❌ Error: Cannot find ${NOTIFY_SRC}" >&2
  exit 1
fi

chmod +x "${NOTIFY_SRC}"

INSTALL_DIR=""
# Priority 1: ~/.local/bin (user space, typically in PATH without sudo)
# Priority 2: /usr/local/bin (system wide)
mkdir -p "${HOME}/.local/bin"

if ln -sf "${NOTIFY_SRC}" "${HOME}/.local/bin/v11-notify" 2>/dev/null; then
  INSTALL_DIR="${HOME}/.local/bin"
fi

if [[ -w "/usr/local/bin" ]]; then
  ln -sf "${NOTIFY_SRC}" "/usr/local/bin/v11-notify" 2>/dev/null || true
  INSTALL_DIR="/usr/local/bin"
fi

echo "✅ Installed v11-notify to: ${INSTALL_DIR}/v11-notify"

# Check if in PATH
if command -v v11-notify >/dev/null 2>&1; then
  echo "🎉 v11-notify is in your PATH: $(command -v v11-notify)"
else
  echo "⚠️ Note: Please add ${INSTALL_DIR} to your PATH if not already present:"
  echo "   export PATH=\"\${HOME}/.local/bin:\$PATH\""
fi
