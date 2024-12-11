#!/bin/bash

# Set environment variables
export TESTING=1
export PYTHONPATH=$PYTHONPATH:/Users/stark/item/zgi/zgi/api
export $(cat /Users/stark/item/zgi/zgi/api/.env.test | grep -v '^#' | xargs)

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo "Starting Authentication API Tests..."

# Register admin user
echo -e "\n${GREEN}1. Registering admin user...${NC}"
ADMIN_RESPONSE=$(curl -s -X POST http://localhost:7001/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "email": "admin@example.com",
    "username": "admin",
    "password": "Admin@123456"
  }')
echo "Response: $ADMIN_RESPONSE"
ADMIN_TOKEN=$(echo $ADMIN_RESPONSE | jq -r '.access_token')

# Verify admin status
echo -e "\n${GREEN}2. Verifying admin status...${NC}"
curl -s -X GET http://localhost:7001/v1/auth/me \
  -H "Authorization: Bearer $ADMIN_TOKEN"

# Register normal user
echo -e "\n${GREEN}3. Registering normal user...${NC}"
USER_RESPONSE=$(curl -s -X POST http://localhost:7001/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "email": "user@example.com",
    "username": "user",
    "password": "User123!"
  }')
echo "Response: $USER_RESPONSE"
USER_TOKEN=$(echo $USER_RESPONSE | jq -r '.access_token')

# Verify normal user status
echo -e "\n${GREEN}4. Verifying normal user status...${NC}"
curl -s -X GET http://localhost:7001/v1/auth/me \
  -H "Authorization: Bearer $USER_TOKEN"

# Test login with admin
echo -e "\n${GREEN}5. Testing admin login...${NC}"
ADMIN_LOGIN_RESPONSE=$(curl -s -X POST http://localhost:7001/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "admin@example.com",
    "password": "Admin@123456"
  }')
echo "Response: $ADMIN_LOGIN_RESPONSE"

# Test login with normal user
echo -e "\n${GREEN}6. Testing normal user login...${NC}"
USER_LOGIN_RESPONSE=$(curl -s -X POST http://localhost:7001/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "user@example.com",
    "password": "User123!"
  }')
echo "Response: $USER_LOGIN_RESPONSE"

# Test invalid login
echo -e "\n${GREEN}7. Testing invalid login...${NC}"
INVALID_LOGIN_RESPONSE=$(curl -s -X POST http://localhost:7001/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "wrong@example.com",
    "password": "wrongpass"
  }')
echo "Response: $INVALID_LOGIN_RESPONSE"

echo -e "\n${GREEN}Tests completed!${NC}"
