# API Keys Management Documentation

This document describes the API Keys management endpoints. All endpoints require authentication using a Bearer token.

## Base URL

```
http://localhost:8000/v1
```

## Authentication

All endpoints require a valid JWT token in the Authorization header:

```
Authorization: Bearer <access_token>
```

## Endpoints

### Create API Key

Creates a new API key for a specific project.

**Endpoint:** `POST /projects/{project_uuid}/api-keys`

**Path Parameters:**
- `project_uuid`: UUID of the project

**Request Body:**
```json
{
    "name": "string"
}
```

**Response:** `200 OK`
```json
{
    "uuid": "string",
    "name": "string",
    "key": "string",
    "project_uuid": "string",
    "created_by": "integer",
    "is_active": true,
    "created_at": "datetime",
    "updated_at": "datetime"
}
```

### List API Keys

Retrieves all API keys for a specific project.

**Endpoint:** `GET /projects/{project_uuid}/api-keys`

**Path Parameters:**
- `project_uuid`: UUID of the project

**Response:** `200 OK`
```json
[
    {
        "uuid": "string",
        "name": "string",
        "key": "string",
        "project_uuid": "string",
        "created_by": "integer",
        "is_active": true,
        "created_at": "datetime",
        "updated_at": "datetime"
    }
]
```

### Disable API Key

Disables an API key. Disabled keys cannot be used for authentication.

**Endpoint:** `POST /projects/{project_uuid}/api-keys/{key_uuid}/disable`

**Path Parameters:**
- `project_uuid`: UUID of the project
- `key_uuid`: UUID of the API key

**Response:** `200 OK`
```json
{
    "message": "API key disabled successfully"
}
```

### Delete API Key

Soft deletes an API key. Deleted keys cannot be recovered.

**Endpoint:** `DELETE /projects/{project_uuid}/api-keys/{key_uuid}`

**Path Parameters:**
- `project_uuid`: UUID of the project
- `key_uuid`: UUID of the API key

**Response:** `200 OK`
```json
{
    "message": "API key deleted successfully"
}
```

## Error Responses

The API may return the following error responses:

- `400 Bad Request`: Invalid request body or parameters
- `401 Unauthorized`: Invalid or missing authentication token
- `404 Not Found`: Project or API key not found
- `500 Internal Server Error`: Server-side error

## API Key Format

API keys are generated with the following format:
- Prefix: `zgi_`
- Key: Random URL-safe base64 encoded string (32 bytes)
- Example: `zgi_1234abcd...`

## Security Notes

1. API keys are sensitive information and should be treated as secrets
2. API keys cannot be retrieved after creation - make sure to store them securely
3. Disabled or deleted API keys cannot be re-enabled
4. API keys are project-specific and cannot be shared between projects
