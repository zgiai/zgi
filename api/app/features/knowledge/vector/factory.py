from functools import cache
from typing import Dict, Type, Any
from .base import VectorDBProvider
from .providers.pinecone import PineconeProvider
from .providers.weaviate import WeaviateProvider
from .providers.mock import MockProvider

class VectorDBFactory:
    """Factory for creating vector database providers"""
    
    _providers: Dict[str, Type[VectorDBProvider]] = {
        'pinecone': PineconeProvider,
        'weaviate': WeaviateProvider,
        'mock': MockProvider
    }
    
    @classmethod
    @cache
    def create(cls, provider: str, **kwargs) -> VectorDBProvider:
        """Create a vector database provider instance
        
        Args:
            provider: Provider name
            **kwargs: Provider-specific configuration
            
        Returns:
            VectorDBProvider: Provider instance
            
        Raises:
            ValueError: If provider is not supported
        """
        if provider not in cls._providers:
            raise ValueError(f"Unsupported vector database provider: {provider}")
            
        provider_class = cls._providers[provider]
        return provider_class(**kwargs)
    
    @classmethod
    def register(cls, name: str, provider_class: Type[VectorDBProvider]):
        """Register a new provider
        
        Args:
            name: Provider name
            provider_class: Provider class
        """
        cls._providers[name] = provider_class
