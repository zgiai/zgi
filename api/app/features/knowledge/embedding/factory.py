from typing import Dict, Type
from .base import EmbeddingProvider
from .providers.openai import OpenAIEmbeddingProvider
from .providers.mock import MockEmbeddingProvider

class EmbeddingFactory:
    """Factory for creating embedding providers"""
    
    _providers: Dict[str, Type[EmbeddingProvider]] = {
        'openai': OpenAIEmbeddingProvider,
        'mock': MockEmbeddingProvider
    }
    
    @classmethod
    def create(cls, provider: str, **kwargs) -> EmbeddingProvider:
        """Create an embedding provider instance
        
        Args:
            provider: Provider name
            **kwargs: Provider-specific configuration
            
        Returns:
            EmbeddingProvider: Provider instance
            
        Raises:
            ValueError: If provider is not supported
        """
        if provider not in cls._providers:
            raise ValueError(f"Unsupported embedding provider: {provider}")
            
        provider_class = cls._providers[provider]
        return provider_class(**kwargs)
    
    @classmethod
    def register(cls, name: str, provider_class: Type[EmbeddingProvider]):
        """Register a new provider
        
        Args:
            name: Provider name
            provider_class: Provider class
        """
        cls._providers[name] = provider_class
