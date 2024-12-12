# Chat API Documentation

## Streaming Chat Endpoint

Stream responses from AI models with real-time token generation.

### Endpoint

```
POST /v1/chat/stream
```

### Authentication

Requires Bearer token authentication.

### Request Body

```json
{
  "model": "string",
  "messages": [
    {
      "role": "string",
      "content": "string"
    }
  ],
  "temperature": float,
  "max_tokens": integer,
  "stream": boolean,
  "session_id": integer
}
```

#### Parameters

- `model` (required): The model to use (e.g., "gpt-4", "gpt-3.5-turbo")
- `messages` (required): Array of message objects with role and content
- `temperature` (optional): Sampling temperature (0.0 to 2.0, default: 0.7)
- `max_tokens` (optional): Maximum tokens to generate
- `stream` (optional): Whether to stream the response (default: true)
- `session_id` (optional): ID of an existing chat session

### Response

The endpoint returns a stream of Server-Sent Events (SSE) with the following format:

```
data: token1

data: token2

data: token3

data: [DONE]
```

Each line starts with "data: " followed by the token content. The stream ends with "[DONE]".

### Example Usage

#### cURL

```bash
curl -X POST http://localhost:7001/v1/chat/stream \
  -H "Authorization: Bearer YOUR_ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4",
    "messages": [
      {
        "role": "user",
        "content": "Tell me a story"
      }
    ],
    "temperature": 0.7
  }'
```

#### Python

```python
import aiohttp
import asyncio

async def stream_chat():
    url = "http://localhost:7001/v1/chat/stream"
    headers = {
        "Authorization": "Bearer YOUR_ACCESS_TOKEN",
        "Content-Type": "application/json"
    }
    data = {
        "model": "gpt-4",
        "messages": [
            {
                "role": "user",
                "content": "Tell me a story"
            }
        ]
    }
    
    async with aiohttp.ClientSession() as session:
        async with session.post(url, json=data, headers=headers) as response:
            async for line in response.content:
                if line.startswith(b'data: '):
                    content = line[6:].decode('utf-8').strip()
                    if content == "[DONE]":
                        break
                    print(content, end='', flush=True)

asyncio.run(stream_chat())
```

### Error Responses

- `401 Unauthorized`: Invalid or missing authentication token
- `400 Bad Request`: Invalid request parameters
- `500 Internal Server Error`: Server-side error

### Rate Limiting

The API is subject to rate limiting based on your subscription tier. Please refer to the usage documentation for details.

## Switch Model Endpoint

Switch the model for an existing chat session while preserving the conversation history.

### Endpoint

```
POST /v1/chat/switch-model
```

### Authentication

Requires Bearer token authentication.

### Request Body

```json
{
  "session_id": integer,
  "model": string
}
```

#### Parameters

- `session_id` (required): ID of the chat session to update
- `model` (required): Name of the new model to use

### Response

```json
{
  "session_id": integer,
  "model": string,
  "messages": [
    {
      "role": string,
      "content": string
    }
  ],
  "title": string
}
```

### Notes on Model Switching

- The conversation history is preserved when switching models
- The new model will have access to all previous messages in the chat session
- Switching models does not reset or clear the chat context
- The session's metadata is updated to reflect the new model
- All future messages in the session will use the new model

### Example Usage

#### cURL

```bash
# Stream chat with session
curl -X POST http://localhost:7001/v1/chat/stream \
  -H "Authorization: Bearer YOUR_ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4",
    "messages": [
      {
        "role": "user",
        "content": "Tell me a story"
      }
    ],
    "session_id": 123
  }'

# Switch model
curl -X POST http://localhost:7001/v1/chat/switch-model \
  -H "Authorization: Bearer YOUR_ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": 123,
    "model": "gpt-3.5-turbo"
  }'
```

#### Python

```python
import aiohttp
import asyncio

async def switch_model():
    url = "http://localhost:7001/v1/chat/switch-model"
    headers = {
        "Authorization": "Bearer YOUR_ACCESS_TOKEN",
        "Content-Type": "application/json"
    }
    data = {
        "session_id": 123,
        "model": "gpt-3.5-turbo"
    }
    
    async with aiohttp.ClientSession() as session:
        async with session.post(url, json=data, headers=headers) as response:
            return await response.json()

asyncio.run(switch_model())
```

