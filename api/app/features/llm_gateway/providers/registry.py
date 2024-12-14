from typing import Dict, Type
from .base import BaseProvider
from .openai_provider import OpenAIProvider
from .deepseek_provider import DeepSeekProvider

# Registry mapping models to their provider implementation
MODEL_REGISTRY: Dict[str, Dict[str, str | Type[BaseProvider]]] = {
    # OpenAI Models
    "gpt-4": {"provider": "openai", "handler": OpenAIProvider},
    "gpt-3.5-turbo": {"provider": "openai", "handler": OpenAIProvider},
    
    # DeepSeek Models
    "deepseek-chat": {"provider": "deepseek", "handler": DeepSeekProvider, "model": "deepseek-chat-v1"},
    "deepseek-coder": {"provider": "deepseek", "handler": DeepSeekProvider, "model": "deepseek-coder-v1"},
}

def get_provider(model_name: str, api_key: str, base_url: str | None = None) -> BaseProvider:
    """
    Return the appropriate provider handler for a given model name.
    
    Args:
        model_name: Name of the model to use
        api_key: API key for the provider
        base_url: Optional base URL for the provider API
    
    Returns:
        An instance of the appropriate provider handler
    
    Raises:
        ValueError: If the model is not supported
    """
    model_info = MODEL_REGISTRY.get(model_name)
    if not model_info:
        raise ValueError(f"Model {model_name} is not supported")

    handler_class = model_info["handler"]
    
    # Set default base URL for DeepSeek if not provided
    if model_info["provider"] == "deepseek" and not base_url:
        base_url = "https://api.deepseek.com/v1"
        
    return handler_class(api_key=api_key, base_url=base_url)
