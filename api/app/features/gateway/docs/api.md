# LLM Gateway API Documentation

## Overview
The LLM Gateway provides a unified interface for interacting with various Language Model providers. It currently supports OpenAI and DeepSeek models.

## Base URL
```
http://localhost:7001
```

## Authentication
All API endpoints require authentication using Bearer token in the Authorization header:
```
Authorization: Bearer your-api-key-here
```

## Endpoints

### Create Chat Completion
Creates a completion for the chat message.

**Endpoint:** `POST /v1/chat/completions`

**Request Headers:**
```
Authorization: Bearer your-api-key
Content-Type: application/json
```

**Request Body:**
```json
{
    "model": string,          // Required: Model ID to use
    "messages": [             // Required: Array of messages
        {
            "role": string,   // Required: "system", "user", or "assistant"
            "content": string // Required: The message content
        }
    ],
    "temperature": float,     // Optional: Sampling temperature (0-2), default: 0.7
    "max_tokens": integer,    // Optional: Maximum tokens to generate
    "stream": boolean,        // Optional: Stream responses, default: false
    "base_url": string       // Optional: Override default base URL for the provider
}
```

**Supported Models:**
- OpenAI Models:
  - `gpt-4`: GPT-4 model with 8k context
  - `gpt-3.5-turbo`: GPT-3.5 Turbo with 4k context
- DeepSeek Models:
  - `deepseek-chat`: General chat model
  - `deepseek-coder`: Code-specialized model

**Example Request:**
```bash
curl -X POST http://localhost:7001/v1/chat/completions \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-chat",
    "messages": [
      {
        "role": "user",
        "content": "Hello, how are you?"
      }
    ],
    "temperature": 0.7
  }'
```

**Success Response (200 OK):**
```json
{
    "id": string,            // Unique identifier for the completion
    "object": "chat.completion",
    "created": integer,      // Unix timestamp
    "model": string,         // Model used
    "choices": [
        {
            "index": integer,
            "message": {
                "role": "assistant",
                "content": string
            },
            "finish_reason": string
        }
    ],
    "usage": {
        "prompt_tokens": integer,
        "completion_tokens": integer,
        "total_tokens": integer
    }
}
```

**Error Responses:**
- `401 Unauthorized`: Missing or invalid API key
- `422 Unprocessable Entity`: Invalid request parameters
- `400 Bad Request`: Invalid model or other request error
- `500 Internal Server Error`: Server-side error

## Rate Limiting
Rate limiting is applied based on the underlying provider's limits.

## Notes
- The API aims to be compatible with OpenAI's chat completion API format
- Response format matches OpenAI's format for easy integration
- Stream mode is supported for real-time responses
- Token usage is tracked and returned in responses
