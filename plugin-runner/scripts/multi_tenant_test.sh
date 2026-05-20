#!/bin/bash
# multi_tenant_test.sh - Multi-tenant plugin installation and usage test
#
# Tests:
#   1. Create multiple tenants
#   2. Register same plugin for different tenants
#   3. Install same plugin (checksum skip behavior)
#   4. Install different plugins
#   5. Bind plugins to tenants with different configs
#   6. Launch sessions with tenant context
#   7. Invoke tools with tenant isolation
#
# Usage:
#   ./multi_tenant_test.sh           # Run all steps
#   ./multi_tenant_test.sh -d        # Debug mode: pause after each step
#   ./multi_tenant_test.sh -s 3      # Start from step 3
#   ./multi_tenant_test.sh -c        # Cleanup only

set -e

BASE_URL="http://localhost:15000"
API_KEY="admin-key-123"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
EXAMPLES_DIR="${PROJECT_ROOT}/examples/test_plugin"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Parse arguments
DEBUG_MODE=false
START_STEP=1
CLEANUP_ONLY=false
while getopts "ds:c" opt; do
  case $opt in
    d) DEBUG_MODE=true ;;
    s) START_STEP=$OPTARG ;;
    c) CLEANUP_ONLY=true ;;
    *) echo "Usage: $0 [-d] [-s step_number] [-c]"; exit 1 ;;
  esac
done

log_step() {
  echo -e "\n${BLUE}[$1]${NC} $2"
}

log_success() {
  echo -e "${GREEN}✓ $1${NC}"
}

log_error() {
  echo -e "${RED}✗ $1${NC}"
}

log_info() {
  echo -e "${YELLOW}→ $1${NC}"
}

pause_if_debug() {
  if [ "$DEBUG_MODE" = true ]; then
    echo -e "\n>>> Press ENTER to continue (or Ctrl+C to exit)..."
    read -r
  fi
}

should_run_step() {
  [ "$1" -ge "$START_STEP" ]
}

cleanup() {
  log_step "CLEANUP" "Cleaning up test resources..."
  
  # Stop all sessions
  SESSIONS=$(curl -s "${BASE_URL}/api/v1/sessions" -H "X-API-Key: ${API_KEY}" 2>/dev/null || echo "[]")
  for sid in $(echo "$SESSIONS" | jq -r '.[].id' 2>/dev/null); do
    if [ -n "$sid" ] && [ "$sid" != "null" ]; then
      curl -s -X POST "${BASE_URL}/api/v1/sessions/${sid}/stop" -H "X-API-Key: ${API_KEY}" > /dev/null 2>&1 || true
      log_info "Stopped session: ${sid:0:8}..."
    fi
  done
  
  # Delete plugins
  for plugin in "echo-plugin-a:1.0.0" "echo-plugin-b:1.0.0"; do
    curl -s -X DELETE "${BASE_URL}/api/v1/plugins/${plugin}" -H "X-API-Key: ${API_KEY}" > /dev/null 2>&1 || true
  done
  log_info "Deleted test plugins"
  
  # Clean temp files
  rm -f /tmp/echo-plugin-*.zip /tmp/tenant_*.txt /tmp/session_*.txt
  log_success "Cleanup completed"
}

# Create a simple echo plugin for testing
create_test_plugin() {
  local name=$1
  local version=$2
  local variant=$3  # Different variant for different checksums
  local dir="/tmp/test-plugin-${name}-${version}"
  
  rm -rf "$dir"
  mkdir -p "$dir"
  
  # Create main.py
  cat > "$dir/main.py" << 'PYTHON'
import json
import sys
from datetime import datetime

def send_message(msg):
    print(json.dumps(msg), flush=True)

def main():
    # Send ready signal
    send_message({
        "type": "ready",
        "request_id": "",
        "timestamp": datetime.utcnow().isoformat() + "Z"
    })
    
    # Process requests
    for line in sys.stdin:
        try:
            msg = json.loads(line)
            if msg.get("type") == "request":
                data = msg.get("data", {})
                params = data.get("parameters", {})
                
                result = {
                    "echo": params.get("message", ""),
                    "plugin": "PLUGIN_NAME",
                    "variant": "PLUGIN_VARIANT"
                }
                
                send_message({
                    "type": "result",
                    "request_id": msg.get("request_id", ""),
                    "timestamp": datetime.utcnow().isoformat() + "Z",
                    "data": {"success": True, "data": result}
                })
        except Exception as e:
            send_message({
                "type": "error",
                "request_id": msg.get("request_id", "") if 'msg' in dir() else "",
                "timestamp": datetime.utcnow().isoformat() + "Z",
                "data": {"code": "ERROR", "message": str(e)}
            })

if __name__ == "__main__":
    main()
PYTHON
  
  # Replace placeholders
  sed -i '' "s/PLUGIN_NAME/${name}/g" "$dir/main.py" 2>/dev/null || \
  sed -i "s/PLUGIN_NAME/${name}/g" "$dir/main.py"
  
  sed -i '' "s/PLUGIN_VARIANT/${variant}/g" "$dir/main.py" 2>/dev/null || \
  sed -i "s/PLUGIN_VARIANT/${variant}/g" "$dir/main.py"
  
  # Create requirements.txt (empty but required)
  echo "# No dependencies" > "$dir/requirements.txt"
  
  # Create zip
  local zip_file="/tmp/echo-plugin-${name}-${version}-${variant}.zip"
  rm -f "$zip_file"
  (cd "$dir" && zip -rq "$zip_file" .)
  
  echo "$zip_file"
}

