"""LLM provider router."""
import os
from typing import Dict, Any
import logging
from typing import AsyncGenerator

from .factory import create_provider
from ..config.models import ModelConfig
from ..exceptions.provider_errors import InvalidAPIKeyError

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
    
    def __init__(self):
        """Initialize the router."""
        self._provider_cache = {}
        
        # Debug: Print all environment variables
        logger.debug("Environment variables:")
        for key, value in os.environ.items():
            if any(provider in key.lower() for provider in ["anthropic", "openai", "deepseek"]):
                logger.debug(f"  {key}: {'*' * min(len(value), 10)}")
                
    def get_provider_name(self, model_id: str) -> str:
        """Get provider name from model ID.
        
        Args:
            model_id: ID of the model (e.g., "claude-3-opus-20240229")
            
        Returns:
            Provider name (e.g., "anthropic")
            
        Raises:
            ValueError: If provider cannot be determined
        """
        return ModelConfig.get_provider_for_model(model_id)
        
    def get_provider(self, model_id: str):
        """Get or create provider for a given model.
        
        Args:
            model_id: ID of the model
            
        Returns:
            Provider instance
            
        Raises:
            ValueError: If provider is not properly configured
        """
        provider_name = self.get_provider_name(model_id)
        logger.debug(f"Getting provider instance for: {provider_name}")
        
        if provider_name not in self._provider_cache:
            # List all possible environment variable names
            possible_keys = [
                f"{provider_name.upper()}_API_KEY",
                f"{provider_name.upper()}_KEY",
                f"{provider_name.upper()}_SECRET_KEY"
            ]
            possible_urls = [
                f"{provider_name.upper()}_API_BASE",
                f"{provider_name.upper()}_BASE_URL",
                f"{provider_name.upper()}_URL"
            ]
            
            # Try to find API key
            api_key = None
            for key in possible_keys:
                api_key = os.environ.get(key)
                if api_key:
                    logger.debug(f"Found API key in {key}")
                    break
                    
            # Try to find base URL
            base_url = None
            for url in possible_urls:
                base_url = os.environ.get(url)
                if base_url:
                    logger.debug(f"Found base URL in {url}")
                    break
                    
            # Use default base URL if none found
            base_url = base_url or self.DEFAULT_BASE_URLS[provider_name]
            
            if not api_key:
                logger.error(f"Missing API key. Tried: {', '.join(possible_keys)}")
                raise ValueError(f"Provider {provider_name} is not properly configured: missing API key")
                
            # Check if API key looks valid
            if len(api_key.strip()) < 10:  # Arbitrary minimum length
                logger.warning(f"API key for {provider_name} looks suspiciously short")
                
            logger.debug(f"Creating provider for {provider_name} with base_url: {base_url}")
            logger.debug(f"API key length: {len(api_key)}")
            
            try:
                self._provider_cache[provider_name] = create_provider(
                    provider_name=provider_name,
                    base_url=base_url
                )
            except Exception as e:
                logger.error(f"Error creating provider {provider_name}: {str(e)}")
                raise ValueError(f"Provider {provider_name} initialization failed: {str(e)}")
            
        return self._provider_cache[provider_name]
        
    async def route_request(self, request_params: Dict[str, Any]) -> AsyncGenerator[Dict[str, Any], None]:
        """Route a request to the appropriate provider.
        
        Args:
            request_params: Request parameters
            
        Returns:
            Provider response
            
        Raises:
            ValueError: If request cannot be routed
            InvalidAPIKeyError: If no valid API key is found
        """
        try:
            model_id = request_params["model"]
            provider_name = self.get_provider_name(model_id)
            logger.debug(f"Routing request for model {model_id} to provider {provider_name}")
            
            # Get API key from request or environment
            api_key = request_params.get("api_key")
            if not api_key or api_key == "undefined":
                env_key = os.environ.get(f"{provider_name.upper()}_API_KEY")
                if not env_key:
                    raise InvalidAPIKeyError(
                        f"No API key provided for {provider_name}. Please set the {provider_name.upper()}_API_KEY "
                        "environment variable or provide the key in the request."
                    )
                api_key = env_key
                logger.debug(f"Using API key from environment variable {provider_name.upper()}_API_KEY")
            
            # Get base URL from request or default
            base_url = request_params.get("base_url") or ModelConfig.get_default_base_url(provider_name)
            
            # Create or get cached provider instance
            cache_key = provider_name
            if cache_key not in self._provider_cache:
                self._provider_cache[cache_key] = create_provider(
                    provider_name=provider_name
                )
            
            provider = self._provider_cache[cache_key]
            
            # Extract required parameters
            model = request_params["model"]
            messages = request_params["messages"]
            temperature = request_params.get("temperature", 1.0)
            max_tokens = request_params.get("max_tokens")
            stream = request_params.get("stream", False)
            
            # Add base_url to kwargs if provided
            kwargs = {}
            if base_url:
                kwargs["base_url"] = base_url
                
            # Call chat_completion and yield responses
            async for response in provider.chat_completion(
                messages=messages,
                model=model,
                temperature=temperature,
                max_tokens=max_tokens,
                stream=stream,
                **kwargs
            ):
                yield response
        except Exception as e:
            logger.error(f"Error routing request: {str(e)}")
            raise
