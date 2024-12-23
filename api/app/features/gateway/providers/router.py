"""LLM router module."""
from typing import Dict, Any, Optional, Type
import logging
from .base import BaseProvider
from .anthropic_provider import AnthropicProvider
from .openai_provider import OpenAIProvider

# Configure logging
logger = logging.getLogger(__name__)
logger.setLevel(logging.DEBUG)

class LLMRouter:
    """Router for LLM providers."""
    
    def __init__(self):
        """Initialize the LLM router."""
        self._providers = {
            "anthropic": AnthropicProvider,
            "openai": OpenAIProvider
        }
        
        self._model_to_provider = {
            # Anthropic models
            "claude-3-opus-20240229": "anthropic",
            "claude-3-sonnet-20240229": "anthropic",
            "claude-3-haiku-20240229": "anthropic",
            # OpenAI models
            "gpt-4-turbo-preview": "openai",
            "gpt-4": "openai",
            "gpt-3.5-turbo": "openai"
        }
        
    def get_provider_name(self, model: str) -> str:
        """Get provider name for a model.
        
        Args:
            model: Model name
            
        Returns:
            Provider name
            
        Raises:
            ValueError: If model is not supported
        """
        provider = self._model_to_provider.get(model)
        if not provider:
            raise ValueError(f"Unsupported model: {model}")
        return provider
        
    def get_provider_class(self, provider_name: str) -> Type[BaseProvider]:
        """Get provider class by name.
        
        Args:
            provider_name: Provider name
            
        Returns:
            Provider class
            
        Raises:
            ValueError: If provider is not supported
        """
        provider_class = self._providers.get(provider_name)
        if not provider_class:
            raise ValueError(f"Unsupported provider: {provider_name}")
        return provider_class
        
    async def route_request(self, params: Dict[str, Any]) -> Dict[str, Any]:
        """Route a request to the appropriate provider.
        
        Args:
            params: Request parameters including:
                - model: Model name
                - api_key: Optional API key
                - messages: List of messages
                - temperature: Optional sampling temperature
                - max_tokens: Optional maximum tokens
                - stream: Optional streaming flag
                
        Returns:
            Provider response
            
        Raises:
            ValueError: If request is invalid
        """
        try:
            # Get provider name from model
            model = params.get("model")
            if not model:
                raise ValueError("Model is required")
                
            provider_name = self.get_provider_name(model)
            provider_class = self.get_provider_class(provider_name)
            
            # Create provider instance with API key if provided
            api_key = params.pop("api_key", None)
            provider = provider_class(api_key=api_key) if api_key else provider_class()
            
            # Route request to provider
            logger.debug(f"Routing request to provider {provider_name}")
            return await provider.create_chat_completion(params)
            
        except Exception as e:
            logger.error(f"Error routing request: {str(e)}")
            raise ValueError(f"Error routing request: {str(e)}")
