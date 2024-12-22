import pytest
from httpx import AsyncClient
import json
from ..providers.deepseek_provider import DeepSeekProvider

@pytest.mark.asyncio
async def test_deepseek_chat_completion():
    provider = DeepSeekProvider(api_key="sk-3e5f5d61abc341c584d5c508f618d7f5")
    
    params = {
        "model": "deepseek-chat",
        "messages": [
            {"role": "user", "content": "Hello, how are you?"}
        ],
        "temperature": 0.7
    }
    
    response = await provider.handle_request(params)
    
    assert "choices" in response
    assert len(response["choices"]) > 0
    assert "message" in response["choices"][0]
    assert "content" in response["choices"][0]["message"]
    assert "role" in response["choices"][0]["message"]
    assert response["choices"][0]["message"]["role"] == "assistant"

@pytest.mark.asyncio
async def test_deepseek_api_key_validation():
    provider = DeepSeekProvider(api_key="sk-3e5f5d61abc341c584d5c508f618d7f5")
    is_valid = await provider.validate_api_key()
    assert is_valid == True

@pytest.mark.asyncio
async def test_deepseek_model_info():
    provider = DeepSeekProvider(api_key="sk-3e5f5d61abc341c584d5c508f618d7f5")
    model_info = provider.get_model_info()
    
    assert "deepseek-chat" in model_info
    assert "deepseek-coder" in model_info
    assert model_info["deepseek-chat"]["name"] == "DeepSeek Chat"
    assert model_info["deepseek-coder"]["name"] == "DeepSeek Coder"
