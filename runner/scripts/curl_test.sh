#!/bin/bash
# curl_test.sh - Complete E2E test for uv-echo plugin
# Tests: regex extraction, network (HTTP GET)
#
# Usage:
#   ./curl_test.sh           # Run all steps automatically
#   ./curl_test.sh -d         # Debug mode: pause after each step
#   ./curl_test.sh -s 3       # Start from step 3
#   ./curl_test.sh -d -s 5    # Debug mode, start from step 5

set -e

BASE_URL="${BASE_URL:-http://localhost:2665}"
API_KEY="admin-key-123"
PLUGIN_DIR="${PLUGIN_DIR:-$(cd "$(dirname "$0")/.." && pwd)/examples/test_plugin/uv_echo_0.0.1}"
WORKSPACE_DIR="${WORKSPACE_DIR:-$(cd "$(dirname "$0")/.." && pwd)/workspace/uv-echo-0.0.1}"

# Parse arguments
DEBUG_MODE=false
START_STEP=1
while getopts "ds:" opt; do
  case $opt in
    d) DEBUG_MODE=true ;;
    s) START_STEP=$OPTARG ;;
    *) echo "Usage: $0 [-d] [-s step_number]"; exit 1 ;;
  esac
done

# Debug pause function
pause_if_debug() {
  if [ "$DEBUG_MODE" = true ]; then
    echo -e "\n>>> Press ENTER to continue to next step (or Ctrl+C to exit)..."
    read -r
  fi
}

# Check if step should run
should_run_step() {
  [ "$1" -ge "$START_STEP" ]
}

echo "=========================================="
echo "Runner E2E Test"
if [ "$DEBUG_MODE" = true ]; then
  echo "Mode: DEBUG (will pause after each step)"
fi
if [ "$START_STEP" -gt 1 ]; then
  echo "Starting from step: $START_STEP"
fi
echo "=========================================="

# Health check
if should_run_step 1; then
  echo -e "\n[1/9] Health check..."
  curl -s "${BASE_URL}/healthz" \
      -H "X-API-Key: ${API_KEY}"| jq
  pause_if_debug
fi


# Register plugin
if should_run_step 2; then
  echo -e "\n[2/9] Registering uv-echo plugin..."
  curl -s -X POST "${BASE_URL}/api/v1/plugins" \
    -H "Content-Type: application/json" \
    -H "X-API-Key: ${API_KEY}" \
    -d '{
      "manifest": {
        "name": "uv-echo",
        "version": "0.0.1",
        "author": "vic",
        "runner": {
          "language": "python",
          "entrypoint": "main_runner"
        }
      }
    }' | jq
  pause_if_debug
fi

# Package and install plugin
if should_run_step 3; then
  echo -e "\n[3/9] Packaging and installing plugin..."
  rm -f /tmp/uv-echo.zip
  (cd "${PLUGIN_DIR}" && zip -rq /tmp/uv-echo.zip .)

  INSTALL_RESULT=$(curl -s -X POST "${BASE_URL}/api/v1/plugins/uv-echo:0.0.1/install" \
    -H "X-API-Key: ${API_KEY}" \
    -F "file=@/tmp/uv-echo.zip")
  echo "${INSTALL_RESULT}" | jq
  pause_if_debug
fi

# Launch session
if should_run_step 4; then
  echo -e "\n[4/9] Launching plugin session..."
  SESSION_RESULT=$(curl -s -X POST "${BASE_URL}/api/v1/sessions" \
    -H "Content-Type: application/json" \
    -H "X-API-Key: ${API_KEY}" \
    -d '{
      "name": "uv-echo",
      "version": "0.0.1",
      "language": "python",
      "entrypoint": "main_runner",
      "working_dir": "'"${WORKSPACE_DIR}"'"
    }')
  echo "${SESSION_RESULT}" | jq

  SESSION_ID=$(echo "${SESSION_RESULT}" | jq -r '.id')
  echo "Session ID: ${SESSION_ID}"
  # Save session ID for later steps
  echo "${SESSION_ID}" > /tmp/plugin_session_id.txt
  pause_if_debug
else
  # Load session ID from previous run
  if [ -f /tmp/plugin_session_id.txt ]; then
    SESSION_ID=$(cat /tmp/plugin_session_id.txt)
    echo "Using saved Session ID: ${SESSION_ID}"
  fi
fi

# Wait for ready
if should_run_step 5; then
  echo -e "\n[5/9] Waiting for plugin ready..."
  for i in {1..30}; do
    READY=$(curl -s "${BASE_URL}/api/v1/invoke/sessions/${SESSION_ID}/ready" \
      -H "X-API-Key: ${API_KEY}" | jq -r '.ready')
    if [ "${READY}" = "true" ]; then
      echo "Plugin is ready!"
      break
    fi
    echo "  Waiting... (${i}/30)"
    sleep 1
  done
  pause_if_debug
fi

# Test regex - email extraction
if should_run_step 6; then
  echo -e "\n[6/9] Testing regex_extract (emails)..."
  curl -s -X POST "${BASE_URL}/api/v1/invoke/tool" \
    -H "Content-Type: application/json" \
    -H "X-API-Key: ${API_KEY}" \
    -d '{
      "session_id": "'"${SESSION_ID}"'",
      "provider": "echo",
      "tool": "regex_extract",
      "parameters": {
        "content": "Contact us: test@example.com or support@domain.org",
        "expression": "[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,}"
      },
      "timeout": 30
    }' | jq
  pause_if_debug
fi

# Test regex - phone extraction
if should_run_step 7; then
  echo -e "\n[7/9] Testing regex_extract (phone numbers)..."
  curl -s -X POST "${BASE_URL}/api/v1/invoke/tool" \
    -H "Content-Type: application/json" \
    -H "X-API-Key: ${API_KEY}" \
    -d '{
      "session_id": "'"${SESSION_ID}"'",
      "provider": "echo",
      "tool": "regex_extract",
      "parameters": {
        "content": "Phone: 13800138000, 13900139000, 15012345678",
        "expression": "1[3-9]\\d{9}"
      },
      "timeout": 30
    }' | jq
  pause_if_debug
fi

# Test network - echo_http
if should_run_step 8; then
  echo -e "\n[8/9] Testing echo_http (network request)..."
  curl -s -X POST "${BASE_URL}/api/v1/invoke/tool" \
    -H "Content-Type: application/json" \
    -H "X-API-Key: ${API_KEY}" \
    -d '{
      "session_id": "'"${SESSION_ID}"'",
      "provider": "echo",
      "tool": "echo_http",
      "parameters": {
        "url": "https://httpbin.org/get",
        "message": "hello-from-curl-test"
      },
      "timeout": 60
    }' | jq
  pause_if_debug
fi

# Stop session
if should_run_step 9; then
  echo -e "\n[9/9] Stopping session..."
  curl -s -X POST "${BASE_URL}/api/v1/sessions/${SESSION_ID}/stop" \
    -H "X-API-Key: ${API_KEY}" | jq
  rm -f /tmp/plugin_session_id.txt
  pause_if_debug
fi

echo -e "\n=========================================="
echo "Test completed!"
echo "=========================================="
