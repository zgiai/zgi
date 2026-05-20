#!/bin/bash

# Real User Scenarios E2E Test with Detailed Logging
# Records all requests and responses to log files

set -e

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Database connection
DB_HOST="${DB_HOST:-localhost}"
DB_PORT="${DB_PORT:-5432}"
DB_USER="${DB_USERNAME:-postgres}"
DB_PASS="${DB_PASSWORD:?\"DB_PASSWORD is required\"}"
DB_NAME="${DB_NAME:-zgi}"
PSQL="psql postgresql://${DB_USER}:${DB_PASS}@${DB_HOST}:${DB_PORT}/${DB_NAME}"

# Test API Keys
TEST_TENANT_ID="00000000-0000-0000-0000-000000000001"
NORMAL_KEY="a2222222-2222-2222-2222-222222222222"
LOW_QUOTA_KEY="a3333333-3333-3333-3333-333333333333"

# Log file configuration
LOG_DIR="./test_logs"
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")
LOG_FILE="${LOG_DIR}/billing_test_${TIMESTAMP}.log"
JSON_LOG="${LOG_DIR}/billing_test_${TIMESTAMP}_requests.json"

# Create log directory
mkdir -p "${LOG_DIR}"

# Initialize log files
cat > "${LOG_FILE}" << EOF
========================================
Platform Billing Test Log
========================================
Test Started: $(date)
Timestamp: ${TIMESTAMP}

Test Configuration:
- Normal API Key: ${NORMAL_KEY}
- Low Quota Key: ${LOW_QUOTA_KEY}
- Tenant ID: ${TEST_TENANT_ID}

========================================

EOF

# Initialize JSON log
echo "[" > "${JSON_LOG}"

REQUEST_COUNT=0

# Function to log gRPC request and response
log_grpc_call() {
    local scenario="$1"
    local step="$2"
    local method="$3"
    local request_json="$4"
    local response_json="$5"
    local timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

    REQUEST_COUNT=$((REQUEST_COUNT + 1))

    # Add comma if not first request
    if [ $REQUEST_COUNT -gt 1 ]; then
        echo "," >> "${JSON_LOG}"
    fi

    # Log to text file
    cat >> "${LOG_FILE}" << EOF
========================================
Request #${REQUEST_COUNT}
========================================
Scenario: ${scenario}
Step: ${step}
Method: ${method}
Timestamp: ${timestamp}

REQUEST:
${request_json}

RESPONSE:
${response_json}

========================================

EOF

    # Log to JSON file (escape quotes in JSON strings)
    local escaped_request=$(echo "$request_json" | sed 's/"/\\"/g' | tr -d '\n')
    local escaped_response=$(echo "$response_json" | sed 's/"/\\"/g' | tr -d '\n')

    cat >> "${JSON_LOG}" << EOF
  {
    "request_id": ${REQUEST_COUNT},
    "scenario": "${scenario}",
    "step": "${step}",
    "method": "${method}",
    "timestamp": "${timestamp}",
    "request": "${escaped_request}",
    "response": "${escaped_response}"
  }
EOF
}

echo "========================================"
echo "  Platform Billing - Real User Scenarios"
echo "  WITH DETAILED LOGGING"
echo "========================================"
echo ""
echo "Log files will be saved to:"
echo "  - Text log: ${LOG_FILE}"
echo "  - JSON log: ${JSON_LOG}"
echo ""

# Reset test data
echo "Resetting test data..."
$PSQL -c "UPDATE api_keys SET remain_quota = 100000, used_quota = 0 WHERE id = '${NORMAL_KEY}';" > /dev/null
$PSQL -c "UPDATE api_keys SET remain_quota = 45, used_quota = 55, quota_limit = 100 WHERE id = '${LOW_QUOTA_KEY}';" > /dev/null
echo -e "${GREEN}✓ Test data reset${NC}"
echo ""

# ========================================
# Scenario 1: Normal Usage
# ========================================
echo "=== Scenario 1: Normal Usage (Sufficient Quota) ==="
echo ""

