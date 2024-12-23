"""Provider factory for creating LLM providers."""
import os
from typing import Dict, Any, Optional
import logging

from .base import BaseProvider
from .anthropic_provider import AnthropicProvider
from .deepseek_provider import DeepSeekProvider

logger = logging.getLogger(__name__)
logger.setLevel(logging.DEBUG)

def create_provider(provider_name: str, api_key: str, base_url: Optional[str] = None) -> BaseProvider:
    """Create a provider instance based on provider name.
    
    Args:
        provider_name: Name of the provider (e.g., "anthropic", "deepseek")
        api_key: API key for the provider
        base_url: Optional base URL for the provider API
        
    Returns:
        Provider instance
        
    Raises:
        ValueError: If provider is not supported
    """
    logger.debug(f"Creating provider {provider_name} with base_url: {base_url}")
    
    provider_class = None
    if provider_name == "anthropic":
        provider_class = AnthropicProvider
    elif provider_name == "deepseek":
        provider_class = DeepSeekProvider
    
    if provider_class is None:
        logger.error(f"Unsupported provider: {provider_name}")
        raise ValueError(f"Unsupported provider: {provider_name}")
        
    logger.debug(f"Using provider class: {provider_class.__name__}")
    return provider_class(api_key=api_key, base_url=base_url)
