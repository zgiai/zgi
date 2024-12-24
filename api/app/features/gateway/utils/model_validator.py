"""Model validation utilities."""
from typing import Dict, List, Optional, Set
from ..exceptions.provider_errors import InvalidRequestError

class ModelValidator:
    """Model validator for different providers."""
    
    _provider_models: Dict[str, Set[str]] = {}
    
    @classmethod
    def register_provider_models(cls, provider: str, models: Set[str]):
        """Register supported models for a provider.
        
        Args:
            provider: Provider name
            models: Set of supported model names
        """
        cls._provider_models[provider] = models
    
    @classmethod
    def validate_model(cls, provider: str, model: str) -> bool:
        """Validate if model is supported by provider.
        
        Args:
            provider: Provider name
            model: Model name
            
        Returns:
            True if model is supported
            
        Raises:
            InvalidRequestError: If model is not supported
        """
        supported_models = cls._provider_models.get(provider, set())
        if not supported_models:
            raise InvalidRequestError(f"Unknown provider: {provider}")
            
        if model not in supported_models:
            supported = ", ".join(sorted(supported_models))
            raise InvalidRequestError(
                f"Model {model} is not supported by {provider}. "
                f"Supported models: {supported}"
            )
        return True
        
    @classmethod
    def get_supported_models(cls, provider: str) -> List[str]:
        """Get supported models for provider.
        
        Args:
            provider: Provider name
            
        Returns:
            List of supported model names
        """
        return sorted(cls._provider_models.get(provider, set()))