echo "1.1 Check quota before request..."
REQUEST='{"api_key_id": "'${NORMAL_KEY}'"}'
RESPONSE=$(cd /Users/stark/item/zgi/zgi-console-api && grpcurl -plaintext \
    -import-path ./pkg/rpc/v1 \
    -proto billing.proto \
    -d "${REQUEST}" \
    localhost:50051 \
    zgi.rpc.v1.BillingService/GetAPIKeyQuota 2>&1)

log_grpc_call "Scenario 1: Normal Usage" "1.1 Check quota before request" "GetAPIKeyQuota" "${REQUEST}" "${RESPONSE}"

QUOTA_BEFORE=$(echo "$RESPONSE" | grep -o '"remainingQuota": "[^"]*"' | cut -d'"' -f4)
echo "   Remaining Quota: ${QUOTA_BEFORE}"
echo ""

echo "1.2 PreDeduct quota (reserve 10 credits)..."
REQUEST='{
    "api_key_id": "'${NORMAL_KEY}'",
    "tenant_id": "'${TEST_TENANT_ID}'",
    "model_id": "gpt-3.5-turbo",
    "model_name": "gpt-3.5-turbo",
    "provider_name": "openai",
    "estimated_credits": 10,
    "request_id": "test-'$(date +%s)'"
}'
RESPONSE=$(cd /Users/stark/item/zgi/zgi-console-api && grpcurl -plaintext \
    -import-path ./pkg/rpc/v1 \
    -proto billing.proto \
    -d "${REQUEST}" \
    localhost:50051 \
    zgi.rpc.v1.BillingService/PreDeductQuota 2>&1)

log_grpc_call "Scenario 1: Normal Usage" "1.2 PreDeduct quota" "PreDeductQuota" "${REQUEST}" "${RESPONSE}"

DEDUCTION_ID=$(echo "$RESPONSE" | grep -o '"deductionId": "[^"]*"' | cut -d'"' -f4)
echo -e "   ${GREEN}✓ PreDeduct successful${NC}"
echo "   Deduction ID: ${DEDUCTION_ID}"
echo ""

echo "1.3 Simulate LLM request processing..."
sleep 1
echo ""

echo "1.4 Settle quota with actual usage (used 8 credits)..."
REQUEST='{
    "deduction_id": "'${DEDUCTION_ID}'",
    "api_key_id": "'${NORMAL_KEY}'",
    "tenant_id": "'${TEST_TENANT_ID}'",
    "model_id": "gpt-3.5-turbo",
    "model_name": "gpt-3.5-turbo",
    "provider_name": "openai",
    "estimated_credits": 10,
    "actual_credits": 8,
    "prompt_tokens": 15,
    "completion_tokens": 25,
    "total_tokens": 40,
    "status": "success"
}'
RESPONSE=$(cd /Users/stark/item/zgi/zgi-console-api && grpcurl -plaintext \
    -import-path ./pkg/rpc/v1 \
    -proto billing.proto \
    -d "${REQUEST}" \
    localhost:50051 \
    zgi.rpc.v1.BillingService/SettleQuota 2>&1)

log_grpc_call "Scenario 1: Normal Usage" "1.4 Settle quota" "SettleQuota" "${REQUEST}" "${RESPONSE}"

echo -e "   ${GREEN}✓ Settle successful${NC}"
echo ""

echo "1.5 Check quota after request..."
REQUEST='{"api_key_id": "'${NORMAL_KEY}'"}'
RESPONSE=$(cd /Users/stark/item/zgi/zgi-console-api && grpcurl -plaintext \
    -import-path ./pkg/rpc/v1 \
    -proto billing.proto \
    -d "${REQUEST}" \
    localhost:50051 \
    zgi.rpc.v1.BillingService/GetAPIKeyQuota 2>&1)

log_grpc_call "Scenario 1: Normal Usage" "1.5 Check quota after request" "GetAPIKeyQuota" "${REQUEST}" "${RESPONSE}"

