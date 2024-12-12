# Organizations API Documentation

## Overview
The Organizations API provides endpoints for managing organizations within the system. Organizations can have multiple members with different roles (Owner, Admin, Member).

## Authentication
All endpoints require authentication using a Bearer token in the Authorization header:
```
Authorization: Bearer <access_token>
```

## API Endpoints

### Create Organization
Creates a new organization and assigns the creator as the owner.

**Endpoint:** `POST /v1/organizations/`

**Request Body:**
```json
{
    "name": "string",         // required, 1-255 characters
    "description": "string"   // optional, max 1000 characters
}
```

**Response:** `200 OK`
```json
{
    "id": "string",          // UUID
    "name": "string",
    "description": "string",
    "created_by": "integer",
    "is_active": true,
    "created_at": "datetime",
    "updated_at": "datetime"
}
```

### List Organizations
Returns a list of organizations that the authenticated user is a member of.

**Endpoint:** `GET /v1/organizations/`

**Response:** `200 OK`
```json
[
    {
        "id": "string",
        "name": "string",
        "description": "string",
        "created_by": "integer",
        "is_active": true,
        "created_at": "datetime",
        "updated_at": "datetime"
    }
]
```

### Get Organization
Returns details of a specific organization.

**Endpoint:** `GET /v1/organizations/{org_id}`

**Parameters:**
- `org_id`: Organization UUID (path parameter)

**Response:** `200 OK`
```json
{
    "id": "string",
    "name": "string",
    "description": "string",
    "created_by": "integer",
    "is_active": true,
    "created_at": "datetime",
    "updated_at": "datetime"
}
```

### Update Organization
Updates an organization's details. Only owners and admins can update organization details.

**Endpoint:** `PUT /v1/organizations/{org_id}`

**Parameters:**
- `org_id`: Organization UUID (path parameter)

**Request Body:**
```json
{
    "name": "string",         // optional, 1-255 characters
    "description": "string",  // optional, max 1000 characters
    "is_active": true        // optional
}
```

**Response:** `200 OK`
```json
{
    "id": "string",
    "name": "string",
    "description": "string",
    "created_by": "integer",
    "is_active": true,
    "created_at": "datetime",
    "updated_at": "datetime"
}
```

## Error Responses

### 401 Unauthorized
```json
{
    "detail": "Not authenticated"
}
```

### 403 Forbidden
```json
{
    "detail": "Not enough permissions"
}
```

### 404 Not Found
```json
{
    "detail": "Organization not found"
}
```

### 422 Validation Error
```json
{
    "detail": [
        {
            "loc": ["body", "field_name"],
            "msg": "error message",
            "type": "error_type"
        }
    ]
}
```

## Testing
To run the tests:

```bash
pytest app/features/organizations/tests/test_organizations.py -v
```

The test suite includes:
1. Creating organizations
2. Listing organizations
3. Getting organization details
4. Updating organization details
5. Testing permissions
6. Error cases

Make sure to set up the test database and environment variables before running the tests.
