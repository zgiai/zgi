# Providers API Documentation

## Base URL
`/v1/providers`

## Authentication
All endpoints require authentication. Include your API key in the Authorization header:
```
Authorization: Bearer YOUR_API_KEY
```

## Endpoints

### List Providers
Get a list of all providers with optional filtering.

**GET** `/v1/providers`

**Query Parameters**
- `skip` (integer, optional): Number of items to skip. Default: 0
- `limit` (integer, optional): Maximum number of items to return. Default: 100, Max: 100
- `enabled_only` (boolean, optional): Filter only enabled providers. Default: false
- `search` (string, optional): Search providers by name

**Response**
```json
[
  {
    "id": 1,
    "provider_name": "OpenAI",
    "enabled": true,
    "api_key": "sk-...",
    "org_id": "org-...",
    "base_url": "https://api.openai.com",
    "created_at": "2024-12-13T22:34:27.446475",
    "updated_at": "2024-12-13T22:34:27.446475"
  }
]
```

### Create Provider
Create a new model provider.

**POST** `/v1/providers`

**Request Body**
```json
{
  "provider_name": "string",
  "enabled": true,
  "api_key": "string",
  "org_id": "string",
  "base_url": "string"
}
```

**Response**
```json
{
  "id": 1,
  "provider_name": "string",
  "enabled": true,
  "api_key": "string",
  "org_id": "string",
  "base_url": "string",
  "created_at": "2024-12-13T22:34:27.446475",
  "updated_at": "2024-12-13T22:34:27.446475"
}
```

### Get Provider
Get a specific provider by ID.

**GET** `/v1/providers/{provider_id}`

**Path Parameters**
- `provider_id` (integer, required): The ID of the provider

**Response**
```json
{
  "id": 1,
  "provider_name": "string",
  "enabled": true,
  "api_key": "string",
  "org_id": "string",
  "base_url": "string",
  "created_at": "2024-12-13T22:34:27.446475",
  "updated_at": "2024-12-13T22:34:27.446475"
}
```

### Update Provider
Update a specific provider.

**PUT** `/v1/providers/{provider_id}`

**Path Parameters**
- `provider_id` (integer, required): The ID of the provider

**Request Body**
```json
{
  "provider_name": "string",
  "enabled": true,
  "api_key": "string",
  "org_id": "string",
  "base_url": "string"
}
```

**Response**
```json
{
  "id": 1,
  "provider_name": "string",
  "enabled": true,
  "api_key": "string",
  "org_id": "string",
  "base_url": "string",
  "created_at": "2024-12-13T22:34:27.446475",
  "updated_at": "2024-12-13T22:34:27.446475"
}
```

### Delete Provider
Soft delete a provider.

**DELETE** `/v1/providers/{provider_id}`

**Path Parameters**
- `provider_id` (integer, required): The ID of the provider

**Response**
```json
true
```

## Error Responses

### 404 Not Found
```json
{
  "detail": "Provider not found"
}
```

### 400 Bad Request
```json
{
  "detail": "Invalid request parameters"
}
```
