#!/usr/bin/env bash
#
# Pre keeps a byte-for-byte mirror of the Console-authored ratingcore package.
# The in-package Go test enforces the frozen vectors and the sha256 contract
# lock. When the Console checkout is available, additionally compare the mirror
# against the Console source with the Console-owned sync script.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
API_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

echo "ratingcore: running mirrored golden vectors and sha256 contract lock"
( cd "${API_DIR}" && go test ./internal/modules/llm/gateway/ratingcore/... )

CONSOLE_API_DIR="${ZGI_CONSOLE_API_DIR:-}"
if [[ -z "${CONSOLE_API_DIR}" ]]; then
  GUESS="$(cd "${API_DIR}/../../zgi-console/api" 2>/dev/null && pwd || true)"
  [[ -n "${GUESS}" ]] && CONSOLE_API_DIR="${GUESS}"
fi

if [[ -n "${CONSOLE_API_DIR}" && -x "${CONSOLE_API_DIR}/scripts/ratingcore-sync.sh" ]]; then
  "${CONSOLE_API_DIR}/scripts/ratingcore-sync.sh" verify
else
  echo "ratingcore: Console source not available, skipped cross-repo source comparison" >&2
fi
