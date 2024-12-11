# ZGI API Documentation

## Authentication

All API requests require authentication using JWT Bearer token. Include the token in the Authorization header:

```
Authorization: Bearer your-jwt-token
```

## API Endpoints

### Authentication

#### Register User
- **URL**: `/v1/client/auth/register`
- **Method**: `POST`
- **Body**:
  ```json
  {
    "email": "user@example.com",
    "username": "username",
    "password": "password123"
  }
  ```
- **Response**:
  ```json
  {
    "access_token": "jwt-token",
    "token_type": "bearer"
  }
  ```

#### Login
- **URL**: `/v1/client/auth/login`
- **Method**: `POST`
- **Body**:
  ```json
  {
    "email": "user@example.com",
    "password": "password123"
  }
  ```
- **Response**:
  ```json
  {
    "access_token": "jwt-token",
    "token_type": "bearer"
  }
  ```

#### Get Current User
- **URL**: `/v1/client/auth/me`
- **Method**: `GET`
- **Headers**: Required `Authorization`
- **Response**:
  ```json
  {
    "email": "user@example.com",
    "username": "username",
    "id": 1,
    "created_at": "2024-12-10T22:22:18",
    "updated_at": "2024-12-10T22:22:18"
  }
  ```

### API Keys Management

#### Create API Key
- **URL**: `/v1/client/api-keys`
- **Method**: `POST`
- **Headers**: Required `Authorization`
- **Body**:
  ```json
  {
    "provider": "openai",
    "key_name": "OpenAI Production",
    "key_value": "sk-xxx",
    "is_active": true
  }
  ```
- **Response**:
  ```json
  {
    "id": 1,
    "provider": "openai",
    "key_name": "OpenAI Production",
    "key_value": "sk-xxx",
    "is_active": true,
    "user_id": 1,
    "last_used_at": null,
    "created_at": "2024-12-10T22:28:49",
    "updated_at": "2024-12-10T22:28:49"
  }
  ```

#### List API Keys
- **URL**: `/v1/client/api-keys`
- **Method**: `GET`
- **Headers**: Required `Authorization`
- **Response**:
  ```json
  [
    {
      "id": 1,
      "provider": "openai",
      "key_name": "OpenAI Production",
      "is_active": true,
      "user_id": 1,
      "last_used_at": null,
      "created_at": "2024-12-10T22:28:49",
      "updated_at": "2024-12-10T22:28:49"
    }
  ]
  ```

#### Get API Key Details
- **URL**: `/v1/client/api-keys/{api_key_id}`
- **Method**: `GET`
- **Headers**: Required `Authorization`
- **Response**:
  ```json
  {
    "id": 1,
    "provider": "openai",
    "key_name": "OpenAI Production",
    "key_value": "sk-xxx",
    "is_active": true,
    "user_id": 1,
    "last_used_at": null,
    "created_at": "2024-12-10T22:28:49",
    "updated_at": "2024-12-10T22:28:49"
  }
  ```

#### Update API Key
- **URL**: `/v1/client/api-keys/{api_key_id}`
- **Method**: `PUT`
- **Headers**: Required `Authorization`
- **Body**:
  ```json
  {
    "key_name": "OpenAI GPT-4",
    "is_active": false
  }
  ```
- **Response**:
  ```json
  {
    "id": 1,
    "provider": "openai",
    "key_name": "OpenAI GPT-4",
    "key_value": "sk-xxx",
    "is_active": false,
    "user_id": 1,
    "last_used_at": null,
    "created_at": "2024-12-10T22:28:49",
    "updated_at": "2024-12-10T22:29:15"
  }
  ```

#### Delete API Key
- **URL**: `/v1/client/api-keys/{api_key_id}`
- **Method**: `DELETE`
- **Headers**: Required `Authorization`
- **Response**:
  ```json
  {
    "message": "API key deleted successfully"
  }
  ```

## Error Responses

### 401 Unauthorized
```json
{
  "detail": "Could not validate credentials"
}
```

### 404 Not Found
```json
{
  "detail": "API key not found"
}
```

### 400 Bad Request
```json
{
  "detail": "Email already registered"
}
```