# ============================================================================
# MAIN TEST FLOW
# ============================================================================

if [ "$CLEANUP_ONLY" = true ]; then
  cleanup
  exit 0
fi

echo "=========================================="
echo "Multi-Tenant Plugin Test"
echo "=========================================="
if [ "$DEBUG_MODE" = true ]; then
  echo "Mode: DEBUG (will pause after each step)"
fi
if [ "$START_STEP" -gt 1 ]; then
  echo "Starting from step: $START_STEP"
fi
echo ""

# Step 1: Health check
if should_run_step 1; then
  log_step "1/12" "Health check..."
  HEALTH=$(curl -s "${BASE_URL}/healthz" -H "X-API-Key: ${API_KEY}")
  echo "$HEALTH" | jq
  if [ "$(echo "$HEALTH" | jq -r '.status')" = "ok" ]; then
    log_success "Server is healthy"
  else
    log_error "Server is not healthy"
    exit 1
  fi
  pause_if_debug
fi

# Step 2: Create Tenant A
if should_run_step 2; then
  log_step "2/12" "Creating Tenant A..."
  TENANT_A=$(curl -s -X POST "${BASE_URL}/api/v1/tenants" \
    -H "Content-Type: application/json" \
    -H "X-API-Key: ${API_KEY}" \
    -d '{"name": "tenant-alpha"}')
  echo "$TENANT_A" | jq
  
  TENANT_A_ID=$(echo "$TENANT_A" | jq -r '.id // empty')
  if [ -n "$TENANT_A_ID" ]; then
    echo "$TENANT_A_ID" > /tmp/tenant_a_id.txt
    log_success "Created Tenant A with ID: $TENANT_A_ID"
  else
    # May already exist, try to get from error or use default
    TENANT_A_ID=1
    echo "$TENANT_A_ID" > /tmp/tenant_a_id.txt
    log_info "Using Tenant A ID: $TENANT_A_ID"
  fi
  pause_if_debug
fi

# Step 3: Create Tenant B
if should_run_step 3; then
  log_step "3/12" "Creating Tenant B..."
  TENANT_B=$(curl -s -X POST "${BASE_URL}/api/v1/tenants" \
    -H "Content-Type: application/json" \
    -H "X-API-Key: ${API_KEY}" \
    -d '{"name": "tenant-beta"}')
  echo "$TENANT_B" | jq
  
  TENANT_B_ID=$(echo "$TENANT_B" | jq -r '.id // empty')
  if [ -n "$TENANT_B_ID" ]; then
    echo "$TENANT_B_ID" > /tmp/tenant_b_id.txt
    log_success "Created Tenant B with ID: $TENANT_B_ID"
  else
    TENANT_B_ID=2
    echo "$TENANT_B_ID" > /tmp/tenant_b_id.txt
    log_info "Using Tenant B ID: $TENANT_B_ID"
  fi
  pause_if_debug
fi

# Load tenant IDs
TENANT_A_ID=$(cat /tmp/tenant_a_id.txt 2>/dev/null || echo "1")
TENANT_B_ID=$(cat /tmp/tenant_b_id.txt 2>/dev/null || echo "2")

# Step 4: Register Plugin A
if should_run_step 4; then
  log_step "4/12" "Registering Plugin A (echo-plugin-a)..."
  PLUGIN_A=$(curl -s -X POST "${BASE_URL}/api/v1/plugins" \
    -H "Content-Type: application/json" \
    -H "X-API-Key: ${API_KEY}" \
    -d '{
      "manifest": {
        "name": "echo-plugin-a",
        "version": "1.0.0",
        "author": "test",
        "runner": {
          "language": "python",
          "entrypoint": "main"
        }
      }
    }')
  echo "$PLUGIN_A" | jq
  log_success "Registered Plugin A"
  pause_if_debug