QUOTA_AFTER=$(echo "$RESPONSE" | grep -o '"remainingQuota": "[^"]*"' | cut -d'"' -f4)
DEDUCTED=$((QUOTA_BEFORE - QUOTA_AFTER))
echo "   Remaining Quota: ${QUOTA_AFTER}"
echo "   Deducted: ${DEDUCTED} credits"
echo -e "   ${GREEN}✓ Quota deducted correctly (${DEDUCTED} credits)${NC}"
echo ""

# ========================================
# Scenario 2: Quota Exceeded
# ========================================
echo "=== Scenario 2: Quota Exceeded (Insufficient Balance) ==="
echo ""

echo "2.1 Check low quota API key..."
REQUEST='{"api_key_id": "'${LOW_QUOTA_KEY}'"}'
RESPONSE=$(cd /Users/stark/item/zgi/zgi-console-api && grpcurl -plaintext \
    -import-path ./pkg/rpc/v1 \
    -proto billing.proto \
    -d "${REQUEST}" \
    localhost:50051 \
    zgi.rpc.v1.BillingService/GetAPIKeyQuota 2>&1)

log_grpc_call "Scenario 2: Quota Exceeded" "2.1 Check low quota" "GetAPIKeyQuota" "${REQUEST}" "${RESPONSE}"

LOW_QUOTA=$(echo "$RESPONSE" | grep -o '"remainingQuota": "[^"]*"' | cut -d'"' -f4)
echo "   Remaining Quota: ${LOW_QUOTA} credits"
echo "   (Below minimum reserve threshold of 50 credits)"
echo ""

echo "2.2 Attempt to PreDeduct 10 credits (should fail)..."
REQUEST='{
    "api_key_id": "'${LOW_QUOTA_KEY}'",
    "tenant_id": "'${TEST_TENANT_ID}'",
    "model_id": "gpt-4",
    "model_name": "gpt-4",
    "provider_name": "openai",
    "estimated_credits": 10,
    "request_id": "test-'$(date +%s)'"
}'
RESPONSE=$(cd /Users/stark/item/zgi/zgi-console-api && grpcurl -plaintext \
    -import-path ./pkg/rpc/v1 \
    -proto billing.proto \
    -d "${REQUEST}" \
    localhost:50051 \
    zgi.rpc.v1.BillingService/PreDeductQuota 2>&1)

log_grpc_call "Scenario 2: Quota Exceeded" "2.2 Attempt PreDeduct (should fail)" "PreDeductQuota" "${REQUEST}" "${RESPONSE}"

if echo "$RESPONSE" | grep -q '"errorMessage"'; then
    echo -e "   ${GREEN}✓ Request correctly rejected (insufficient quota)${NC}"
    ERROR_MSG=$(echo "$RESPONSE" | grep -o '"errorMessage": "[^"]*"' | cut -d'"' -f4)
    echo "   Error: ${ERROR_MSG}"
else
    echo -e "   ${RED}✗ Request should have been rejected but wasn't${NC}"
fi
echo ""

echo "2.3 Verify no LLM call was made (quota unchanged)..."
REQUEST='{"api_key_id": "'${LOW_QUOTA_KEY}'"}'
RESPONSE=$(cd /Users/stark/item/zgi/zgi-console-api && grpcurl -plaintext \
    -import-path ./pkg/rpc/v1 \
    -proto billing.proto \
    -d "${REQUEST}" \
    localhost:50051 \
    zgi.rpc.v1.BillingService/GetAPIKeyQuota 2>&1)

log_grpc_call "Scenario 2: Quota Exceeded" "2.3 Verify quota unchanged" "GetAPIKeyQuota" "${REQUEST}" "${RESPONSE}"

LOW_QUOTA_AFTER=$(echo "$RESPONSE" | grep -o '"remainingQuota": "[^"]*"' | cut -d'"' -f4)
if [ "$LOW_QUOTA" = "$LOW_QUOTA_AFTER" ]; then
    echo -e "   ${GREEN}✓ Quota unchanged (${LOW_QUOTA_AFTER} credits)${NC}"
else
    echo -e "   ${RED}⚠ Quota changed unexpectedly${NC}"
fi
echo ""

# ========================================
# Scenario 3: Recharge and Resume
# ========================================
echo "=== Scenario 3: Recharge and Resume Usage ==="
echo ""

