"""Test chat completion API."""
import pytest
import json
from unittest.mock import patch, AsyncMock
from app.features.gateway.providers.anthropic_provider import AnthropicProvider
from app.features.gateway.providers.openai_provider import OpenAIProvider

def test_chat_completion_no_auth(client):
    """Test chat completion without authentication."""
    response = client.post(
        "/v1/chat/completions",
        json={
            "model": "claude-3-sonnet-20240229",
            "messages": [{"role": "user", "content": "Hello!"}]
        }
    )
    assert response.status_code == 401
    assert "Invalid API key" in response.json()["detail"]

def test_chat_completion_invalid_key(client):
    """Test chat completion with invalid API key."""
    response = client.post(
        "/v1/chat/completions",
        headers={"Authorization": "Bearer invalid_key"},
        json={
            "model": "claude-3-sonnet-20240229",
            "messages": [{"role": "user", "content": "Hello!"}]
        }
    )
    assert response.status_code == 401
    assert "Invalid API key" in response.json()["detail"]

def test_chat_completion_invalid_model(client, test_headers):
    """Test chat completion with invalid model."""
    response = client.post(
        "/v1/chat/completions",
        headers=test_headers,
        json={
            "model": "invalid-model",
            "messages": [{"role": "user", "content": "Hello!"}]
        }
    )
    assert response.status_code == 400
    assert "Cannot determine provider" in response.json()["detail"]["error"]["message"]

@pytest.mark.asyncio
async def test_chat_completion_anthropic(client, test_headers):
    """Test chat completion with Anthropic provider."""
    mock_response = {
        "id": "msg_123",
        "model": "claude-3-sonnet-20240229",
        "content": [{"text": "Hello! How can I help you today?"}],
        "usage": {"input_tokens": 10, "output_tokens": 20}
    }
    
    with patch.object(
        AnthropicProvider,
        'create_chat_completion',
        new_callable=AsyncMock,
        return_value=mock_response
    ):
        response = client.post(
            "/v1/chat/completions",
            headers=test_headers,
            json={
                "model": "claude-3-sonnet-20240229",
                "messages": [{"role": "user", "content": "Hello!"}]
            }
        )
        
    assert response.status_code == 200
    data = response.json()
    assert data["model"] == "claude-3-sonnet-20240229"
    assert data["choices"][0]["message"]["content"] == "Hello! How can I help you today?"
    assert "usage" in data

@pytest.mark.asyncio
async def test_chat_completion_openai(client, test_headers):
    """Test chat completion with OpenAI provider."""
    mock_response = {
        "id": "chatcmpl-123",
        "model": "gpt-4",
        "choices": [{
            "message": {"role": "assistant", "content": "Hello! How can I help you?"},
            "finish_reason": "stop"
        }],
        "usage": {"total_tokens": 30}
    }
    
    with patch.object(
        OpenAIProvider,
        'create_chat_completion',
        new_callable=AsyncMock,
        return_value=mock_response
    ):
        response = client.post(
            "/v1/chat/completions",
            headers=test_headers,
            json={
                "model": "gpt-4",
                "messages": [{"role": "user", "content": "Hello!"}]
            }
        )
        
    assert response.status_code == 200
    data = response.json()
    assert data["model"] == "gpt-4"
    assert data["choices"][0]["message"]["content"] == "Hello! How can I help you?"
    assert "usage" in data

@pytest.mark.asyncio
async def test_chat_completion_streaming(client, test_headers):
    """Test streaming chat completion."""
    async def mock_stream():
        chunks = [
            {"id": "1", "choices": [{"delta": {"content": "Hello"}}]},
            {"id": "2", "choices": [{"delta": {"content": " world"}}]},
            {"id": "3", "choices": [{"delta": {"content": "!"}, "finish_reason": "stop"}]}
        ]
        for chunk in chunks:
            yield chunk
    
    with patch.object(
        AnthropicProvider,
        'create_chat_completion',
        new_callable=AsyncMock,
        return_value=mock_stream()
    ):
        response = client.post(
            "/v1/chat/completions",
            headers=test_headers,
            json={
                "model": "claude-3-sonnet-20240229",
                "messages": [{"role": "user", "content": "Hello!"}],
                "stream": True
            }
        )
        
    assert response.status_code == 200
    assert response.headers["content-type"] == "text/event-stream"
    
    # Parse SSE response
    content = ""
    for line in response.iter_lines():
        if line:
            line = line.decode("utf-8")
            if line.startswith("data: "):
                data = json.loads(line[6:])
                if "choices" in data:
                    content += data["choices"][0]["delta"].get("content", "")
                    
    assert content == "Hello world!"

def test_chat_completion_quota_exceeded(client, test_headers, test_project, db):
    """Test chat completion with exceeded quota."""
    # Update project quota to 0
    test_project.quota = 0
    db.commit()
    
    response = client.post(
        "/v1/chat/completions",
        headers=test_headers,
        json={
            "model": "claude-3-sonnet-20240229",
            "messages": [{"role": "user", "content": "Hello!"}]
        }
    )
    
    assert response.status_code == 400
    assert "Daily quota exceeded" in response.json()["detail"]["error"]["message"]
