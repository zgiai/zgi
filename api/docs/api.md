# API Documentation

## Overview
This API provides comprehensive functionality for knowledge base management, user management, analytics, and more. All endpoints require authentication unless specifically marked as public.

## Authentication
Authentication is handled via JWT tokens. Include the token in the Authorization header:
```
Authorization: Bearer <your_token>
```

## User Management

### User Profile
#### GET /api/v1/users/me
Get current user's profile.

Response:
```json
{
  "email": "user@example.com",
  "full_name": "John Doe",
  "is_active": true,
  "is_verified": true,
  "created_at": "2024-01-01T00:00:00Z",
  "updated_at": "2024-01-01T00:00:00Z"
}
```

#### PUT /api/v1/users/me
Update current user's profile.

Request:
```json
{
  "full_name": "John Smith",
  "password": "new_password"  // Optional
}
```

### Email Verification
#### POST /api/v1/users/verify-email/request
Request email verification. Sends verification email to user's email address.

#### POST /api/v1/users/verify-email/confirm
Confirm email verification.

Request:
```json
{
  "email": "user@example.com",
  "token": "verification_token"
}
```

### Password Reset
#### POST /api/v1/users/password-reset/request
Request password reset. Sends reset email to user's email address.

Request:
```json
{
  "email": "user@example.com"
}
```

#### POST /api/v1/users/password-reset/confirm
Confirm password reset.

Request:
```json
{
  "email": "user@example.com",
  "token": "reset_token",
  "new_password": "new_password"
}
```

## Analytics

### Usage Metrics
#### POST /api/v1/analytics/usage-metrics
Get usage metrics for a specific time range.

Request:
```json
{
  "start_date": "2024-01-01T00:00:00Z",
  "end_date": "2024-01-07T00:00:00Z"
}
```

Response:
```json
{
  "api_calls": 1000,
  "total_tokens": 50000,
  "total_documents": 100,
  "storage_used": 1024.5,
  "vector_count": 10000,
  "timestamp": "2024-01-07T00:00:00Z"
}
```

### System Metrics
#### GET /api/v1/analytics/system-metrics
Get current system metrics.

Response:
```json
{
  "cpu_usage": 45.5,
  "memory_usage": 70.2,
  "disk_usage": 60.8,
  "api_latency": 0.15,
  "error_rate": 0.01,
  "timestamp": "2024-01-07T00:00:00Z"
}
```

### Analytics Report
#### POST /api/v1/analytics/report
Generate analytics report for a specific time range.

Request:
```json
{
  "start_date": "2024-01-01T00:00:00Z",
  "end_date": "2024-01-07T00:00:00Z"
}
```

Response:
```json
{
  "time_range": {
    "start_date": "2024-01-01T00:00:00Z",
    "end_date": "2024-01-07T00:00:00Z"
  },
  "total_users": 1000,
  "active_users": 750,
  "total_api_calls": 50000,
  "average_response_time": 0.12,
  "error_rate": 0.01,
  "usage_by_endpoint": {
    "/api/v1/kb/search": 25000,
    "/api/v1/kb/upload": 5000
  },
  "top_users": [
    {
      "user_id": 1,
      "api_calls": 1000,
      "total_tokens": 50000
    }
  ]
}
```

## Rate Limiting
All API endpoints are rate-limited. The default limits are:
- 60 requests per minute for regular endpoints
- 10 requests per minute for heavy operations (document upload, vector search)

Rate limit headers are included in all responses:
```
X-RateLimit-Limit: 60
X-RateLimit-Remaining: 59
X-RateLimit-Reset: 1641024000
```

## Error Handling
All errors follow a standard format:
```json
{
  "detail": "Error message",
  "code": "ERROR_CODE",
  "params": {}  // Optional additional parameters
}
```

Common error codes:
- `UNAUTHORIZED`: Authentication required or failed
- `FORBIDDEN`: Insufficient permissions
- `NOT_FOUND`: Resource not found
- `RATE_LIMITED`: Rate limit exceeded
- `VALIDATION_ERROR`: Invalid request parameters
- `INTERNAL_ERROR`: Internal server error

## Caching
The API uses caching for:
- User profiles (1 hour TTL)
- System metrics (1 minute TTL)
- Usage metrics (5 minutes TTL)
- Vector search results (1 hour TTL)

Cache headers are included in responses where applicable:
```
Cache-Control: max-age=3600
ETag: "abc123"
```
