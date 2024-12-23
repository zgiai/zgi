"""Provider registry for managing LLM providers."""
from typing import Dict, Type, Optional
from functools import lru_cache
from .base import BaseProvider
from .openai_provider import OpenAIProvider
from .deepseek_provider import DeepSeekProvider
from .anthropic_provider import AnthropicProvider

class ProviderRegistry:
    """Registry for managing LLM providers."""
    
    def __init__(self):
        """Initialize provider registry."""
        # Map provider prefixes to provider classes
        self._provider_mapping = {
            "gpt": ("openai", OpenAIProvider),
            "claude": ("anthropic", AnthropicProvider),
            "deepseek": ("deepseek", DeepSeekProvider)
        }
                
    def get_provider_for_model(self, model_name: str) -> Optional[Dict[str, any]]:
        """Get provider information for a given model.
        
        Args:
            model_name: Name of the model (e.g., gpt-4, claude-3-opus-20240229)
            
        Returns:
            Provider information if found, None otherwise
        """
        # Find matching provider based on model name prefix
        for prefix, (provider_id, provider_class) in self._provider_mapping.items():
            if model_name.startswith(prefix):
                return {
                    "provider": provider_id,
                    "class": provider_class
                }
        return None

@lru_cache()
def get_provider_registry() -> ProviderRegistry:
    """Get singleton instance of provider registry."""
    return ProviderRegistry()

def get_provider(provider_name: str) -> Optional[Type[BaseProvider]]:
    """Get provider class by name.
    
    Args:
        provider_name: Name of the provider (e.g., openai, anthropic)
        
    Returns:
        Provider class if found, None otherwise
    """
    registry = get_provider_registry()
    for _, (provider_id, provider_class) in registry._provider_mapping.items():
        if provider_id == provider_name:
            return provider_class
    return None
