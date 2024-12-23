"""Provider factory module."""
from typing import Optional
from .base import BaseProvider
from .openai_provider import OpenAIProvider
from .anthropic_provider import AnthropicProvider
from .deepseek_provider import DeepSeekProvider

def create_provider(
    provider_name: str,
    api_key: str,
    base_url: Optional[str] = None
) -> BaseProvider:
    """Create a provider instance.
    
    Args:
        provider_name: Name of the provider (e.g., openai, anthropic)
        api_key: API key for authentication
        base_url: Optional base URL override
        
    Returns:
        Provider instance
        
    Raises:
        ValueError: If provider name is invalid
    """
    providers = {
        "openai": OpenAIProvider,
        "anthropic": AnthropicProvider,
        "deepseek": DeepSeekProvider
    }
    
    provider_class = providers.get(provider_name)
    if not provider_class:
        raise ValueError(f"Invalid provider name: {provider_name}")
        
    return provider_class(api_key=api_key, base_url=base_url)
