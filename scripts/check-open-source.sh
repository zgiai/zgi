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

if ! command -v rg >/dev/null 2>&1; then
  echo "open-source-check: ripgrep (rg) is required" >&2
  echo "  Install ripgrep locally or ensure CI installs it before running this check." >&2
  exit 2
fi

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

  case "$path" in
    api/cmd/reset_db/*)
      echo "open-source-check: global database reset command is not allowed: $path" >&2
      echo "  Open-source builds must not include commands that drop or wipe all user data." >&2
      failures=$((failures + 1))
      continue
      ;;
    api/internal/migrationsv2/*|api/pkg/database/migrations/*|api/internal/migrations/m[0-9]*.go|api/internal/migrations/ids.go|api/internal/migrations/lineage.go)
      echo "open-source-check: retired migration path is not allowed: $path" >&2
      echo "  Use api/internal/migrations for schema migrations or api/internal/seeders for seed data." >&2
      failures=$((failures + 1))
      continue
      ;;
  esac

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
    '(/Users/[^[:space:]"'"'"']+|GolandProjects|zgi-pre|git@github|CLAUDE|Claude Code|Windsurf|\.codex|\.cursorrules|internal channel|placeholder and should be updated|Secrets 调试|self-hosted runner|FEISHU_WEBHOOK_URL)' \
    "${scan_targets[@]}"; then
    echo "open-source-check: remove local/private/tooling references above" >&2
    failures=$((failures + 1))
  fi

  command_targets=()
  for path in "${scan_targets[@]}"; do
    case "$path" in
      Makefile|*/Makefile|dev/*|docker/*|scripts/*|api/cmd/*|README.md|api/README.md|web/README.md)
        [ "$path" = "scripts/check-open-source.sh" ] && continue
        command_targets+=("$path")
        ;;
    esac
  done
  if [ "${#command_targets[@]}" -gt 0 ]; then
    if rg -n --hidden \
      '(DROP SCHEMA public CASCADE|DROP DATABASE|Database reset successful|migrate:fresh|db:fresh|db:reset|reset_db|docker compose[^[:cntrl:]]* down -v)' \
      "${command_targets[@]}"; then
      echo "open-source-check: global database reset/fresh commands are not allowed" >&2
      failures=$((failures + 1))
    fi
  fi

  if rg -n --hidden --glob '!.git' --glob '!node_modules' --glob '!web/.next' \
    --glob '!scripts/check-open-source.sh' \
    '(AKIA[0-9A-Z]{16}|AIza[0-9A-Za-z_-]{35}|sk-(proj|live)-[A-Za-z0-9_-]{20,}|xox[baprs]-[A-Za-z0-9-]{10,}|ghp_[A-Za-z0-9_]{30,}|github_pat_[A-Za-z0-9_]{20,}|-----BEGIN (RSA|OPENSSH|EC|DSA|PRIVATE) KEY-----)' \
    "${scan_targets[@]}"; then
    echo "open-source-check: possible secret detected above" >&2
    failures=$((failures + 1))
  fi

  comment_targets=()
  for path in "${scan_targets[@]}"; do
    case "$path" in
      *.go|*.ts|*.tsx|*.js|*.jsx|*.mjs|*.sh|*.yaml|*.yml|*.toml|*.md|*.editorconfig)
        comment_targets+=("$path")
        ;;
    esac
  done
  if [ "${#comment_targets[@]}" -gt 0 ]; then
    if rg -n --hidden --pcre2 '^\s*(//|/\*|\*|#)\s*.*\p{Han}' "${comment_targets[@]}" ||
      rg -n --hidden --pcre2 '^\s*.*//.*\p{Han}' "${comment_targets[@]}"; then
      echo "open-source-check: source comments must be written in English" >&2
      failures=$((failures + 1))
    fi
  fi

  sqlite_source_targets=()
  for path in "${scan_targets[@]}"; do
    case "$path" in
      api/*.go|api/**/*.go)
        case "$path" in
          *_test.go) ;;
          *) sqlite_source_targets+=("$path") ;;
        esac
        ;;
    esac
  done
  if [ "${#sqlite_source_targets[@]}" -gt 0 ]; then
    if rg -n --hidden \
      '(gorm\.io/driver/sqlite|github\.com/glebarez/sqlite|sqlite\.Open|file::memory:|:memory:|mode=memory)' \
      "${sqlite_source_targets[@]}"; then
      echo "open-source-check: SQLite usage is not allowed in production API Go code" >&2
      failures=$((failures + 1))
    fi
  fi

  declare -a migration_targets=()
  for path in "${scan_targets[@]}"; do
    case "$path" in
      api/internal/migrations/*_test.go)
        continue
        ;;
      api/internal/migrations/[0-9]*.go)
        migration_targets+=("$path")
        ;;
    esac
  done
  if [ "${#migration_targets[@]}" -gt 0 ]; then
    for path in "${migration_targets[@]}"; do
      [ "$path" = "api/internal/migrations/20260520000000_initial_schema.go" ] && continue
      file_id="$(basename "$path" .go)"
      if ! [[ "$file_id" =~ ^[0-9]{14}([0-9]{2}|[0-9]{4})?_[a-z0-9_]+$ ]]; then
        echo "open-source-check: migration file must use timestamp_slug naming: $path" >&2
        failures=$((failures + 1))
      fi
      if ! grep -Fq "\"$file_id\"" "$path"; then
        echo "open-source-check: migration filename must match migration ID: $path" >&2
        failures=$((failures + 1))
      fi
      if grep -Eq 'registerMigration\(' "$path"; then
        echo "open-source-check: migrations must use registerSchemaMigration: $path" >&2
        failures=$((failures + 1))
      fi
      if grep -Eq 'AllowDestructive\(' "$path"; then
        echo "open-source-check: migration files must not call AllowDestructive directly: $path" >&2
        failures=$((failures + 1))
      fi
    done
  fi
fi

if [ "$failures" -ne 0 ]; then
  exit 1
fi

echo "open-source-check: ok"
