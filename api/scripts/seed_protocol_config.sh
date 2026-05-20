#!/bin/bash
# Protocol Configuration Data Maintenance Script
# Usage: ./scripts/seed_protocol_config.sh [environment]

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Load environment variables
if [ -f "$PROJECT_ROOT/.env" ]; then
    export $(cat "$PROJECT_ROOT/.env" | grep -v '^#' | xargs)
fi

# Database connection
DB_URL="postgresql://${DB_USERNAME}:${DB_PASSWORD}@${DB_HOST}:${DB_PORT}/${DB_NAME}?sslmode=${DB_SSLMODE}"

echo "🔧 Protocol Configuration Data Maintenance"
echo "=========================================="
echo ""

# Function to execute SQL
execute_sql() {
    local sql="$1"
    psql "$DB_URL" -c "$sql"
}

# 1. Seed provider-protocol mappings
echo "📝 Seeding provider-protocol mappings..."
execute_sql "
-- Update existing providers with protocol configuration
UPDATE llm_providers SET 
    protocol = name,
    fallback_protocol = CASE 
        WHEN name = 'openai' THEN NULL 
        ELSE 'openai' 
    END
WHERE protocol IS NULL;

-- Special cases
UPDATE llm_providers SET protocol = 'openai' WHERE name IN ('azure', 'openrouter');
UPDATE llm_providers SET protocol = 'anthropic' WHERE name = 'claude';
"

# 2. Verify seeding
echo ""
echo "✅ Verification: Current provider-protocol mappings"
echo "---------------------------------------------------"
execute_sql "
SELECT 
    name AS provider,
    protocol,
    fallback_protocol,
    is_active
FROM llm_providers 
ORDER BY name;
"

# 3. Show providers without protocol configuration
echo ""
echo "⚠️  Providers without protocol configuration:"
echo "---------------------------------------------"
execute_sql "
SELECT name, display_name 
FROM llm_providers 
WHERE protocol IS NULL OR protocol = '';
"

echo ""
echo "✅ Protocol configuration seeding completed!"
echo ""
echo "📖 To add a new provider with protocol:"
echo "   INSERT INTO llm_providers (name, display_name, protocol, fallback_protocol)"
echo "   VALUES ('new-provider', 'New Provider', 'new-protocol', 'openai');"
echo ""
echo "📖 To update existing provider protocol:"
echo "   UPDATE llm_providers SET protocol = 'new-protocol' WHERE name = 'provider-name';"
