"""Model configuration for providers."""
from typing import Dict, Set, List

class ModelConfig:
    """Configuration for provider models."""
    
    # Model prefixes for each provider
    MODEL_PREFIXES = {
        "anthropic": ["claude"],
        "openai": ["gpt"],
        "deepseek": ["deepseek"]
    }
    
    # Supported models for each provider
    SUPPORTED_MODELS = {
        "anthropic": {
            "claude-3-opus-20240229",
            "claude-3-sonnet-20240229",
            "claude-3-haiku-20240307",
            "claude-2.1",
            "claude-2.0",
            "claude-instant-1.2"
        },
        "openai": {
            "gpt-4",
            "gpt-4-turbo-preview",
            "gpt-4-0125-preview",
            "gpt-4-1106-preview",
            "gpt-4-0613",
            "gpt-4-32k",
            "gpt-4-32k-0613",
            "gpt-3.5-turbo",
            "gpt-3.5-turbo-0125",
            "gpt-3.5-turbo-1106",
            "gpt-3.5-turbo-0613",
            "gpt-3.5-turbo-16k",
            "gpt-3.5-turbo-16k-0613"
        },
        "deepseek": {
            "deepseek-chat",
            "deepseek-coder",
            "deepseek-coder-instruct",
            "deepseek-coder-instruct-6.7b",
            "deepseek-coder-instruct-33b"
        }
    }
    
    # Default base URLs for providers
    DEFAULT_BASE_URLS = {
        "anthropic": "https://api.anthropic.com",
        "openai": "https://api.openai.com/v1",
        "deepseek": "https://api.deepseek.com/v1"
    }
    
    @classmethod
    def get_provider_for_model(cls, model_id: str) -> str:
        """Get provider name from model ID.
        
        Args:
            model_id: Model identifier
            
        Returns:
            Provider name
            
        Raises:
            ValueError: If provider cannot be determined
        """
        model_id = model_id.lower()
        for provider, prefixes in cls.MODEL_PREFIXES.items():
            for prefix in prefixes:
                if model_id.startswith(prefix):
                    if model_id not in cls.SUPPORTED_MODELS[provider]:
                        supported = sorted(cls.SUPPORTED_MODELS[provider])
                        raise ValueError(
                            f"Model {model_id} is not supported by {provider}. "
                            f"Supported models: {', '.join(supported)}"
                        )
                    return provider
        raise ValueError(f"Could not determine provider for model: {model_id}")
    
    @classmethod
    def get_supported_models(cls, provider: str) -> Set[str]:
        """Get supported models for provider.
        
        Args:
            provider: Provider name
            
        Returns:
            Set of supported model names
        """
        return cls.SUPPORTED_MODELS.get(provider, set())
    
    @classmethod
    def get_default_base_url(cls, provider: str) -> str:
        """Get default base URL for provider.
        
        Args:
            provider: Provider name
            
        Returns:
            Default base URL
        """
        return cls.DEFAULT_BASE_URLS.get(provider, "")
