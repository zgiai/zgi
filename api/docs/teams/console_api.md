# Console Teams API Documentation

## Create Team

### Endpoint: POST /v1/console/teams

### Description
Create a new team in the system. This endpoint is restricted to admin users only.

### Request
- Headers:
  - Authorization: Bearer {token}
  - Content-Type: application/json

- Body:
  ```json
  {
    "name": "string",          // Team name (required)
    "description": "string",   // Team description
    "max_members": integer,    // Maximum number of members allowed (default: 5)
    "allow_member_invite": boolean,  // Allow members to invite others (default: false)
    "default_member_role": "string", // Default role for new members (default: "member")
    "isolated_data": boolean,        // Enable data isolation (default: true)
    "shared_api_keys": boolean       // Allow API key sharing (default: false)
  }
  ```

### Response
- Success (200 OK):
  ```json
  {
    "id": integer,
    "name": "string",
    "description": "string",
    "max_members": integer,
    "allow_member_invite": boolean,
    "default_member_role": "string",
    "isolated_data": boolean,
    "shared_api_keys": boolean,
    "created_at": "datetime",
    "updated_at": "datetime"
  }
  ```

- Error (4xx/5xx):
  ```json
  {
    "error": "error message",
    "details": "error details"
  }
  ```

### Test Cases
1. Create team with all fields
   ```bash
   curl -X POST \
     -H "Authorization: Bearer {token}" \
     -H "Content-Type: application/json" \
     -d '{
       "name": "Test Team",
       "description": "Test Team Description",
       "max_members": 10,
       "allow_member_invite": true,
       "default_member_role": "member",
       "isolated_data": true,
       "shared_api_keys": false
     }' \
     http://localhost:7001/v1/console/teams
   ```

2. Create team with minimum required fields
   ```bash
   curl -X POST \
     -H "Authorization: Bearer {token}" \
     -H "Content-Type: application/json" \
     -d '{
       "name": "Minimal Team"
     }' \
     http://localhost:7001/v1/console/teams
   ```

### Notes
- The team name must be unique
- Only authenticated admin users can create teams
- All fields except `name` are optional and have default values
