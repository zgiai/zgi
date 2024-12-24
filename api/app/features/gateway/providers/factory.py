"""Provider factory for creating LLM providers."""
import os
from typing import Dict, Any, Optional, Type
import logging
from functools import wraps

from .base import LLMProvider
from ..exceptions.provider_errors import ProviderError

logger = logging.getLogger(__name__)

class ProviderRegistry:
    """Registry for provider classes."""
    
    _providers: Dict[str, Type[LLMProvider]] = {}
    
    @classmethod
    def register(cls, name: Optional[str] = None):
        """Register provider class decorator.
        
        Args:
            name: Optional provider name override
        """
        def decorator(provider_class: Type[LLMProvider]):
            provider_name = name or provider_class.__name__.lower().replace('provider', '')
            cls._providers[provider_name] = provider_class
            logger.debug(f"Registered provider {provider_name}: {provider_class.__name__}")
            return provider_class
        return decorator
    
    @classmethod
    def get_provider(cls, name: str) -> Type[LLMProvider]:
        """Get provider class by name.
        
        Args:
            name: Provider name
            
        Returns:
            Provider class
            
        Raises:
            ProviderError: If provider not found
        """
        provider_class = cls._providers.get(name)
        if not provider_class:
            raise ProviderError(f"Provider not found: {name}")
        return provider_class
    
    @classmethod
    def list_providers(cls) -> Dict[str, Type[LLMProvider]]:
        """Get all registered providers.
        
        Returns:
            Dictionary of provider name to provider class
        """
        return cls._providers.copy()

def create_provider(provider_name: str) -> LLMProvider:
    """Create a provider instance.
    
    Args:
        provider_name: Provider name
        
    Returns:
        Provider instance
        
    Raises:
        ProviderError: If provider not found or initialization fails
    """
    try:
        provider_class = ProviderRegistry.get_provider(provider_name)
        return provider_class()
    except Exception as e:
        logger.error(f"Failed to create provider {provider_name}: {str(e)}")
        raise ProviderError(f"Failed to create provider {provider_name}: {str(e)}") from e

# Import and register providers
from .anthropic_provider import AnthropicProvider
from .openai_provider import OpenAIProvider 
from .deepseek_provider import DeepSeekProvider

ProviderRegistry.register()(AnthropicProvider)
ProviderRegistry.register()(OpenAIProvider)
ProviderRegistry.register()(DeepSeekProvider)
