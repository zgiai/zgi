#!/bin/bash
# ============================================================
# ZGI Web Platform - Container Entrypoint
# ============================================================
# Note: If using Windows, convert to Unix format:
#   dos2unix entrypoint.sh

set -e

# ===== Normalize legacy env names =====
export NEXT_PUBLIC_API_URL=${NEXT_PUBLIC_API_URL:-${API_URL}}
export NEXT_PUBLIC_BASE_PATH=${NEXT_PUBLIC_BASE_PATH:-${BASE_PATH}}
export NEXT_PUBLIC_MARKET_API_URL=${NEXT_PUBLIC_MARKET_API_URL:-${MARKETPLACE_API_URL}}
export NEXT_PUBLIC_DEPLOY_ENV=${NEXT_PUBLIC_DEPLOY_ENV:-${DEPLOY_ENV}}
export NEXT_PUBLIC_RUN_MODE=${NEXT_PUBLIC_RUN_MODE:-${ZGI_RUN_MODE:-${EDITION}}}
export NEXT_PUBLIC_EDITION=${NEXT_PUBLIC_EDITION:-${NEXT_PUBLIC_RUN_MODE}}
export NEXT_PUBLIC_APP_VERSION=${NEXT_PUBLIC_APP_VERSION:-${APP_VERSION}}
export NEXT_PUBLIC_SENTRY_DSN=${NEXT_PUBLIC_SENTRY_DSN:-${SENTRY_DSN}}
export NEXT_PUBLIC_SENTRY_ENVIRONMENT=${NEXT_PUBLIC_SENTRY_ENVIRONMENT:-${SENTRY_ENVIRONMENT:-${DEPLOY_ENV}}}
export NEXT_PUBLIC_SENTRY_TRACES_SAMPLE_RATE=${NEXT_PUBLIC_SENTRY_TRACES_SAMPLE_RATE:-${SENTRY_TRACES_SAMPLE_RATE}}

# ===== Server Configuration =====
export HOSTNAME=0.0.0.0
export NEXT_TELEMETRY_DISABLED=${NEXT_TELEMETRY_DISABLED:-1}

# ===== Generate env.js for client runtime =====
ENV_JS_PATH="/app/web/public/env.js"
TMP_ENV_JS="/tmp/env.js"
mkdir -p /app/web/public

echo 'window.__ENV__ = Object.assign({}, window.__ENV__ || {}, {' > "$TMP_ENV_JS"

# Collect and serialize NEXT_PUBLIC_* variables
ENV_VARS=$(env | grep '^NEXT_PUBLIC_' | sort || true)
if [ -n "$ENV_VARS" ]; then
  while IFS='=' read -r NAME VALUE; do
    [ -z "$VALUE" ] && continue
    # Escape backslashes and quotes for safe JS string embedding
    ESCAPED=$(printf '%s' "$VALUE" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g')
    printf '  "%s":"%s",\n' "$NAME" "$ESCAPED" >> "$TMP_ENV_JS"
  done <<< "$ENV_VARS"
fi

echo '});' >> "$TMP_ENV_JS"
mv "$TMP_ENV_JS" "$ENV_JS_PATH"

NORMALIZED_BASE_PATH="${NEXT_PUBLIC_BASE_PATH#/}"
NORMALIZED_BASE_PATH="${NORMALIZED_BASE_PATH%/}"
if [ -n "$NORMALIZED_BASE_PATH" ]; then
  BASEPATH_ENV_JS_PATH="/app/web/public/${NORMALIZED_BASE_PATH}/env.js"
  mkdir -p "$(dirname "$BASEPATH_ENV_JS_PATH")"
  cp "$ENV_JS_PATH" "$BASEPATH_ENV_JS_PATH"
fi

echo "[entrypoint] Starting PM2 with ${PM2_INSTANCES:-2} instances..."
pm2 start /app/web/server.js --name zgi-web --cwd /app/web -i ${PM2_INSTANCES:-2} --no-daemon
