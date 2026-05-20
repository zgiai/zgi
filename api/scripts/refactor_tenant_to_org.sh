#!/bin/bash

# Script to refactor tenant_id to organization_id in LLM module
# This script performs batch find-and-replace operations

set -e

echo "🚀 Starting refactor: tenant_id → organization_id"

# Base directory
BASE_DIR="/Users/stark/item/zgi/zgi-api/internal/modules/llm"

# 1. Replace TenantID field in structs
echo "📝 Step 1: Replacing TenantID → OrganizationID in struct fields..."
find "$BASE_DIR" -type f -name "*.go" -exec sed -i '' \
  -e 's/TenantID[[:space:]]*uuid\.UUID[[:space:]]*`gorm:"\([^"]*\)tenant_id/OrganizationID uuid.UUID `gorm:"\1organization_id/g' \
  -e 's/TenantID[[:space:]]*string[[:space:]]*`gorm:"\([^"]*\)tenant_id/OrganizationID string `gorm:"\1organization_id/g' \
  -e 's/TenantID[[:space:]]*\*uuid\.UUID[[:space:]]*`gorm:"\([^"]*\)tenant_id/OrganizationID *uuid.UUID `gorm:"\1organization_id/g' \
  {} \;

# 2. Replace json tags
echo "📝 Step 2: Replacing json:\"tenant_id\" → json:\"organization_id\"..."
find "$BASE_DIR" -type f -name "*.go" -exec sed -i '' \
  -e 's/json:"tenant_id"/json:"organization_id"/g' \
  {} \;

# 3. Replace function parameters
echo "📝 Step 3: Replacing function parameters tenantID → organizationID..."
find "$BASE_DIR" -type f -name "*.go" -exec sed -i '' \
  -e 's/tenantID uuid\.UUID/organizationID uuid.UUID/g' \
  -e 's/tenantID string/organizationID string/g' \
  -e 's/tenantID \*uuid\.UUID/organizationID *uuid.UUID/g' \
  -e 's/(tenantID,/(organizationID,/g' \
  -e 's/, tenantID)/, organizationID)/g' \
  -e 's/, tenantID,/, organizationID,/g' \
  {} \;

# 4. Replace variable assignments and usage
echo "📝 Step 4: Replacing variable usage..."
find "$BASE_DIR" -type f -name "*.go" -exec sed -i '' \
  -e 's/\.TenantID/.OrganizationID/g' \
  -e 's/tenant_id = ?/organization_id = ?/g' \
  -e 's/tenant_id IN/organization_id IN/g' \
  -e 's/tenant_id,/organization_id,/g' \
  -e 's/"tenant_id"/"organization_id"/g' \
  {} \;

# 5. Replace table names in TableName() functions
echo "📝 Step 5: Replacing table names..."
find "$BASE_DIR" -type f -name "*.go" -exec sed -i '' \
  -e 's/return "llm_tenant_balances"/return "llm_organization_balances"/g' \
  -e 's/return "llm_tenant_transactions"/return "llm_organization_transactions"/g' \
  -e 's/return "llm_tenant_api_keys"/return "llm_organization_api_keys"/g' \
  -e 's/return "llm_tenant_credentials"/return "llm_organization_credentials"/g' \
  {} \;

# 6. Replace context keys
echo "📝 Step 6: Replacing context keys..."
find "$BASE_DIR" -type f -name "*.go" -exec sed -i '' \
  -e 's/c\.GetString("tenant_id")/c.GetString("organization_id")/g' \
  -e 's/c\.Set("tenant_id",/c.Set("organization_id",/g' \
  -e 's/c\.GetHeader("X-Tenant-ID")/c.GetHeader("X-Organization-ID")/g' \
  {} \;

# 7. Replace error messages
echo "📝 Step 7: Replacing error messages..."
find "$BASE_DIR" -type f -name "*.go" -exec sed -i '' \
  -e 's/tenant_id is required/organization_id is required/g' \
  -e 's/invalid tenant_id/invalid organization_id/g' \
  -e 's/ErrInvalidTenantID/ErrInvalidOrganizationID/g' \
  {} \;

# 8. Replace function names
echo "📝 Step 8: Replacing function names..."
find "$BASE_DIR" -type f -name "*.go" -exec sed -i '' \
  -e 's/func getTenantID/func getOrganizationID/g' \
  -e 's/getTenantID(/getOrganizationID(/g' \
  {} \;

echo "✅ Refactor complete!"
echo ""
echo "📊 Summary of changes:"
echo "  - Struct fields: TenantID → OrganizationID"
echo "  - JSON tags: tenant_id → organization_id"
echo "  - Function parameters: tenantID → organizationID"
echo "  - Database queries: tenant_id → organization_id"
echo "  - Table names: llm_tenant_* → llm_organization_*"
echo "  - Context keys: tenant_id → organization_id"
echo ""
echo "⚠️  Please review the changes and run: go build ./internal/modules/llm/..."
