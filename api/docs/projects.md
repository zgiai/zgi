# Projects API Documentation

This document describes the Projects API endpoints. All endpoints require authentication using a Bearer token.

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

### Create Project

Creates a new project in the specified organization.

**Endpoint:** `POST /projects`

**Request Body:**
```json
{
    "name": "string",
    "description": "string",
    "organization_uuid": "string",
    "status": "string"
}
```

**Response:** `200 OK`
```json
{
    "uuid": "string",
    "name": "string",
    "description": "string",
    "organization_uuid": "string",
    "created_by": "integer",
    "status": "string",
    "created_at": "datetime",
    "updated_at": "datetime"
}
```

### List Projects

Retrieves all projects for a specific organization.

**Endpoint:** `GET /projects`

**Query Parameters:**
- `organization_uuid` (required): UUID of the organization

**Response:** `200 OK`
```json
[
    {
        "uuid": "string",
        "name": "string",
        "description": "string",
        "organization_uuid": "string",
        "created_by": "integer",
        "status": "string",
        "created_at": "datetime",
        "updated_at": "datetime"
    }
]
```

### Get Project

Retrieves details of a specific project.

**Endpoint:** `GET /projects/{project_uuid}`

**Path Parameters:**
- `project_uuid`: UUID of the project

**Response:** `200 OK`
```json
{
    "uuid": "string",
    "name": "string",
    "description": "string",
    "organization_uuid": "string",
    "created_by": "integer",
    "status": "string",
    "created_at": "datetime",
    "updated_at": "datetime"
}
```

### Update Project

Updates an existing project.

**Endpoint:** `PUT /projects/{project_uuid}`

**Path Parameters:**
- `project_uuid`: UUID of the project

**Request Body:**
```json
{
    "name": "string",
    "description": "string",
    "status": "string"
}
```

**Response:** `200 OK`
```json
{
    "uuid": "string",
    "name": "string",
    "description": "string",
    "organization_uuid": "string",
    "created_by": "integer",
    "status": "string",
    "created_at": "datetime",
    "updated_at": "datetime"
}
```

### Delete Project

Soft deletes a project.

**Endpoint:** `DELETE /projects/{project_uuid}`

**Path Parameters:**
- `project_uuid`: UUID of the project

**Response:** `200 OK`
```json
{
    "uuid": "string",
    "name": "string",
    "description": "string",
    "organization_uuid": "string",
    "created_by": "integer",
    "status": "deleted",
    "created_at": "datetime",
    "updated_at": "datetime"
}
```

## Error Responses

The API may return the following error responses:

- `400 Bad Request`: Invalid request body or parameters
- `401 Unauthorized`: Invalid or missing authentication token
- `404 Not Found`: Project or organization not found
- `500 Internal Server Error`: Server-side error

## Project Status Values

Projects can have the following status values:
- `active`: Project is active and available
- `inactive`: Project is temporarily disabled
- `deleted`: Project has been soft deleted
