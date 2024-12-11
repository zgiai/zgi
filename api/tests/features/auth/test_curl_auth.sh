#!/bin/bash

# Set the base URL
BASE_URL="http://localhost:7001"

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
        echo "Response: $3"
    fi
}

echo "Running API Authentication Tests..."

# Test 1: User Registration
echo -e "\nTesting User Registration..."
REGISTER_RESPONSE=$(curl -s -X POST "${BASE_URL}/v1/console/auth/register" \
    -H "Content-Type: application/json" \
    -d '{
        "email": "test.user@example.com",
        "password": "Test123!@#",
        "username": "testuser"
    }')

if echo "$REGISTER_RESPONSE" | grep -q "email"; then
    print_result 0 "User Registration Success"
else
    print_result 1 "User Registration Failed" "$REGISTER_RESPONSE"
fi

# Test 2: User Login
echo -e "\nTesting User Login..."
LOGIN_RESPONSE=$(curl -s -X POST "${BASE_URL}/v1/console/auth/login" \
    -H "Content-Type: application/json" \
    -d '{
        "email": "test.user@example.com",
        "password": "Test123!@#"
    }')

# Extract access token if login successful
ACCESS_TOKEN=$(echo "$LOGIN_RESPONSE" | grep -o '"access_token":"[^"]*' | cut -d'"' -f4)

if [ ! -z "$ACCESS_TOKEN" ]; then
    print_result 0 "User Login Success"
else
    print_result 1 "User Login Failed" "$LOGIN_RESPONSE"
fi

# Test 3: Access Protected Resource
echo -e "\nTesting Protected Resource Access..."
if [ ! -z "$ACCESS_TOKEN" ]; then
    PROTECTED_RESPONSE=$(curl -s -X GET "${BASE_URL}/v1/console/auth/protected-resource" \
        -H "Authorization: Bearer ${ACCESS_TOKEN}")
    
    if echo "$PROTECTED_RESPONSE" | grep -q "message"; then
        print_result 0 "Protected Resource Access Success"
    else
        print_result 1 "Protected Resource Access Failed" "$PROTECTED_RESPONSE"
    fi
else
    print_result 1 "Protected Resource Access Skipped - No Access Token"
fi

# Test 4: Invalid Login
echo -e "\nTesting Invalid Login..."
INVALID_LOGIN_RESPONSE=$(curl -s -X POST "${BASE_URL}/v1/console/auth/login" \
    -H "Content-Type: application/json" \
    -d '{
        "email": "wrong@example.com",
        "password": "wrongpassword"
    }')

if echo "$INVALID_LOGIN_RESPONSE" | grep -q "detail.*Unauthorized\|detail.*Incorrect"; then
    print_result 0 "Invalid Login Test Success"
else
    print_result 1 "Invalid Login Test Failed" "$INVALID_LOGIN_RESPONSE"
fi

# Test 5: Invalid Registration (Weak Password)
echo -e "\nTesting Invalid Registration (Weak Password)..."
WEAK_PASSWORD_RESPONSE=$(curl -s -X POST "${BASE_URL}/v1/console/auth/register" \
    -H "Content-Type: application/json" \
    -d '{
        "email": "test2@example.com",
        "password": "weak",
        "username": "test2"
    }')

if echo "$WEAK_PASSWORD_RESPONSE" | grep -q "detail.*password\|validation"; then
    print_result 0 "Weak Password Test Success"
else
    print_result 1 "Weak Password Test Failed" "$WEAK_PASSWORD_RESPONSE"
fi

# Test 6: Protected Resource Without Token
echo -e "\nTesting Protected Resource Without Token..."
NO_TOKEN_RESPONSE=$(curl -s -X GET "${BASE_URL}/v1/console/auth/protected-resource")

if echo "$NO_TOKEN_RESPONSE" | grep -q "detail.*Unauthorized\|detail.*Not authenticated"; then
    print_result 0 "No Token Test Success"
else
    print_result 1 "No Token Test Failed" "$NO_TOKEN_RESPONSE"
fi

# Test 7: Duplicate Registration
echo -e "\nTesting Duplicate Registration..."
DUPLICATE_REGISTER_RESPONSE=$(curl -s -X POST "${BASE_URL}/v1/console/auth/register" \
    -H "Content-Type: application/json" \
    -d '{
        "email": "test.user@example.com",
        "password": "Test123!@#",
        "username": "testuser"
    }')

if echo "$DUPLICATE_REGISTER_RESPONSE" | grep -q "detail.*exists\|already registered"; then
    print_result 0 "Duplicate Registration Test Success"
else
    print_result 1 "Duplicate Registration Test Failed" "$DUPLICATE_REGISTER_RESPONSE"
fi

echo -e "\nAPI Authentication Tests Completed!"