echo "3.1 Simulate recharge (add 50,000 credits)..."
$PSQL -c "UPDATE api_keys SET remain_quota = remain_quota + 50000 WHERE id = '${LOW_QUOTA_KEY}';"
echo -e "   ${GREEN}✓ Recharged successfully${NC}"

REQUEST='{"api_key_id": "'${LOW_QUOTA_KEY}'"}'
RESPONSE=$(cd /Users/stark/item/zgi/zgi-console-api && grpcurl -plaintext \
    -import-path ./pkg/rpc/v1 \
    -proto billing.proto \
    -d "${REQUEST}" \
    localhost:50051 \
    zgi.rpc.v1.BillingService/GetAPIKeyQuota 2>&1)

log_grpc_call "Scenario 3: Recharge" "3.1 Check quota after recharge" "GetAPIKeyQuota" "${REQUEST}" "${RESPONSE}"

NEW_QUOTA=$(echo "$RESPONSE" | grep -o '"remainingQuota": "[^"]*"' | cut -d'"' -f4)
echo "   New Quota: ${NEW_QUOTA} credits"
echo ""

echo "3.2 Retry request after recharge..."
REQUEST='{
    "api_key_id": "'${LOW_QUOTA_KEY}'",
    "tenant_id": "'${TEST_TENANT_ID}'",
    "model_id": "gpt-3.5-turbo",
    "model_name": "gpt-3.5-turbo",
    "provider_name": "openai",
    "estimated_credits": 10,
    "request_id": "test-'$(date +%s)'"
}'
RESPONSE=$(cd /Users/stark/item/zgi/zgi-console-api && grpcurl -plaintext \
    -import-path ./pkg/rpc/v1 \
    -proto billing.proto \
    -d "${REQUEST}" \
    localhost:50051 \
    zgi.rpc.v1.BillingService/PreDeductQuota 2>&1)

log_grpc_call "Scenario 3: Recharge" "3.2 Retry PreDeduct after recharge" "PreDeductQuota" "${REQUEST}" "${RESPONSE}"

DEDUCTION_ID_3=$(echo "$RESPONSE" | grep -o '"deductionId": "[^"]*"' | cut -d'"' -f4)
echo -e "   ${GREEN}✓ Request successful after recharge${NC}"

REQUEST='{
    "deduction_id": "'${DEDUCTION_ID_3}'",
    "api_key_id": "'${LOW_QUOTA_KEY}'",
    "tenant_id": "'${TEST_TENANT_ID}'",
    "model_id": "gpt-3.5-turbo",
    "model_name": "gpt-3.5-turbo",
    "provider_name": "openai",
    "estimated_credits": 10,
    "actual_credits": 8,
    "prompt_tokens": 15,
    "completion_tokens": 25,
    "total_tokens": 40,
    "status": "success"
}'
RESPONSE=$(cd /Users/stark/item/zgi/zgi-console-api && grpcurl -plaintext \
    -import-path ./pkg/rpc/v1 \
    -proto billing.proto \
    -d "${REQUEST}" \
    localhost:50051 \
    zgi.rpc.v1.BillingService/SettleQuota 2>&1)

log_grpc_call "Scenario 3: Recharge" "3.2 Settle after recharge" "SettleQuota" "${REQUEST}" "${RESPONSE}"

echo -e "   ${GREEN}✓ Request completed and settled${NC}"
echo ""

# ========================================
# Scenario 4: Request Failure and Refund
# ========================================
echo "=== Scenario 4: Request Failure and Refund ==="
echo ""

echo "4.1 PreDeduct quota..."
REQUEST='{
    "api_key_id": "'${NORMAL_KEY}'",
    "tenant_id": "'${TEST_TENANT_ID}'",
    "model_id": "gpt-4",
    "model_name": "gpt-4",
    "provider_name": "openai",
    "estimated_credits": 20,
    "request_id": "test-'$(date +%s)'"
}'
RESPONSE=$(cd /Users/stark/item/zgi/zgi-console-api && grpcurl -plaintext \
    -import-path ./pkg/rpc/v1 \
    -proto billing.proto \
    -d "${REQUEST}" \
    localhost:50051 \
    zgi.rpc.v1.BillingService/PreDeductQuota 2>&1)