### Error Responses

- `401 Unauthorized`: Invalid or missing authentication token
- `404 Not Found`: Chat session not found
- `400 Bad Request`: Invalid model name or request parameters
- `500 Internal Server Error`: Server-side error

### Rate Limiting

The API is subject to rate limiting based on your subscription tier. Please refer to the usage documentation for details.

## Chat History Endpoints

### Get Chat History

Retrieve paginated chat history for the authenticated user.

#### Endpoint

```
GET /v1/chat/history
```

#### Authentication

Requires Bearer token authentication.

#### Query Parameters

- `page` (optional): Page number (default: 1)
- `page_size` (optional): Number of items per page (default: 10, max: 100)
- `start_date` (optional): Filter sessions created after this date (ISO format)
- `end_date` (optional): Filter sessions created before this date (ISO format)
- `model` (optional): Filter sessions by model name

#### Response

```json
{
  "total": integer,
  "page": integer,
  "page_size": integer,
  "sessions": [
    {
      "id": integer,
      "user_id": integer,
      "application_id": integer,
      "model": string,
      "title": string,
      "messages": [
        {
          "role": string,
          "content": string
        }
      ],
      "created_at": string,
      "updated_at": string
    }
  ]
}
```

### Delete Chat Session

Delete a specific chat session.

#### Endpoint

```
DELETE /v1/chat/sessions/{session_id}
```

#### Authentication

Requires Bearer token authentication.

#### Path Parameters

- `session_id`: ID of the chat session to delete

#### Response

```json
{
  "message": "Session deleted successfully"
}
```

### Example Usage

#### cURL

```bash
# Get chat history
curl -X GET "http://localhost:7001/v1/chat/history?page=1&page_size=10" \
  -H "Authorization: Bearer YOUR_ACCESS_TOKEN"

# Get history with filters
curl -X GET "http://localhost:7001/v1/chat/history?model=gpt-4&start_date=2024-01-01T00:00:00Z" \
  -H "Authorization: Bearer YOUR_ACCESS_TOKEN"

# Delete a chat session
curl -X DELETE "http://localhost:7001/v1/chat/sessions/123" \
  -H "Authorization: Bearer YOUR_ACCESS_TOKEN"
```

#### Python

```python
import aiohttp
import asyncio
from datetime import datetime, timedelta

async def get_chat_history():
    url = "http://localhost:7001/v1/chat/history"
    headers = {
        "Authorization": "Bearer YOUR_ACCESS_TOKEN"
    }
    params = {
        "page": 1,
        "page_size": 10,
        "start_date": (datetime.utcnow() - timedelta(days=7)).isoformat(),
        "model": "gpt-4"
    }
    
    async with aiohttp.ClientSession() as session:
        async with session.get(url, params=params, headers=headers) as response:
            return await response.json()

asyncio.run(get_chat_history())
```

### Notes on Chat History

- Chat history is automatically saved for all conversations
- History includes full message context for each session
- Sessions are ordered by last update time (newest first)
- Deleted sessions cannot be recovered
- History is user-specific and requires authentication

### Error Responses

- `401 Unauthorized`: Invalid or missing authentication token
- `404 Not Found`: Chat session not found (for deletion)
- `400 Bad Request`: Invalid query parameters
- `500 Internal Server Error`: Server-side error

### Rate Limiting

The API is subject to rate limiting based on your subscription tier. Please refer to the usage documentation for details.

## File Upload API

### Upload PDF File
`POST /api/chat/sessions/{session_id}/upload`

Upload a PDF file to be used as context in the chat session.

**Parameters:**
- `session_id` (path, required): ID of the chat session
- `file` (form, required): PDF file to upload (max 10MB)

**Response:**
```json
{
  "file": {
    "id": 1,
    "filename": "document.pdf",
    "file_size": 1024,
    "mime_type": "application/pdf",
    "extracted_text": "...",
    "metadata": {
      "page_count": 5
    }
  },
  "message": "File uploaded successfully"
}
```

**cURL Example:**
```bash
curl -X POST \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -F "file=@document.pdf" \
  http://localhost:7001/api/chat/sessions/123/upload
```

**Notes:**
- Only PDF files are supported
- Maximum file size: 10MB
- File content is automatically extracted and used as context

## Prompt Management API

