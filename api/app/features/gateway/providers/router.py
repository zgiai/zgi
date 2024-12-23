"""LLM provider router."""
import os
from typing import Dict, Any
import logging
import re

from .factory import create_provider

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
    
    # Model patterns for each provider
    MODEL_PATTERNS = {
        "anthropic": [
            r"^claude-\d",  # claude-1, claude-2, claude-3
            r"^claude-instant",  # claude-instant
        ],
        "openai": [
            r"^gpt-\d",  # gpt-3.5, gpt-4
        ],
        "deepseek": [
            r"^deepseek-chat$",  # deepseek-chat
            r"^deepseek-coder",  # deepseek-coder
        ]
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
        model_id = model_id.lower()
        logger.debug(f"Lowercase model ID: {model_id}")
        
        # Try to match model ID against each provider's patterns
        for provider, patterns in self.MODEL_PATTERNS.items():
            for pattern in patterns:
                if re.match(pattern, model_id):
                    logger.debug(f"Model {model_id} matched pattern {pattern} for provider {provider}")
                    return provider
                    
        # If no pattern matches, try exact match with model name
        if model_id == "deepseek-chat":
            logger.debug("Model matched exact name 'deepseek-chat'")
            return "deepseek"
            
        logger.error(f"No provider pattern matched for model: {model_id}")
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
                logger.error(f"Missing API key: {api_key_var}")
                raise ValueError(f"Missing API key: {api_key_var}")
                
            # Check if API key looks valid
            if len(api_key.strip()) < 10:  # Arbitrary minimum length
                logger.warning(f"API key for {provider_name} looks suspiciously short")
                
            logger.debug(f"Creating provider for {provider_name} with base_url: {base_url}")
            logger.debug(f"API key length: {len(api_key)}")
            
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
        
        logger.debug(f"Routing request to {provider.__class__.__name__} with params: {request_params}")
        return await provider.create_chat_completion(request_params)
