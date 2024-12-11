# Authentication API Tests

## API Endpoints

Base URL: `http://localhost:7001`

### 1. Register First Admin User

```bash
# First user becomes admin (requires complex password)
curl -X POST http://localhost:7001/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "email": "admin@example.com",
    "username": "admin",
    "password": "Admin@123456"
  }'

# Expected Response:
{
    "access_token": "eyJhbG...",
    "token_type": "bearer"
}
```

### 2. Register Normal User

```bash
# Second user becomes normal user
curl -X POST http://localhost:7001/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "email": "user@example.com",
    "username": "user",
    "password": "User123!"
  }'

# Expected Response:
{
    "access_token": "eyJhbG...",
    "token_type": "bearer"
}
```

### 3. Login

```bash
curl -X POST http://localhost:7001/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "admin@example.com",
    "password": "Admin@123456"
  }'

# Expected Response:
{
    "access_token": "eyJhbG...",
    "token_type": "bearer"
}
```

### 4. Get Current User

```bash
curl -X GET http://localhost:7001/v1/auth/me \
  -H "Authorization: Bearer YOUR_ACCESS_TOKEN"

# Expected Response for Admin:
{
    "id": 1,
    "email": "admin@example.com",
    "username": "admin",
    "is_admin": true
}

# Expected Response for Normal User:
{
    "id": 2,
    "email": "user@example.com",
    "username": "user",
    "is_admin": false
}
```

## Test Cases

1. First User Registration (Admin)
   - Should succeed with complex password
   - Should return access token
   - User should have admin privileges

2. Second User Registration (Normal)
   - Should succeed with normal password
   - Should return access token
   - User should not have admin privileges

3. Password Validation
   - Admin registration should fail with simple password
   - Normal user registration should accept simpler passwords

4. Login Flow
   - Should succeed with correct credentials
   - Should fail with incorrect password
   - Should fail with non-existent email

5. User Information
   - Admin user should see is_admin: true
   - Normal user should see is_admin: false
   - Invalid token should return 401 error

## Running Tests

You can run these tests in sequence using the following script:

```bash
#!/bin/bash

# Register admin user
echo "Registering admin user..."
ADMIN_RESPONSE=$(curl -s -X POST http://localhost:7001/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "email": "admin@example.com",
    "username": "admin",
    "password": "Admin@123456"
  }')
ADMIN_TOKEN=$(echo $ADMIN_RESPONSE | jq -r '.access_token')

# Verify admin status
echo "Verifying admin status..."
curl -X GET http://localhost:7001/v1/auth/me \
  -H "Authorization: Bearer $ADMIN_TOKEN"

# Register normal user
echo "Registering normal user..."
USER_RESPONSE=$(curl -s -X POST http://localhost:7001/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "email": "user@example.com",
    "username": "user",
    "password": "User123!"
  }')
USER_TOKEN=$(echo $USER_RESPONSE | jq -r '.access_token')

# Verify normal user status
echo "Verifying normal user status..."
curl -X GET http://localhost:7001/v1/auth/me \
  -H "Authorization: Bearer $USER_TOKEN"
```

Save this script as `test_auth.sh` and run it with:
```bash
chmod +x test_auth.sh
./test_auth.sh
```