### Create Prompt
`POST /api/chat/prompts`

Create a new chat prompt template.

**Request Body:**
```json
{
  "title": "Text Summarizer",
  "content": "Summarize the following text: {text}",
  "scenario": "summarization",
  "description": "Generates concise summaries",
  "metadata": {
    "tags": ["summary", "concise"]
  }
}
```

**Response:**
```json
{
  "id": 1,
  "title": "Text Summarizer",
  "content": "Summarize the following text: {text}",
  "scenario": "summarization",
  "description": "Generates concise summaries",
  "metadata": {
    "tags": ["summary", "concise"]
  },
  "is_template": false,
  "usage_count": 0,
  "created_at": "2024-12-11T01:19:23Z",
  "updated_at": "2024-12-11T01:19:23Z"
}
```

**cURL Example:**
```bash
curl -X POST \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Text Summarizer",
    "content": "Summarize the following text: {text}",
    "scenario": "summarization",
    "description": "Generates concise summaries"
  }' \
  http://localhost:7001/api/chat/prompts
```

### List Prompts
`GET /api/chat/prompts`

List available prompts with filtering and pagination.

**Query Parameters:**
- `scenario` (optional): Filter by scenario
- `include_templates` (optional): Include system templates (default: true)
- `search` (optional): Search in title, content, and description
- `page` (optional): Page number (default: 1)
- `page_size` (optional): Items per page (default: 20, max: 100)

**Response:**
```json
{
  "items": [
    {
      "id": 1,
      "title": "Text Summarizer",
      "content": "Summarize the following text: {text}",
      "scenario": "summarization",
      "description": "Generates concise summaries",
      "metadata": {},
      "is_template": false,
      "usage_count": 5,
      "created_at": "2024-12-11T01:19:23Z",
      "updated_at": "2024-12-11T01:19:23Z"
    }
  ],
  "total": 1,
  "page": 1,
  "page_size": 20
}
```

**cURL Example:**
```bash
curl -X GET \
  -H "Authorization: Bearer YOUR_TOKEN" \
  "http://localhost:7001/api/chat/prompts?scenario=summarization&page=1&page_size=20"
```

### Preview Prompt
`POST /api/chat/prompts/preview`

Preview the output of a prompt with the chat model.

**Request Body:**
```json
{
  "prompt_id": 1,
  "variables": {
    "text": "Sample text to summarize"
  }
}
```
or
```json
{
  "content": "Summarize this text: {text}",
  "variables": {
    "text": "Sample text to summarize"
  }
}
```

**Response:**
```json
{
  "preview": "Generated summary of the sample text...",
  "tokens_used": 150,
  "model_used": "gpt-3.5-turbo"
}
```

**cURL Example:**
```bash
curl -X POST \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "prompt_id": 1,
    "variables": {
      "text": "Sample text to summarize"
    }
  }' \
  http://localhost:7001/api/chat/prompts/preview
```

### Update Prompt
`PUT /api/chat/prompts/{prompt_id}`

Update an existing prompt.

**Path Parameters:**
- `prompt_id` (required): ID of the prompt to update

**Request Body:**
```json
{
  "title": "Updated Title",
  "content": "Updated content with {variable}",
  "scenario": "new_scenario",
  "description": "Updated description"
}
```

**Response:** Same as Create Prompt

**cURL Example:**
```bash
curl -X PUT \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Updated Title",
    "content": "Updated content"
  }' \
  http://localhost:7001/api/chat/prompts/1
```

### Delete Prompt
`DELETE /api/chat/prompts/{prompt_id}`

Delete a prompt.

**Path Parameters:**
- `prompt_id` (required): ID of the prompt to delete

**Response:**
```json
{
  "message": "Prompt deleted successfully"
}
```

**cURL Example:**
```bash
curl -X DELETE \
  -H "Authorization: Bearer YOUR_TOKEN" \
  http://localhost:7001/api/chat/prompts/1
```

## Error Responses

All endpoints may return the following errors:

- `400 Bad Request`: Invalid request parameters
- `401 Unauthorized`: Missing or invalid authentication token
- `403 Forbidden`: Insufficient permissions
- `404 Not Found`: Resource not found
- `422 Unprocessable Entity`: Validation error

Example error response:
```json
{
  "detail": "Error message describing the problem"
}