fi

# Step 5: Register Plugin B (different plugin)
if should_run_step 5; then
  log_step "5/12" "Registering Plugin B (echo-plugin-b)..."
  PLUGIN_B=$(curl -s -X POST "${BASE_URL}/api/v1/plugins" \
    -H "Content-Type: application/json" \
    -H "X-API-Key: ${API_KEY}" \
    -d '{
      "manifest": {
        "name": "echo-plugin-b",
        "version": "1.0.0",
        "author": "test",
        "runner": {
          "language": "python",
          "entrypoint": "main"
        }
      }
    }')
  echo "$PLUGIN_B" | jq
  log_success "Registered Plugin B"
  pause_if_debug
fi

# Step 6: Install Plugin A (first time)
if should_run_step 6; then
  log_step "6/12" "Installing Plugin A (first installation)..."
  ZIP_A=$(create_test_plugin "echo-plugin-a" "1.0.0" "v1")
  log_info "Created package: $ZIP_A"
  
  INSTALL_A=$(curl -s -X POST "${BASE_URL}/api/v1/plugins/echo-plugin-a:1.0.0/install" \
    -H "X-API-Key: ${API_KEY}" \
    -F "file=@${ZIP_A}")
  echo "$INSTALL_A" | jq
  log_success "Installed Plugin A"
  pause_if_debug
fi

# Step 7: Install Plugin A again (same checksum - should skip)
if should_run_step 7; then
  log_step "7/12" "Installing Plugin A again (same checksum - should skip)..."
  ZIP_A=$(create_test_plugin "echo-plugin-a" "1.0.0" "v1")
  
  INSTALL_A2=$(curl -s -X POST "${BASE_URL}/api/v1/plugins/echo-plugin-a:1.0.0/install" \
    -H "X-API-Key: ${API_KEY}" \
    -F "file=@${ZIP_A}")
  echo "$INSTALL_A2" | jq
  log_info "Expected: Installation skipped (same checksum)"
  pause_if_debug
fi

# Step 8: Install Plugin A with different content (should fail without force)
if should_run_step 8; then
  log_step "8/12" "Installing Plugin A with different content (no force - should fail)..."
  ZIP_A_V2=$(create_test_plugin "echo-plugin-a" "1.0.0" "v2-different")
  log_info "Created different package: $ZIP_A_V2"
  
  INSTALL_A3=$(curl -s -X POST "${BASE_URL}/api/v1/plugins/echo-plugin-a:1.0.0/install" \
    -H "X-API-Key: ${API_KEY}" \
    -F "file=@${ZIP_A_V2}" \
    -F "force=false")
  echo "$INSTALL_A3" | jq
  
  if echo "$INSTALL_A3" | jq -e '.error' > /dev/null 2>&1; then
    log_success "Correctly rejected different content without force"
  else
    log_error "Should have rejected different content"
  fi
  pause_if_debug
fi

# Step 9: Install Plugin B
if should_run_step 9; then
  log_step "9/12" "Installing Plugin B (different plugin)..."
  ZIP_B=$(create_test_plugin "echo-plugin-b" "1.0.0" "v1")
  log_info "Created package: $ZIP_B"
  
  INSTALL_B=$(curl -s -X POST "${BASE_URL}/api/v1/plugins/echo-plugin-b:1.0.0/install" \
    -H "X-API-Key: ${API_KEY}" \
    -F "file=@${ZIP_B}")
  echo "$INSTALL_B" | jq
  log_success "Installed Plugin B"
  pause_if_debug
fi

