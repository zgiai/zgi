#!/bin/bash

# Final Platform Billing Integration Test
# Tests the complete billing flow via gRPC

set -e

echo "=== Platform Billing Integration - Final Test ==="
echo ""

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Test counters
PASSED=0
FAILED=0

# Test 1: Console API gRPC Server Status
echo "Test 1: Checking Console API gRPC Server..."
if lsof -i :50051 > /dev/null 2>&1; then
    echo -e "${GREEN}✓ Console API gRPC server is running${NC}"
    ((PASSED++))
else
    echo -e "${RED}✗ Console API gRPC server is NOT running${NC}"
    ((FAILED++))
    exit 1
fi

echo ""

# Test 2: zgi-api Server Status
echo "Test 2: Checking zgi-api Server..."
if lsof -i :2620 > /dev/null 2>&1; then
    echo -e "${GREEN}✓ zgi-api server is running on port 2620${NC}"
    ((PASSED++))
else
    echo -e "${RED}✗ zgi-api server is NOT running${NC}"
    ((FAILED++))
fi

echo ""

# Test 3: GetAPIKeyQuota via gRPC
echo "Test 3: Testing GetAPIKeyQuota..."
cd /Users/stark/item/zgi/zgi-console-api
RESULT=$(grpcurl -plaintext \
    -import-path ./pkg/rpc/v1 \
    -proto billing.proto \
    -d '{"api_key_id": "a2222222-2222-2222-2222-222222222222"}' \
    localhost:50051 \
    zgi.rpc.v1.BillingService/GetAPIKeyQuota 2>&1)

if echo "$RESULT" | grep -q '"success": true'; then
    echo -e "${GREEN}✓ GetAPIKeyQuota successful${NC}"
    REMAINING=$(echo "$RESULT" | grep -o '"remainingQuota": "[^"]*"' | cut -d'"' -f4)
    echo "  Remaining Quota: $REMAINING"
    ((PASSED++))
else
    echo -e "${RED}✗ GetAPIKeyQuota failed${NC}"
    echo "$RESULT"
    ((FAILED++))
fi

echo ""

# Test 4: PreDeductQuota via gRPC
echo "Test 4: Testing PreDeductQuota..."
RESULT=$(grpcurl -plaintext \
    -import-path ./pkg/rpc/v1 \
    -proto billing.proto \
    -d '{
        "api_key_id": "a2222222-2222-2222-2222-222222222222",
        "tenant_id": "00000000-0000-0000-0000-000000000001",
        "model_id": "gpt-3.5-turbo",
        "model_name": "gpt-3.5-turbo",
        "provider_name": "openai",
        "estimated_credits": 5,
        "request_id": "test-'$(date +%s)'"
    }' \
    localhost:50051 \
    zgi.rpc.v1.BillingService/PreDeductQuota 2>&1)

if echo "$RESULT" | grep -q '"success": true'; then
    echo -e "${GREEN}✓ PreDeductQuota successful${NC}"
    DEDUCTION_ID=$(echo "$RESULT" | grep -o '"deductionId": "[^"]*"' | cut -d'"' -f4)
    echo "  Deduction ID: $DEDUCTION_ID"
    ((PASSED++))
    
    # Test 5: SettleQuota via gRPC
    echo ""
    echo "Test 5: Testing SettleQuota..."
    SETTLE_RESULT=$(grpcurl -plaintext \
        -import-path ./pkg/rpc/v1 \
        -proto billing.proto \
        -d '{
            "deduction_id": "'"$DEDUCTION_ID"'",
            "api_key_id": "a2222222-2222-2222-2222-222222222222",
            "tenant_id": "00000000-0000-0000-0000-000000000001",
            "model_name": "gpt-3.5-turbo",
            "provider_name": "openai",
            "estimated_credits": 5,
            "actual_credits": 3,
            "prompt_tokens": 10,
            "completion_tokens": 15,
            "total_tokens": 25,
            "status": "success",
            "request_id": "test-'$(date +%s)'"
        }' \
        localhost:50051 \
        zgi.rpc.v1.BillingService/SettleQuota 2>&1)
    
    if echo "$SETTLE_RESULT" | grep -q '"success": true'; then
        echo -e "${GREEN}✓ SettleQuota successful${NC}"
        ((PASSED++))
    else
        echo -e "${RED}✗ SettleQuota failed${NC}"
        echo "$SETTLE_RESULT"
        ((FAILED++))
    fi
else
    echo -e "${RED}✗ PreDeductQuota failed${NC}"
    echo "$RESULT"
    ((FAILED++))
fi

echo ""

# Test 6: Go Unit Tests
echo "Test 6: Running Go Unit Tests..."
cd /Users/stark/item/zgi/zgi-api
if go test -v ./tests/integration/llm/platform_billing_integration_test.go 2>&1 | grep -q "PASS"; then
    echo -e "${GREEN}✓ All Go unit tests passed${NC}"
    ((PASSED++))
else
    echo -e "${RED}✗ Some Go unit tests failed${NC}"
    ((FAILED++))
fi

echo ""
echo "=== Test Summary ==="
echo -e "Passed: ${GREEN}$PASSED${NC}"
echo -e "Failed: ${RED}$FAILED${NC}"
echo ""

if [ $FAILED -eq 0 ]; then
    echo -e "${GREEN}✓✓✓ All tests passed! Platform Billing Integration is working correctly!${NC}"
    echo ""
    echo "Integration Status:"
    echo "  ✓ Console API gRPC Server: Running"
    echo "  ✓ zgi-api Server: Running"
    echo "  ✓ gRPC Communication: Working"
    echo "  ✓ GetAPIKeyQuota: Working"
    echo "  ✓ PreDeductQuota: Working"
    echo "  ✓ SettleQuota: Working"
    echo "  ✓ Unit Tests: Passing"
    exit 0
else
    echo -e "${RED}✗✗✗ Some tests failed. Please check the output above.${NC}"
    exit 1
fi
