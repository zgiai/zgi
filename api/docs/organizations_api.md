# Organization API Documentation

## Overview
The Organization API provides endpoints for managing organizations within the system. Users can create, list, view, and update organizations.

## Authentication
All endpoints require authentication using a Bearer token in the Authorization header:
```
Authorization: Bearer <access_token>
```

## Endpoints

### Create Organization
Creates a new organization and assigns the creator as the owner.

**Endpoint:** `POST /v1/organizations/`

**Request Body:**
```json
{
    "name": "string",         // Required, 1-255 characters
    "description": "string"   // Optional, max 1000 characters
}
```

**Response:** `201 Created`
```json
{
    "id": 1,
    "name": "string",
    "description": "string",
    "created_by": 1,
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
        "id": 1,
        "name": "string",
        "description": "string",
        "created_by": 1,
        "is_active": true,
        "created_at": "datetime",
        "updated_at": "datetime"
    }
]
```

### Get Organization
Retrieves details of a specific organization.

**Endpoint:** `GET /v1/organizations/{org_id}`

**Parameters:**
- `org_id` (path): Organization ID (integer)

**Response:** `200 OK`
```json
{
    "id": 1,
    "name": "string",
    "description": "string",
    "created_by": 1,
    "is_active": true,
    "created_at": "datetime",
    "updated_at": "datetime"
}
```

**Error Responses:**
- `404 Not Found`: Organization not found

### Update Organization
Updates an organization's details. Requires owner or admin role.

**Endpoint:** `PUT /v1/organizations/{org_id}`

**Parameters:**
- `org_id` (path): Organization ID (integer)

**Request Body:**
```json
{
    "name": "string",         // Optional, 1-255 characters
    "description": "string",  // Optional, max 1000 characters
    "is_active": true        // Optional
}
```

**Response:** `200 OK`
```json
{
    "id": 1,
    "name": "string",
    "description": "string",
    "created_by": 1,
    "is_active": true,
    "created_at": "datetime",
    "updated_at": "datetime"
}
```

**Error Responses:**
- `404 Not Found`: Organization not found or insufficient permissions

## Error Handling
All endpoints may return the following error responses:
- `401 Unauthorized`: Invalid or missing authentication token
- `403 Forbidden`: Insufficient permissions
- `422 Unprocessable Entity`: Invalid request data

## Rate Limiting
API endpoints are subject to rate limiting. Please check response headers for rate limit information.
