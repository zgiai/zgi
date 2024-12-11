#!/bin/bash

# Client Authentication Test Script

# Configuration
BASE_URL="http://localhost:7001"
CLIENT_AUTH_PATH="/v1/client/auth"

# Test API key
API_KEY="test_api_key_12345"
APPLICATION_ID="1"

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo "Testing Client Authentication API..."

# 1. Create API Key (requires admin token)
echo -e "\n1. First login as admin to get token"
echo "Request:"
echo "POST ${BASE_URL}/v1/console/auth/login"

ADMIN_LOGIN_RESPONSE=$(curl -s -X POST "${BASE_URL}/v1/console/auth/login" \
    -H "Content-Type: application/json" \
    -d '{
        "email": "admin@example.com",
        "password": "adminpass123"
    }')

echo "Admin Login Response:"
echo "$ADMIN_LOGIN_RESPONSE" | jq '.'

ADMIN_TOKEN=$(echo "$ADMIN_LOGIN_RESPONSE" | jq -r '.access_token')

if [ "$ADMIN_TOKEN" != "null" ]; then
    echo -e "\n2. Creating new API Key"
    echo "Request:"
    echo "POST ${BASE_URL}${CLIENT_AUTH_PATH}/api-keys"

    API_KEY_RESPONSE=$(curl -s -X POST "${BASE_URL}${CLIENT_AUTH_PATH}/api-keys" \
        -H "Authorization: Bearer ${ADMIN_TOKEN}" \
        -H "Content-Type: application/json" \
        -d "{
            \"name\": \"Test API Key\",
            \"application_id\": ${APPLICATION_ID}
        }")

    echo "API Key Creation Response:"
    echo "$API_KEY_RESPONSE" | jq '.'

    # Extract the actual API key for further tests
    NEW_API_KEY=$(echo "$API_KEY_RESPONSE" | jq -r '.key')

    # 3. Test Client Login with API Key
    echo -e "\n3. Testing Client Login with API Key"
    echo "Request:"
    echo "POST ${BASE_URL}${CLIENT_AUTH_PATH}/login"

    LOGIN_RESPONSE=$(curl -s -X POST "${BASE_URL}${CLIENT_AUTH_PATH}/login" \
        -H "Content-Type: application/json" \
        -H "X-API-Key: ${NEW_API_KEY}")

    echo "Client Login Response:"
    echo "$LOGIN_RESPONSE" | jq '.'

    # Extract client access token
    CLIENT_TOKEN=$(echo "$LOGIN_RESPONSE" | jq -r '.access_token')

    # 4. Test Protected Resource Access with Client Token
    if [ "$CLIENT_TOKEN" != "null" ]; then
        echo -e "\n4. Testing Protected Resource Access with Client Token"
        echo "Request:"
        echo "GET ${BASE_URL}${CLIENT_AUTH_PATH}/protected-resource"

        curl -s -X GET "${BASE_URL}${CLIENT_AUTH_PATH}/protected-resource" \
            -H "Authorization: Bearer ${CLIENT_TOKEN}" | jq '.'
    fi

    # 5. Test Invalid API Key
    echo -e "\n5. Testing Invalid API Key"
    echo "Request:"
    echo "POST ${BASE_URL}${CLIENT_AUTH_PATH}/login"

    curl -s -X POST "${BASE_URL}${CLIENT_AUTH_PATH}/login" \
        -H "Content-Type: application/json" \
        -H "X-API-Key: invalid_api_key_123" | jq '.'

    # 6. List API Keys (admin only)
    echo -e "\n6. Listing API Keys (Admin only)"
    echo "Request:"
    echo "GET ${BASE_URL}${CLIENT_AUTH_PATH}/api-keys"

    curl -s -X GET "${BASE_URL}${CLIENT_AUTH_PATH}/api-keys" \
        -H "Authorization: Bearer ${ADMIN_TOKEN}" | jq '.'

    # 7. Delete API Key (admin only)
    if [ "$NEW_API_KEY" != "null" ]; then
        echo -e "\n7. Deleting API Key"
        echo "Request:"
        echo "DELETE ${BASE_URL}${CLIENT_AUTH_PATH}/api-keys/${NEW_API_KEY}"

        curl -s -X DELETE "${BASE_URL}${CLIENT_AUTH_PATH}/api-keys/${NEW_API_KEY}" \
            -H "Authorization: Bearer ${ADMIN_TOKEN}" | jq '.'
    fi
else
    echo "Failed to get admin token. Skipping API key tests."
fi

# 8. Test Protected Resource without Token
echo -e "\n8. Testing Protected Resource without Token"
echo "Request:"
echo "GET ${BASE_URL}${CLIENT_AUTH_PATH}/protected-resource"

curl -s -X GET "${BASE_URL}${CLIENT_AUTH_PATH}/protected-resource" | jq '.'

echo -e "\nClient Authentication Tests Completed!"