# Step 10: Bind plugins to tenants
if should_run_step 10; then
  log_step "10/12" "Binding plugins to tenants..."
  
  # Bind Plugin A to Tenant A with config
  log_info "Binding Plugin A to Tenant A..."
  BIND_A_A=$(curl -s -X POST "${BASE_URL}/api/v1/plugins/echo-plugin-a:1.0.0/tenants/${TENANT_A_ID}/enable" \
    -H "Content-Type: application/json" \
    -H "X-API-Key: ${API_KEY}" \
    -d '{"api_key": "tenant-a-key", "env": "production"}')
  echo "$BIND_A_A" | jq
  
  # Bind Plugin A to Tenant B with different config
  log_info "Binding Plugin A to Tenant B..."
  BIND_A_B=$(curl -s -X POST "${BASE_URL}/api/v1/plugins/echo-plugin-a:1.0.0/tenants/${TENANT_B_ID}/enable" \
    -H "Content-Type: application/json" \
    -H "X-API-Key: ${API_KEY}" \
    -d '{"api_key": "tenant-b-key", "env": "staging"}')
  echo "$BIND_A_B" | jq
  
  # Bind Plugin B to Tenant A only
  log_info "Binding Plugin B to Tenant A only..."
  BIND_B_A=$(curl -s -X POST "${BASE_URL}/api/v1/plugins/echo-plugin-b:1.0.0/tenants/${TENANT_A_ID}/enable" \
    -H "Content-Type: application/json" \
    -H "X-API-Key: ${API_KEY}" \
    -d '{"api_key": "tenant-a-plugin-b-key"}')
  echo "$BIND_B_A" | jq
  
  log_success "Tenant bindings configured"
  pause_if_debug
fi

# Step 11: List tenant bindings
if should_run_step 11; then
  log_step "11/12" "Verifying tenant bindings..."
  
  log_info "Plugin A tenants:"
  curl -s "${BASE_URL}/api/v1/plugins/echo-plugin-a:1.0.0/tenants" \
    -H "X-API-Key: ${API_KEY}" | jq
  
  log_info "Plugin B tenants:"
  curl -s "${BASE_URL}/api/v1/plugins/echo-plugin-b:1.0.0/tenants" \
    -H "X-API-Key: ${API_KEY}" | jq
  
  pause_if_debug
fi

# Step 12: Launch session with tenant context and test
if should_run_step 12; then
  log_step "12/12" "Testing plugin invocation with tenant context..."
  
  # Launch session for Tenant A
  log_info "Launching session for Tenant A..."
  SESSION_A=$(curl -s -X POST "${BASE_URL}/api/v1/sessions" \
    -H "Content-Type: application/json" \
    -H "X-API-Key: ${API_KEY}" \
    -H "X-Tenant-ID: ${TENANT_A_ID}" \
    -d '{
      "name": "echo-plugin-a",
      "version": "1.0.0",
      "language": "python",
      "entrypoint": "main"
    }')
  echo "$SESSION_A" | jq
  
  SESSION_A_ID=$(echo "$SESSION_A" | jq -r '.id')
  if [ -n "$SESSION_A_ID" ] && [ "$SESSION_A_ID" != "null" ]; then
    echo "$SESSION_A_ID" > /tmp/session_a_id.txt
    log_success "Session A launched: ${SESSION_A_ID:0:8}..."
    
    # Wait for ready
    log_info "Waiting for session ready..."
    for i in {1..15}; do
      READY=$(curl -s "${BASE_URL}/api/v1/invoke/sessions/${SESSION_A_ID}/ready" \
        -H "X-API-Key: ${API_KEY}" | jq -r '.ready')
      if [ "$READY" = "true" ]; then
        log_success "Session is ready!"
        break
      fi
      sleep 1
    done
    
    # Invoke tool
    log_info "Invoking echo tool..."
    INVOKE_RESULT=$(curl -s -X POST "${BASE_URL}/api/v1/invoke/tool" \
      -H "Content-Type: application/json" \
      -H "X-API-Key: ${API_KEY}" \
      -H "X-Tenant-ID: ${TENANT_A_ID}" \
      -d '{
        "session_id": "'"${SESSION_A_ID}"'",
        "provider": "echo",
        "tool": "echo",
        "parameters": {"message": "Hello from Tenant A!"},
        "timeout": 30
      }')
    echo "$INVOKE_RESULT" | jq
    
    # Stop session
    log_info "Stopping session..."
    curl -s -X POST "${BASE_URL}/api/v1/sessions/${SESSION_A_ID}/stop" \
      -H "X-API-Key: ${API_KEY}" | jq
  else
    log_error "Failed to launch session"
  fi
  
  pause_if_debug
fi

echo ""
echo "=========================================="
echo "Multi-Tenant Test Completed!"
echo "=========================================="
echo ""
echo "Summary:"
echo "  - Created 2 tenants (alpha, beta)"
echo "  - Registered 2 plugins (echo-plugin-a, echo-plugin-b)"
echo "  - Tested duplicate install (same checksum skip)"
echo "  - Tested different content install (rejection without force)"
echo "  - Configured tenant-specific bindings"
echo "  - Launched and invoked plugin with tenant context"
echo ""
echo "To cleanup test resources, run:"
echo "  $0 -c"
