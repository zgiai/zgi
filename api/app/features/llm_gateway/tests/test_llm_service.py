import pytest
from unittest.mock import AsyncMock, patch
from app.features.llm_gateway.service.llm_service import LLMService
from app.features.llm_gateway.providers.openai_provider import OpenAIProvider
from app.features.llm_gateway.providers.deepseek_provider import DeepSeekProvider

@pytest.fixture
def mock_openai_provider():
    provider = AsyncMock(spec=OpenAIProvider)
    provider.handle_request.return_value = {
        "id": "test-id",
        "object": "chat.completion",
        "created": 1234567890,
        "model": "gpt-3.5-turbo",
        "choices": [
            {
                "message": {
                    "role": "assistant",
                    "content": "Test response"
                }
            }
        ],
        "usage": {
            "prompt_tokens": 10,
            "completion_tokens": 20,
            "total_tokens": 30
        }
    }
    return provider

@pytest.fixture
def mock_deepseek_provider():
    provider = AsyncMock(spec=DeepSeekProvider)
    provider.handle_request.return_value = {
        "id": "test-id",
        "object": "chat.completion",
        "created": 1234567890,
        "model": "deepseek-chat",
        "choices": [
            {
                "index": 0,
                "message": {
                    "role": "assistant",
                    "content": "Test response"
                },
                "logprobs": None,
                "finish_reason": "stop"
            }
        ],
        "usage": {
            "prompt_tokens": 10,
            "completion_tokens": 20,
            "total_tokens": 30,
            "prompt_cache_hit_tokens": 0,
            "prompt_cache_miss_tokens": 10
        },
        "system_fingerprint": "fp_1234567890"
    }
    return provider

@pytest.mark.asyncio
async def test_create_chat_completion_openai_success(mock_openai_provider):
    with patch("app.features.llm_gateway.service.llm_service.get_provider", return_value=mock_openai_provider):
        params = {
            "model": "gpt-3.5-turbo",
            "messages": [{"role": "user", "content": "Hello"}],
            "api_key": "test-key"
        }
        response = await LLMService.create_chat_completion(params)
        
        assert response["object"] == "chat.completion"
        assert response["model"] == "gpt-3.5-turbo"
        assert len(response["choices"]) > 0
        assert "usage" in response

@pytest.mark.asyncio
async def test_create_chat_completion_deepseek_success(mock_deepseek_provider):
    with patch("app.features.llm_gateway.service.llm_service.get_provider", return_value=mock_deepseek_provider):
        params = {
            "model": "deepseek-chat",
            "messages": [{"role": "user", "content": "Hello"}],
            "api_key": "test-key"
        }
        response = await LLMService.create_chat_completion(params)
        
        assert response["object"] == "chat.completion"
        assert response["model"] == "deepseek-chat"
        assert len(response["choices"]) > 0
        assert response["choices"][0]["finish_reason"] == "stop"
        assert "usage" in response
        assert "prompt_cache_hit_tokens" in response["usage"]

@pytest.mark.asyncio
async def test_create_chat_completion_missing_model():
    with pytest.raises(ValueError, match="model parameter is required"):
        await LLMService.create_chat_completion({
            "messages": [{"role": "user", "content": "Hello"}],
            "api_key": "test-key"
        })

@pytest.mark.asyncio
async def test_create_chat_completion_missing_messages():
    with pytest.raises(ValueError, match="messages parameter is required"):
        await LLMService.create_chat_completion({
            "model": "gpt-3.5-turbo",
            "api_key": "test-key"
        })

@pytest.mark.asyncio
async def test_create_chat_completion_missing_api_key():
    with pytest.raises(ValueError, match="api_key is required"):
        await LLMService.create_chat_completion({
            "model": "gpt-3.5-turbo",
            "messages": [{"role": "user", "content": "Hello"}]
        })
