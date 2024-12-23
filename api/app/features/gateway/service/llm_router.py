"""LLM router service."""
import os
from typing import Dict, Any
import logging
import re

from ..providers.factory import create_provider

logger = logging.getLogger(__name__)
logger.setLevel(logging.DEBUG)

class LLMRouter:
    """Router for LLM requests."""
    
    # Default base URLs for providers
    DEFAULT_BASE_URLS = {
        "anthropic": "https://api.anthropic.com",
        "openai": "https://api.openai.com/v1",
        "deepseek": "https://api.deepseek.com/v1"
    }
    
    def __init__(self):
        """Initialize the router."""
        self._provider_cache = {}
        
    def get_provider_name(self, model_id: str) -> str:
        """Get provider name from model ID.
        
        Args:
            model_id: ID of the model (e.g., "claude-3-opus-20240229")
            
        Returns:
            Provider name (e.g., "anthropic")
            
        Raises:
            ValueError: If provider cannot be determined
        """
        logger.debug(f"Getting provider for model: {model_id}")
        
        # Extract provider name from model ID
        if model_id.startswith("claude"):
            return "anthropic"
        elif model_id.startswith("gpt"):
            return "openai"
        elif model_id.startswith("deepseek"):
            return "deepseek"
        else:
            raise ValueError(f"Cannot determine provider for model: {model_id}")
        
    def get_provider(self, model_id: str):
        """Get or create provider for a given model.
        
        Args:
            model_id: ID of the model
            
        Returns:
            Provider instance
        """
        provider_name = self.get_provider_name(model_id)
        logger.debug(f"Getting provider instance for: {provider_name}")
        
        if provider_name not in self._provider_cache:
            api_key_var = f"{provider_name.upper()}_API_KEY"
            base_url_var = f"{provider_name.upper()}_BASE_URL"
            
            api_key = os.environ.get(api_key_var)
            base_url = os.environ.get(base_url_var, self.DEFAULT_BASE_URLS[provider_name])
            
            if not api_key:
                raise ValueError(f"Missing API key: {api_key_var}")
                
            logger.debug(f"Creating provider for {provider_name} with base_url: {base_url}")
            self._provider_cache[provider_name] = create_provider(
                provider_name=provider_name,
                api_key=api_key,
                base_url=base_url
            )
            
        return self._provider_cache[provider_name]
        
    async def route_request(self, request_params: Dict[str, Any]):
        """Route a request to the appropriate provider.
        
        Args:
            request_params: Request parameters
            
        Returns:
            Provider response
        """
        model_id = request_params["model"]
        provider = self.get_provider(model_id)
        
        logger.debug(f"Routing request to provider with params: {request_params}")
        return await provider.create_chat_completion(request_params)
