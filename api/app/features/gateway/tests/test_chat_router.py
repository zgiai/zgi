import pytest
from unittest.mock import patch
from httpx import AsyncClient
from app.features.gateway.service.llm_service import LLMService

@pytest.mark.asyncio
async def test_create_chat_completion_endpoint(async_client: AsyncClient):
    test_request = {
        "model": "gpt-3.5-turbo",
        "messages": [{"role": "user", "content": "Hello"}],
        "temperature": 0.7
    }
    
    mock_response = {
        "id": "test-id",
        "object": "chat.completion",
        "created": 1234567890,
        "model": "gpt-3.5-turbo",
        "choices": [
            {
                "message": {
                    "role": "assistant",
                    "content": "Test response"
                },
                "finish_reason": "stop",
                "index": 0
            }
        ],
        "usage": {
            "prompt_tokens": 10,
            "completion_tokens": 20,
            "total_tokens": 30
        }
    }
    
    with patch.object(LLMService, "create_chat_completion", return_value=mock_response):
        response = await async_client.post(
            "/v1/chat/completions",
            json=test_request,
            headers={"Authorization": "Bearer test-key"}
        )
        
        assert response.status_code == 200
        data = response.json()
        assert data["object"] == "chat.completion"
        assert data["model"] == "gpt-3.5-turbo"
        assert len(data["choices"]) == 1
        assert data["choices"][0]["message"]["role"] == "assistant"
        assert data["choices"][0]["message"]["content"] == "Test response"
        assert data["choices"][0]["finish_reason"] == "stop"
        assert "usage" in data
        assert data["usage"]["total_tokens"] == 30

@pytest.mark.asyncio
async def test_create_chat_completion_missing_auth(async_client: AsyncClient):
    test_request = {
        "model": "gpt-3.5-turbo",
        "messages": [{"role": "user", "content": "Hello"}]
    }
    
    response = await async_client.post(
        "/v1/chat/completions",
        json=test_request
    )
    
    assert response.status_code == 401
    assert response.json()["detail"] == "No API key provided"

@pytest.mark.asyncio
async def test_create_chat_completion_invalid_request(async_client: AsyncClient):
    test_request = {
        "model": "gpt-3.5-turbo",
        # Missing messages field
    }
    
    response = await async_client.post(
        "/v1/chat/completions",
        json=test_request,
        headers={"Authorization": "Bearer test-key"}
    )
    
    assert response.status_code == 422  # Validation error
    error_detail = response.json()["detail"]
    assert any(error["loc"] == ("body", "messages") for error in error_detail["errors"])

@pytest.mark.asyncio
async def test_create_chat_completion_with_system_message(async_client: AsyncClient):
    test_request = {
        "model": "gpt-3.5-turbo",
        "messages": [
            {"role": "system", "content": "You are a helpful assistant"},
            {"role": "user", "content": "Hello"}
        ],
        "temperature": 0.5,
        "max_tokens": 100
    }
    
    mock_response = {
        "id": "test-id",
        "object": "chat.completion",
        "created": 1234567890,
        "model": "gpt-3.5-turbo",
        "choices": [
            {
                "message": {
                    "role": "assistant",
                    "content": "Hello! How can I assist you today?"
                },
                "finish_reason": "stop",
                "index": 0
            }
        ],
        "usage": {
            "prompt_tokens": 15,
            "completion_tokens": 25,
            "total_tokens": 40
        }
    }
    
    with patch.object(LLMService, "create_chat_completion", return_value=mock_response):
        response = await async_client.post(
            "/v1/chat/completions",
            json=test_request,
            headers={"Authorization": "Bearer test-key"}
        )
        
        assert response.status_code == 200
        data = response.json()
        assert data["choices"][0]["message"]["content"] == "Hello! How can I assist you today?"

@pytest.mark.asyncio
async def test_create_chat_completion_invalid_model(async_client: AsyncClient):
    test_request = {
        "model": "invalid-model",
        "messages": [{"role": "user", "content": "Hello"}]
    }
    
    with patch.object(LLMService, "create_chat_completion", side_effect=ValueError("Invalid model")):
        response = await async_client.post(
            "/v1/chat/completions",
            json=test_request,
            headers={"Authorization": "Bearer test-key"}
        )
        
        assert response.status_code == 400
        assert "Invalid model" in response.json()["detail"]

@pytest.mark.asyncio
async def test_create_chat_completion_with_custom_temperature(async_client: AsyncClient):
    test_request = {
        "model": "gpt-3.5-turbo",
        "messages": [{"role": "user", "content": "Hello"}],
        "temperature": 0.9,
        "max_tokens": 50,
        "stream": False
    }
    
    mock_response = {
        "id": "test-id",
        "object": "chat.completion",
        "created": 1234567890,
        "model": "gpt-3.5-turbo",
        "choices": [
            {
                "message": {
                    "role": "assistant",
                    "content": "Test response with high temperature"
                },
                "finish_reason": "stop",
                "index": 0
            }
        ],
        "usage": {
            "prompt_tokens": 10,
            "completion_tokens": 20,
            "total_tokens": 30
        }
    }
    
    with patch.object(LLMService, "create_chat_completion", return_value=mock_response):
        response = await async_client.post(
            "/v1/chat/completions",
            json=test_request,
            headers={"Authorization": "Bearer test-key"}
        )
        
        assert response.status_code == 200
        data = response.json()
        assert data["choices"][0]["message"]["content"] == "Test response with high temperature"
