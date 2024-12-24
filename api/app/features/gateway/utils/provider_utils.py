"""Utility functions for provider management."""
import os
from typing import Dict, Any, Optional, List
import logging

logger = logging.getLogger(__name__)

class ProviderConfig:
    """Configuration constants for providers."""
    
    # Default base URLs for providers
    DEFAULT_BASE_URLS = {
        "anthropic": "https://api.anthropic.com",
        "openai": "https://api.openai.com/v1",
        "deepseek": "https://api.deepseek.com/v1"
    }
    
    # Environment variable names for API keys
    API_KEY_ENV_VARS = {
        "openai": "OPENAI_API_KEY",
        "anthropic": "ANTHROPIC_API_KEY",
        "deepseek": "DEEPSEEK_API_KEY"
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
            "gpt-3.5-turbo-1106"
        },
        "deepseek": {
            "deepseek-chat",
            "deepseek-coder"
        }
    }

def get_api_key(provider: str) -> Optional[str]:
    """Get API key from environment variables.
    
    Args:
        provider: Provider name
        
    Returns:
        API key if found, None otherwise
    """
    env_var = ProviderConfig.API_KEY_ENV_VARS.get(provider)
    if not env_var:
        return None
    return os.getenv(env_var)

def get_base_url(provider: str) -> Optional[str]:
    """Get base URL for provider.
    
    Args:
        provider: Provider name
        
    Returns:
        Base URL if found, None otherwise
    """
    return ProviderConfig.DEFAULT_BASE_URLS.get(provider)

def is_model_supported(provider: str, model: str) -> bool:
    """Check if a model is supported by a provider.
    
    Args:
        provider: Provider name
        model: Model name
        
    Returns:
        True if model is supported, False otherwise
    """
    supported_models = ProviderConfig.SUPPORTED_MODELS.get(provider, set())
    return model in supported_models

def validate_model_config(provider: str, model: str, temperature: float) -> List[str]:
    """Validate model configuration.
    
    Args:
        provider: Provider name
        model: Model name
        temperature: Temperature value
        
    Returns:
        List of validation errors, empty if valid
    """
    errors = []
    
    if not is_model_supported(provider, model):
        errors.append(f"Model {model} is not supported by provider {provider}")
        
    if not 0 <= temperature <= 2:
        errors.append(f"Temperature {temperature} is out of range [0, 2]")
        
    return errors

def format_request_data(
    messages: List[Dict[str, Any]],
    model: str,
    temperature: float,
    stream: bool,
    max_tokens: Optional[int] = None,
    **kwargs: Any
) -> Dict[str, Any]:
    """Format request data for API calls.
    
    Args:
        messages: List of messages
        model: Model name
        temperature: Temperature value
        stream: Whether to stream responses
        max_tokens: Maximum tokens to generate
        **kwargs: Additional arguments
        
    Returns:
        Formatted request data
    """
    data = {
        "model": model,
        "messages": messages,
        "temperature": temperature,
        "stream": stream
    }
    
    if max_tokens is not None:
        data["max_tokens"] = max_tokens
        
    # Add any additional parameters
    data.update(kwargs)
    
    return data

def log_request_info(
    data: Dict[str, Any],
    headers: Dict[str, str],
    base_url: str,
    endpoint: str
) -> None:
    """Log request information for debugging.
    
    Args:
        data: Request data
        headers: Request headers
        base_url: Base URL
        endpoint: API endpoint
    """
    logger.debug("Request Information:")
    logger.debug(f"URL: {base_url}{endpoint}")
    logger.debug("Headers:")
    for key, value in headers.items():
        # Mask sensitive information
        if "key" in key.lower():
            value = "***"
        logger.debug(f"  {key}: {value}")
    logger.debug("Data:")
    logger.debug(data)
