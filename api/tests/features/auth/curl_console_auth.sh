#!/bin/bash

# Console Authentication Test Script

# Configuration
BASE_URL="http://localhost:7001"
CONSOLE_AUTH_PATH="/v1/console/auth"

# Test user credentials
EMAIL="test.console@example.com"
PASSWORD="Test123!@#"
USERNAME="testconsole"

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Function to print test results
print_result() {
    if [ $1 -eq 0 ]; then
        echo -e "${GREEN}✓ $2${NC}"
    else
        echo -e "${RED}✗ $2${NC}"
    fi
}

# Function to print section header
print_header() {
    echo -e "\n=== $1 ==="
}

echo "=== Console Authentication API Tests ==="

# 1. User Registration Test
print_header "1. User Registration Test"

echo "Request:"
echo "POST ${BASE_URL}${CONSOLE_AUTH_PATH}/register"
echo "Payload: {\"email\": \"$EMAIL\", \"password\": \"$PASSWORD\", \"username\": \"$USERNAME\"}"

REGISTER_RESPONSE=$(curl -s -X POST "${BASE_URL}${CONSOLE_AUTH_PATH}/register" \
    -H "Content-Type: application/json" \
    -d "{
        \"email\": \"${EMAIL}\",
        \"password\": \"${PASSWORD}\",
        \"username\": \"${USERNAME}\"
    }")

echo -e "\nResponse:"
echo "$REGISTER_RESPONSE" | jq '.'

if echo "$REGISTER_RESPONSE" | jq -e 'has("email")' > /dev/null; then
    print_result 0 "User Registration Success"
else
    print_result 1 "User Registration Failed"
fi

# 2. User Login Test
print_header "2. User Login Test"

echo "Request:"
echo "POST ${BASE_URL}${CONSOLE_AUTH_PATH}/login"
echo "Payload: {\"email\": \"$EMAIL\", \"password\": \"$PASSWORD\"}"

LOGIN_RESPONSE=$(curl -s -X POST "${BASE_URL}${CONSOLE_AUTH_PATH}/login" \
    -H "Content-Type: application/json" \
    -d "{
        \"email\": \"${EMAIL}\",
        \"password\": \"${PASSWORD}\"
    }")

echo -e "\nResponse:"
echo "$LOGIN_RESPONSE" | jq '.'

# Extract access token
ACCESS_TOKEN=$(echo "$LOGIN_RESPONSE" | jq -r '.access_token')

if [ "$ACCESS_TOKEN" != "null" ]; then
    print_result 0 "User Login Success"
else
    print_result 1 "User Login Failed"
fi

# 3. Protected Resource Access Test
print_header "3. Protected Resource Access Test"

if [ "$ACCESS_TOKEN" != "null" ]; then
    echo "Request:"
    echo "GET ${BASE_URL}${CONSOLE_AUTH_PATH}/protected-resource"
    echo "Authorization: Bearer ${ACCESS_TOKEN}"

    PROTECTED_RESPONSE=$(curl -s -X GET "${BASE_URL}${CONSOLE_AUTH_PATH}/protected-resource" \
        -H "Authorization: Bearer ${ACCESS_TOKEN}")

    echo -e "\nResponse:"
    echo "$PROTECTED_RESPONSE" | jq '.'

    if echo "$PROTECTED_RESPONSE" | jq -e 'has("message")' > /dev/null; then
        print_result 0 "Protected Resource Access Success"
    else
        print_result 1 "Protected Resource Access Failed"
    fi
else
    print_result 1 "Protected Resource Access Skipped - No Access Token"
fi

# 4. Invalid Login Test
print_header "4. Invalid Login Test"

echo "Request:"
echo "POST ${BASE_URL}${CONSOLE_AUTH_PATH}/login"
echo "Payload: {\"email\": \"wrong@example.com\", \"password\": \"wrongpass\"}"

INVALID_LOGIN_RESPONSE=$(curl -s -X POST "${BASE_URL}${CONSOLE_AUTH_PATH}/login" \
    -H "Content-Type: application/json" \
    -d '{
        "email": "wrong@example.com",
        "password": "wrongpass"
    }')

echo -e "\nResponse:"
echo "$INVALID_LOGIN_RESPONSE" | jq '.'

if echo "$INVALID_LOGIN_RESPONSE" | jq -e '.detail' > /dev/null; then
    print_result 0 "Invalid Login Test Success"
else
    print_result 1 "Invalid Login Test Failed"
fi

# 5. Duplicate Registration Test
print_header "5. Duplicate Registration Test"

echo "Request:"
echo "POST ${BASE_URL}${CONSOLE_AUTH_PATH}/register"
echo "Payload: {\"email\": \"$EMAIL\", \"password\": \"$PASSWORD\", \"username\": \"$USERNAME\"}"

DUPLICATE_REGISTER_RESPONSE=$(curl -s -X POST "${BASE_URL}${CONSOLE_AUTH_PATH}/register" \
    -H "Content-Type: application/json" \
    -d "{
        \"email\": \"${EMAIL}\",
        \"password\": \"${PASSWORD}\",
        \"username\": \"${USERNAME}\"
    }")

echo -e "\nResponse:"
echo "$DUPLICATE_REGISTER_RESPONSE" | jq '.'

if echo "$DUPLICATE_REGISTER_RESPONSE" | jq -e '.detail' > /dev/null; then
    print_result 0 "Duplicate Registration Test Success"
else
    print_result 1 "Duplicate Registration Test Failed"
fi

# 6. Weak Password Registration Test
print_header "6. Weak Password Registration Test"

echo "Request:"
echo "POST ${BASE_URL}${CONSOLE_AUTH_PATH}/register"
echo "Payload: {\"email\": \"test2@example.com\", \"password\": \"weak\", \"username\": \"test2\"}"

WEAK_PASSWORD_RESPONSE=$(curl -s -X POST "${BASE_URL}${CONSOLE_AUTH_PATH}/register" \
    -H "Content-Type: application/json" \
    -d '{
        "email": "test2@example.com",
        "password": "weak",
        "username": "test2"
    }')

echo -e "\nResponse:"
echo "$WEAK_PASSWORD_RESPONSE" | jq '.'

if echo "$WEAK_PASSWORD_RESPONSE" | jq -e '.detail' > /dev/null; then
    print_result 0 "Weak Password Test Success"
else
    print_result 1 "Weak Password Test Failed"
fi

# 7. Invalid Email Format Test
print_header "7. Invalid Email Format Test"

echo "Request:"
echo "POST ${BASE_URL}${CONSOLE_AUTH_PATH}/register"
echo "Payload: {\"email\": \"invalid-email\", \"password\": \"Test123!@#\", \"username\": \"test3\"}"

INVALID_EMAIL_RESPONSE=$(curl -s -X POST "${BASE_URL}${CONSOLE_AUTH_PATH}/register" \
    -H "Content-Type: application/json" \
    -d '{
        "email": "invalid-email",
        "password": "Test123!@#",
        "username": "test3"
    }')

echo -e "\nResponse:"
echo "$INVALID_EMAIL_RESPONSE" | jq '.'

if echo "$INVALID_EMAIL_RESPONSE" | jq -e '.detail' > /dev/null; then
    print_result 0 "Invalid Email Format Test Success"
else
    print_result 1 "Invalid Email Format Test Failed"
fi

echo -e "\n=== Console Authentication Tests Completed! ===\n"
