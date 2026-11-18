#!/usr/bin/env bash
set -e

# Read hook payload from stdin if available (non-blocking if stdin is a terminal)
PAYLOAD=""
if [ ! -t 0 ]; then
  PAYLOAD=$(cat)
fi
TARGET_FILE=$(echo "${PAYLOAD}" | jq -r '.toolCall.args.Path // empty' 2>/dev/null || true)

# Resolve project root (parent directory of scripts/)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

cd "${REPO_ROOT}"

# If a .go file was modified, run deterministic formatting & vet checks
if [[ -z "${TARGET_FILE}" || "${TARGET_FILE}" == *.go ]]; then
  if command -v gofmt >/dev/null 2>&1; then
    gofmt -w .
  fi

  if command -v go >/dev/null 2>&1; then
    VET_ERRORS=$(go vet ./... 2>&1 || true)
    if [[ -n "${VET_ERRORS}" ]]; then
      echo "❌ [Deterministic Hook Guardrail] go vet detected errors:" >&2
      echo "${VET_ERRORS}" >&2
      exit 1
    fi
  fi
fi

# Contract for PostToolUse requires empty JSON object on stdout
echo "{}"
exit 0
