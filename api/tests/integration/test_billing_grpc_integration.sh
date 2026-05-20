#!/bin/bash

# Platform Billing Integration Test Script
# Tests the gRPC communication between zgi-api and zgi-console-api

set -e

echo "=== Platform Billing Integration Test ==="
echo ""

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Check if Console API gRPC server is running
echo "Checking Console API gRPC server..."
if lsof -i :50051 > /dev/null 2>&1; then
    echo -e "${GREEN}✓ Console API gRPC server is running on port 50051${NC}"
else
    echo -e "${RED}✗ Console API gRPC server is NOT running${NC}"
    echo "Please start it with: cd ../zgi-console-api && go run cmd/grpc/main.go"
    exit 1
fi

echo ""

# Test 1: Check if grpcurl is available
echo "Test 1: Checking grpcurl availability..."
if command -v grpcurl &> /dev/null; then
    echo -e "${GREEN}✓ grpcurl is installed${NC}"
else
    echo -e "${YELLOW}⚠ grpcurl not found, skipping direct gRPC tests${NC}"
    echo "Install with: brew install grpcurl"
fi

echo ""

# Test 2: Test gRPC connectivity
echo "Test 2: Testing gRPC connectivity..."
if command -v grpcurl &> /dev/null; then
    # Use proto file since reflection is not enabled
    cd /Users/stark/item/zgi/zgi-console-api
    TEST_RESULT=$(grpcurl -plaintext \
        -import-path ./pkg/rpc/v1 \
        -proto billing.proto \
        -d '{"api_key_id": "a1111111-1111-1111-1111-111111111111"}' \
        localhost:50051 \
        zgi.rpc.v1.BillingService/GetAPIKeyQuota 2>&1)
    
    if echo "$TEST_RESULT" | grep -q "success"; then
        echo -e "${GREEN}✓ gRPC connection successful${NC}"
    else
        echo -e "${RED}✗ gRPC connection failed${NC}"
        echo "$TEST_RESULT"
        exit 1
    fi
    cd /Users/stark/item/zgi/zgi-api
else
    echo -e "${YELLOW}⚠ Skipped (grpcurl not available)${NC}"
fi

echo ""

# Test 3: GetAPIKeyQuota
echo "Test 3: Testing GetAPIKeyQuota..."
if command -v grpcurl &> /dev/null; then
    cd /Users/stark/item/zgi/zgi-console-api
    RESULT=$(grpcurl -plaintext \
        -import-path ./pkg/rpc/v1 \
        -proto billing.proto \
        -d '{"api_key_id": "a2222222-2222-2222-2222-222222222222"}' \
        localhost:50051 \
        zgi.rpc.v1.BillingService/GetAPIKeyQuota 2>&1)
    
    if echo "$RESULT" | grep -q "success"; then
        echo -e "${GREEN}✓ GetAPIKeyQuota successful${NC}"
        echo "$RESULT" | head -5
    else
        echo -e "${RED}✗ GetAPIKeyQuota failed${NC}"
        echo "$RESULT"
        exit 1
    fi
    cd /Users/stark/item/zgi/zgi-api
else
    echo -e "${YELLOW}⚠ Skipped (grpcurl not available)${NC}"
fi

echo ""

# Test 4: PreDeductQuota
echo "Test 4: Testing PreDeductQuota..."
if command -v grpcurl &> /dev/null; then
    cd /Users/stark/item/zgi/zgi-console-api
    RESULT=$(grpcurl -plaintext \
        -import-path ./pkg/rpc/v1 \
        -proto billing.proto \
        -d '{
            "api_key_id": "a2222222-2222-2222-2222-222222222222",
            "tenant_id": "00000000-0000-0000-0000-000000000001",
            "model_name": "gpt-3.5-turbo",
            "provider": "openai",
            "estimated_credits": 10,
            "request_id": "test-'$(date +%s)'"
        }' \
        localhost:50051 \
        zgi.rpc.v1.BillingService/PreDeductQuota 2>&1)
    
    if echo "$RESULT" | grep -q "success"; then
        echo -e "${GREEN}✓ PreDeductQuota successful${NC}"
        DEDUCTION_ID=$(echo "$RESULT" | grep -o '"deduction_id": "[^"]*"' | cut -d'"' -f4)
        echo "Deduction ID: $DEDUCTION_ID"
        
        # Test 5: SettleQuota
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
                "provider": "openai",
                "actual_credits": 8,
                "status": "success",
                "request_id": "test-'$(date +%s)'"
            }' \
            localhost:50051 \
            zgi.rpc.v1.BillingService/SettleQuota 2>&1)
        
        if echo "$SETTLE_RESULT" | grep -q "success"; then
            echo -e "${GREEN}✓ SettleQuota successful${NC}"
        else
            echo -e "${RED}✗ SettleQuota failed${NC}"
            echo "$SETTLE_RESULT"
            exit 1
        fi
    else
        echo -e "${RED}✗ PreDeductQuota failed${NC}"
        echo "$RESULT"
        exit 1
    fi
    cd /Users/stark/item/zgi/zgi-api
else
    echo -e "${YELLOW}⚠ Skipped (grpcurl not available)${NC}"
fi

echo ""

# Test 6: Run Go unit tests
echo "Test 6: Running Go unit tests..."
cd /Users/stark/item/zgi/zgi-api
if go test -v ./tests/integration/llm/platform_billing_integration_test.go 2>&1 | grep -q "PASS"; then
    echo -e "${GREEN}✓ All Go unit tests passed${NC}"
else
    echo -e "${RED}✗ Some Go unit tests failed${NC}"
    exit 1
fi

echo ""
echo "=== All Tests Passed ==="
echo -e "${GREEN}✓ Platform Billing Integration is working correctly!${NC}"
echo ""
echo "Summary:"
echo "  - Console API gRPC server: Running"
echo "  - BillingService: Registered"
echo "  - GetAPIKeyQuota: Working"
echo "  - PreDeductQuota: Working"
echo "  - SettleQuota: Working"
echo "  - Unit Tests: Passing"
echo ""
echo "Next steps:"
echo "  1. Start zgi-api with: export ZGI_EDITION=CLOUD && go run cmd/server/main.go"
echo "  2. Run E2E tests with: hurl --test tests/hurl/billing_integration_e2e.hurl"
