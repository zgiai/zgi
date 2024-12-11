import pytest
from httpx import AsyncClient
from fastapi import status
from app.features.chat.schemas import ChatMessage

@pytest.mark.asyncio
async def test_stream_chat(async_client: AsyncClient, test_user_headers):
    """Test streaming chat endpoint"""
    # Prepare test request
    request_data = {
        "model": "gpt-4",
        "messages": [
            {
                "role": "user",
                "content": "Say hello!"
            }
        ],
        "temperature": 0.7
    }
    
    # Make request
    async with async_client.stream(
        "POST",
        "/v1/chat/stream",
        json=request_data,
        headers=test_user_headers
    ) as response:
        assert response.status_code == status.HTTP_200_OK
        assert response.headers["content-type"] == "text/event-stream"
        
        # Read streaming response
        async for line in response.aiter_lines():
            # Skip empty lines
            if not line.strip():
                continue
            
            # Check line format
            assert line.startswith("data: ")
            
            # Extract content
            content = line[6:].strip()  # Remove "data: " prefix
            
            # Check if it's the end marker
            if content == "[DONE]":
                break
            
            # For actual content, it should be a non-empty string
            assert content

@pytest.mark.asyncio
async def test_stream_chat_invalid_model(async_client: AsyncClient, test_user_headers):
    """Test streaming chat with invalid model"""
    request_data = {
        "model": "invalid-model",
        "messages": [
            {
                "role": "user",
                "content": "Hello"
            }
        ]
    }
    
    response = await async_client.post(
        "/v1/chat/stream",
        json=request_data,
        headers=test_user_headers
    )
    assert response.status_code == status.HTTP_400_BAD_REQUEST

@pytest.mark.asyncio
async def test_stream_chat_unauthorized(async_client: AsyncClient):
    """Test streaming chat without authentication"""
    request_data = {
        "model": "gpt-4",
        "messages": [
            {
                "role": "user",
                "content": "Hello"
            }
        ]
    }
    
    response = await async_client.post("/v1/chat/stream", json=request_data)
    assert response.status_code == status.HTTP_401_UNAUTHORIZED
