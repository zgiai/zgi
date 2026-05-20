#!/usr/bin/env bash
set -euo pipefail

API_BASE_URL="${API_BASE_URL:-http://localhost:2678}"
KEST_EMAIL="${KEST_EMAIL:-}"
KEST_PASSWORD="${KEST_PASSWORD:-}"
DATASET_ID="${DATASET_ID:-}"
WORKSPACE_ID="${WORKSPACE_ID:-}"

if [[ -z "$KEST_EMAIL" || -z "$KEST_PASSWORD" || -z "$DATASET_ID" || -z "$WORKSPACE_ID" ]]; then
  cat >&2 <<'USAGE'
Missing required environment variables.

Required:
  KEST_EMAIL
  KEST_PASSWORD
  DATASET_ID
  WORKSPACE_ID

Optional:
  API_BASE_URL default: http://localhost:2678

Example:
  API_BASE_URL=http://localhost:2678 \
  KEST_EMAIL=you@example.com \
  KEST_PASSWORD='...' \
  DATASET_ID=... \
  WORKSPACE_ID=... \
  ./scripts/run-kest-dataset-regression.sh
USAGE
  exit 2
fi

login_response="$(curl -sS "$API_BASE_URL/console/api/login" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"$KEST_EMAIL\",\"password\":\"$KEST_PASSWORD\"}")"

jwt_token="$(printf '%s' "$login_response" | node -e '
let input = "";
process.stdin.on("data", chunk => input += chunk);
process.stdin.on("end", () => {
  const body = JSON.parse(input);
  const token = body && body.data && body.data.data && body.data.data.access_token;
  if (!token) {
    console.error("Login did not return body.data.data.access_token");
    process.exit(1);
  }
  process.stdout.write(token);
});
')"

flow_file=".kest/flows/dataset/01-contentparse-authenticated-regression.flow.md"
tmp_flow="$(mktemp /tmp/zgi-dataset-regression-XXXX.flow.md)"
trap 'rm -f "$tmp_flow"' EXIT

sed -E "s#^(GET) /#\\1 ${API_BASE_URL%/}/#" "$flow_file" > "$tmp_flow"

kest run "$tmp_flow" \
  --var jwt_token="$jwt_token" \
  --var dataset_id="$DATASET_ID" \
  --var workspace_id="$WORKSPACE_ID"
