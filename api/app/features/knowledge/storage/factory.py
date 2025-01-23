from functools import cache
from typing import Dict, Type
from .base import StorageProvider
from .providers.local_provider import LocalStorageProvider
from .providers.qiniu_provider import QiniuStorageProvider


class StorageFactory:
    """Factory for creating storage providers"""

    _providers: Dict[str, Type[StorageProvider]] = {
        'local': LocalStorageProvider,
        'qiniu': QiniuStorageProvider
    }

    @classmethod
    @cache
    def create(cls, provider: str, **kwargs) -> StorageProvider:
        """Create a storage provider instance

        Args:
            provider: Provider name
            **kwargs: Provider-specific configuration

        Returns:
            StorageProvider: Provider instance

        Raises:
            ValueError: If provider is not supported
        """
        if provider not in cls._providers:
            raise ValueError(f"Unsupported storage provider: {provider}")

        provider_class = cls._providers[provider]
        return provider_class(**kwargs)

    @classmethod
    def register(cls, name: str, provider_class: Type[StorageProvider]):
        """Register a new provider

        Args:
            name: Provider name
            provider_class: Provider class
        """
        cls._providers[name] = provider_class
