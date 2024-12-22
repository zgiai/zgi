import pytest
from unittest.mock import patch
from httpx import AsyncClient
from app.features.gateway.service.llm_service import LLMService

@pytest.mark.asyncio
async def test_create_chat_completion_endpoint(test_client: AsyncClient):
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
                    "content": "Hello! How can I help you today?"
                },
                "finish_reason": "stop",
                "index": 0
            }
        ],
        "usage": {
            "prompt_tokens": 10,
            "completion_tokens": 10,
            "total_tokens": 20
        }
    }

    with patch.object(LLMService, 'create_chat_completion', return_value=mock_response):
        response = await test_client.post("/v1/gateway/chat/completions", json=test_request)
        assert response.status_code == 200
        assert response.json() == mock_response

@pytest.mark.asyncio
async def test_create_chat_completion_missing_auth(test_client: AsyncClient):
    test_request = {
        "model": "gpt-3.5-turbo",
        "messages": [{"role": "user", "content": "Hello"}]
    }
    response = await test_client.post("/v1/gateway/chat/completions", json=test_request)
    assert response.status_code == 401

@pytest.mark.asyncio
async def test_create_chat_completion_invalid_request(test_client: AsyncClient):
    test_request = {
        "model": "gpt-3.5-turbo",
        # Missing messages field
    }
    response = await test_client.post("/v1/gateway/chat/completions", json=test_request)
    assert response.status_code == 422

@pytest.mark.asyncio
async def test_create_chat_completion_with_system_message(test_client: AsyncClient):
    test_request = {
        "model": "gpt-3.5-turbo",
        "messages": [
            {"role": "system", "content": "You are a helpful assistant"},
            {"role": "user", "content": "Hello"}
        ]
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
                    "content": "Hello! I'm here to help. What can I assist you with?"
                },
                "finish_reason": "stop",
                "index": 0
            }
        ],
        "usage": {
            "prompt_tokens": 15,
            "completion_tokens": 12,
            "total_tokens": 27
        }
    }

    with patch.object(LLMService, 'create_chat_completion', return_value=mock_response):
        response = await test_client.post("/v1/gateway/chat/completions", json=test_request)
        assert response.status_code == 200
        assert response.json() == mock_response

@pytest.mark.asyncio
async def test_create_chat_completion_invalid_model(test_client: AsyncClient):
    test_request = {
        "model": "invalid-model",
        "messages": [{"role": "user", "content": "Hello"}]
    }
    response = await test_client.post("/v1/gateway/chat/completions", json=test_request)
    assert response.status_code == 400

@pytest.mark.asyncio
async def test_create_chat_completion_with_custom_temperature(test_client: AsyncClient):
    test_request = {
        "model": "gpt-3.5-turbo",
        "messages": [{"role": "user", "content": "Hello"}],
        "temperature": 0.9
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
            "prompt_tokens": 10,
            "completion_tokens": 8,
            "total_tokens": 18
        }
    }

    with patch.object(LLMService, 'create_chat_completion', return_value=mock_response):
        response = await test_client.post("/v1/gateway/chat/completions", json=test_request)
        assert response.status_code == 200
        assert response.json() == mock_response
