"""Test chat completion API."""
import pytest
import httpx
import os
import json
from typing import Dict, Any

# Test configuration
BASE_URL = "http://localhost:8000"
TEST_TIMEOUT = 30.0

# Test data
TEST_MESSAGES = [
    {"role": "user", "content": "Hello, how are you?"}
]

@pytest.fixture
def api_client():
    """Create test HTTP client."""
    return httpx.AsyncClient(base_url=BASE_URL, timeout=TEST_TIMEOUT)

async def make_chat_request(
    client: httpx.AsyncClient,
    model: str,
    messages: list,
    api_key: str = None
) -> Dict[str, Any]:
    """Make a chat completion request.
    
    Args:
        client: HTTP client
        model: Model name
        messages: List of messages
        api_key: Optional API key
        
    Returns:
        Response data
    """
    headers = {
        "Content-Type": "application/json"
    }
    
    if api_key:
        headers["Authorization"] = f"Bearer {api_key}"
        
    data = {
        "model": model,
        "messages": messages
    }
    
    response = await client.post(
        "/v1/chat/completions",
        json=data,
        headers=headers
    )
    return response

@pytest.mark.asyncio
async def test_chat_completion_with_user_key(api_client):
    """Test chat completion with user-provided API key."""
    # Replace with your test API key
    test_api_key = "your-test-api-key"
    
    response = await make_chat_request(
        client=api_client,
        model="gpt-3.5-turbo",
        messages=TEST_MESSAGES,
        api_key=test_api_key
    )
    
    assert response.status_code == 200
    data = response.json()
    assert "choices" in data
    assert len(data["choices"]) > 0
    assert "message" in data["choices"][0]

@pytest.mark.asyncio
async def test_chat_completion_with_default_key(api_client):
    """Test chat completion with default API key."""
    response = await make_chat_request(
        client=api_client,
        model="gpt-3.5-turbo",
        messages=TEST_MESSAGES
    )
    
    assert response.status_code == 200
    data = response.json()
    assert "choices" in data
    assert len(data["choices"]) > 0
    assert "message" in data["choices"][0]

@pytest.mark.asyncio
async def test_chat_completion_invalid_auth_header(api_client):
    """Test chat completion with invalid authorization header."""
    headers = {
        "Content-Type": "application/json",
        "Authorization": "Invalid-Auth-Header"
    }
    
    data = {
        "model": "gpt-3.5-turbo",
        "messages": TEST_MESSAGES
    }
    
    response = await api_client.post(
        "/v1/chat/completions",
        json=data,
        headers=headers
    )
    
    assert response.status_code == 401
    data = response.json()
    assert "error" in data

@pytest.mark.asyncio
async def test_chat_completion_invalid_key(api_client):
    """Test chat completion with invalid API key."""
    response = await make_chat_request(
        client=api_client,
        model="gpt-3.5-turbo",
        messages=TEST_MESSAGES,
        api_key="invalid-key"
    )
    
    assert response.status_code == 401
    data = response.json()
    assert "error" in data

@pytest.mark.asyncio
async def test_chat_completion_missing_model(api_client):
    """Test chat completion with missing model."""
    data = {
        "messages": TEST_MESSAGES
    }
    response = await api_client.post("/v1/chat/completions", json=data)
    
    assert response.status_code == 400
    data = response.json()
    assert "error" in data

@pytest.mark.asyncio
async def test_chat_completion_unsupported_model(api_client):
    """Test chat completion with unsupported model."""
    response = await make_chat_request(
        client=api_client,
        model="unsupported-model",
        messages=TEST_MESSAGES
    )
    
    assert response.status_code == 400
    data = response.json()
    assert "error" in data

@pytest.mark.asyncio
async def test_chat_completion_anthropic(api_client):
    """Test chat completion with Anthropic model."""
    response = await make_chat_request(
        client=api_client,
        model="claude-3-sonnet-20240229",
        messages=TEST_MESSAGES
    )
    
    assert response.status_code == 200
    data = response.json()
    assert "choices" in data
    assert len(data["choices"]) > 0
    assert "message" in data["choices"][0]

@pytest.mark.asyncio
async def test_chat_completion_deepseek(api_client):
    """Test chat completion with DeepSeek model."""
    response = await make_chat_request(
        client=api_client,
        model="deepseek-chat",
        messages=TEST_MESSAGES
    )
    
    assert response.status_code == 200
    data = response.json()
    assert "choices" in data
    assert len(data["choices"]) > 0
    assert "message" in data["choices"][0]