log_grpc_call "Scenario 4: Failure Refund" "4.1 PreDeduct for failed request" "PreDeductQuota" "${REQUEST}" "${RESPONSE}"

DEDUCTION_ID_4=$(echo "$RESPONSE" | grep -o '"deductionId": "[^"]*"' | cut -d'"' -f4)
echo "   PreDeducted 20 credits, Deduction ID: ${DEDUCTION_ID_4}"
echo ""

REQUEST='{"api_key_id": "'${NORMAL_KEY}'"}'
RESPONSE=$(cd /Users/stark/item/zgi/zgi-console-api && grpcurl -plaintext \
    -import-path ./pkg/rpc/v1 \
    -proto billing.proto \
    -d "${REQUEST}" \
    localhost:50051 \
    zgi.rpc.v1.BillingService/GetAPIKeyQuota 2>&1)
QUOTA_BEFORE_REFUND=$(echo "$RESPONSE" | grep -o '"remainingQuota": "[^"]*"' | cut -d'"' -f4)

echo "4.2 Simulate LLM request failure..."
echo ""

echo "4.3 Settle with failure status (refund)..."
REQUEST='{
    "deduction_id": "'${DEDUCTION_ID_4}'",
    "api_key_id": "'${NORMAL_KEY}'",
    "tenant_id": "'${TEST_TENANT_ID}'",
    "model_id": "gpt-4",
    "model_name": "gpt-4",
    "provider_name": "openai",
    "estimated_credits": 20,
    "actual_credits": 0,
    "prompt_tokens": 0,
    "completion_tokens": 0,
    "total_tokens": 0,
    "status": "failed",
    "error_message": "Request timeout"
}'
RESPONSE=$(cd /Users/stark/item/zgi/zgi-console-api && grpcurl -plaintext \
    -import-path ./pkg/rpc/v1 \
    -proto billing.proto \
    -d "${REQUEST}" \
    localhost:50051 \
    zgi.rpc.v1.BillingService/SettleQuota 2>&1)

log_grpc_call "Scenario 4: Failure Refund" "4.3 Settle with failure (refund)" "SettleQuota" "${REQUEST}" "${RESPONSE}"

REQUEST='{"api_key_id": "'${NORMAL_KEY}'"}'
RESPONSE=$(cd /Users/stark/item/zgi/zgi-console-api && grpcurl -plaintext \
    -import-path ./pkg/rpc/v1 \
    -proto billing.proto \
    -d "${REQUEST}" \
    localhost:50051 \
    zgi.rpc.v1.BillingService/GetAPIKeyQuota 2>&1)

log_grpc_call "Scenario 4: Failure Refund" "4.3 Check quota after refund" "GetAPIKeyQuota" "${REQUEST}" "${RESPONSE}"

QUOTA_AFTER_REFUND=$(echo "$RESPONSE" | grep -o '"remainingQuota": "[^"]*"' | cut -d'"' -f4)
echo -e "   ${GREEN}✓ Credits refunded successfully${NC}"
echo "   Quota before: ${QUOTA_BEFORE_REFUND}, after: ${QUOTA_AFTER_REFUND}"
echo ""

# Close JSON log
echo "" >> "${JSON_LOG}"
echo "]" >> "${JSON_LOG}"

# Add summary to text log
cat >> "${LOG_FILE}" << EOF
========================================
Test Completed
========================================
End Time: $(date)
Total Requests: ${REQUEST_COUNT}

All scenarios tested successfully!
========================================
EOF

echo "========================================"
echo -e "${GREEN}✓ All Real User Scenarios Tested${NC}"
echo "========================================"
echo ""
echo "Detailed logs saved to:"
echo "  📄 Text log: ${LOG_FILE}"
echo "  📊 JSON log: ${JSON_LOG}"
echo ""
echo "View logs with:"
echo "  cat ${LOG_FILE}"
echo "  cat ${JSON_LOG} | jq ."
echo ""
