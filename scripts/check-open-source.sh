#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
MODE="${1:---worktree}"
ALLOWLIST="$ROOT/.github/allowed-binaries.txt"
MAX_TEXT_BYTES="${MAX_TEXT_BYTES:-1048576}"

case "$MODE" in
  --staged|--worktree) ;;
  *)
    echo "Usage: scripts/check-open-source.sh [--staged|--worktree]" >&2
    exit 2
    ;;
esac

cd "$ROOT"

allowed_binary() {
  local path="$1"
  [ -f "$ALLOWLIST" ] && grep -Fxq "$path" "$ALLOWLIST"
}

is_binary_extension() {
  local lower
  lower="$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]')"
  case "$lower" in
    *.zip|*.pdf|*.mp3|*.mp4|*.mov|*.avi|*.png|*.jpg|*.jpeg|*.gif|*.webp|*.ico|*.woff|*.woff2|*.ttf|*.otf|*.db|*.sqlite|*.sqlite3|*.dmg|*.exe|*.dll|*.so|*.dylib|*.jar|*.war|*.tar|*.gz|*.tgz|*.7z|*.rar)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

list_files() {
  if [ "$MODE" = "--staged" ]; then
    git diff --cached --name-only --diff-filter=ACMR
  else
    git ls-files
  fi
}

failures=0

while IFS= read -r path; do
  [ -z "$path" ] && continue
  [ -f "$path" ] || continue

  if is_binary_extension "$path" && ! allowed_binary "$path"; then
    echo "open-source-check: binary file is not allowed: $path" >&2
    echo "  Add a justified exception to .github/allowed-binaries.txt if this is intentional." >&2
    failures=$((failures + 1))
    continue
  fi

  size="$(wc -c < "$path" | tr -d ' ')"
  if [ "$size" -gt "$MAX_TEXT_BYTES" ] && ! allowed_binary "$path"; then
    echo "open-source-check: file exceeds ${MAX_TEXT_BYTES} bytes: $path" >&2
    failures=$((failures + 1))
  fi
done < <(list_files)

scan_targets=()
while IFS= read -r path; do
  [ -z "$path" ] && continue
  [ -f "$path" ] || continue
  [ "$path" = "scripts/check-open-source.sh" ] && continue
  if ! is_binary_extension "$path"; then
    scan_targets+=("$path")
  fi
done < <(list_files)

if [ "${#scan_targets[@]}" -gt 0 ]; then
  if rg -n --hidden --glob '!.git' --glob '!node_modules' --glob '!web/.next' \
    --glob '!scripts/check-open-source.sh' \
    '(/Users/[^[:space:]"'"'"']+|GolandProjects|zgi-pre|git@github|CLAUDE|Claude Code|Windsurf|\.codex|\.cursorrules|internal channel|placeholder and should be updated)' \
    "${scan_targets[@]}"; then
    echo "open-source-check: remove local/private/tooling references above" >&2
    failures=$((failures + 1))
  fi

  if rg -n --hidden --glob '!.git' --glob '!node_modules' --glob '!web/.next' \
    --glob '!scripts/check-open-source.sh' \
    '(AKIA[0-9A-Z]{16}|AIza[0-9A-Za-z_-]{35}|sk-(proj|live)-[A-Za-z0-9_-]{20,}|xox[baprs]-[A-Za-z0-9-]{10,}|ghp_[A-Za-z0-9_]{30,}|github_pat_[A-Za-z0-9_]{20,}|-----BEGIN (RSA|OPENSSH|EC|DSA|PRIVATE) KEY-----)' \
    "${scan_targets[@]}"; then
    echo "open-source-check: possible secret detected above" >&2
    failures=$((failures + 1))
  fi

  sqlite_test_targets=()
  for path in "${scan_targets[@]}"; do
    case "$path" in
      api/*_test.go|api/**/*_test.go) sqlite_test_targets+=("$path") ;;
    esac
  done
  if [ "${#sqlite_test_targets[@]}" -gt 0 ]; then
    if rg -n --hidden \
      '(gorm\.io/driver/sqlite|github\.com/glebarez/sqlite|sqlite\.Open|file::memory:|:memory:|mode=memory)' \
      "${sqlite_test_targets[@]}"; then
      echo "open-source-check: SQLite-backed Go tests are not allowed; use Postgres-compatible tests instead" >&2
      failures=$((failures + 1))
    fi
  fi
fi

if [ "$failures" -ne 0 ]; then
  exit 1
fi

echo "open-source-check: ok"
