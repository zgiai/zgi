"""LLM provider router for managing and routing LLM requests."""
from typing import Dict, Type, Optional, Any, List, AsyncGenerator
from functools import lru_cache
import logging

from .base import LLMProvider
from .openai_provider import OpenAIProvider
from .deepseek_provider import DeepSeekProvider
from .anthropic_provider import AnthropicProvider
from .qwen_provider import QwenProvider
from ..config.models import ModelConfig
from ..exceptions.provider_errors import InvalidAPIKeyError
from ..utils.provider_utils import (
    ProviderConfig,
    get_api_key,
    get_base_url,
    is_model_supported,
    validate_model_config,
    log_request_info,
    format_request_data
)
from ..utils.message_converter import extract_system_message, filter_messages_by_role

logger = logging.getLogger(__name__)

class LLMRouter:
    """Router for managing and routing LLM requests."""
    
    PROVIDER_CLASSES = {
        "openai": OpenAIProvider,
        "anthropic": AnthropicProvider,
        "deepseek": DeepSeekProvider,
        "qwen": QwenProvider,
    }
    
    def __init__(self):
        """Initialize provider router."""
        self._provider_instances = {}
        
    def get_provider_for_model(self, model_name: str) -> Optional[Dict[str, Any]]:
        """Get provider information for a model.
        
        Args:
            model_name: Name of the model
            
        Returns:
            Provider information if found, None otherwise
        """
        for provider_id, provider_class in self.PROVIDER_CLASSES.items():
            if any(model_name.startswith(prefix) 
                  for prefix in provider_class.get_supported_prefixes()):
                return {
                    "provider": provider_id,
                    "class": provider_class,
                    "base_url": get_base_url(provider_id)
                }
        return None
    
    def get_provider(self, provider_name: str) -> Optional[LLMProvider]:
        """Get or create provider instance.
        
        Args:
            provider_name: Name of the provider
            
        Returns:
            Provider instance if found, None otherwise
            
        Raises:
            InvalidAPIKeyError: If API key is not found
        """
        if provider_name not in self._provider_instances:
            provider_class = self.PROVIDER_CLASSES.get(provider_name)
            if not provider_class:
                return None
            
            api_key = get_api_key(provider_name)
            if not api_key:
                raise InvalidAPIKeyError(f"No API key found for {provider_name}")
                
            self._provider_instances[provider_name] = provider_class(
                provider_name=provider_name,
                api_key=api_key,
                base_url=get_base_url(provider_name)
            )
            
        return self._provider_instances[provider_name]
    
    async def route_request(
        self,
        params: Dict[str, Any]
    ) -> AsyncGenerator[Dict[str, Any], None]:
        """Route request to appropriate provider.
        
        Args:
            params: Request parameters
                Required:
                    - model: Model name
                    - messages: List of messages
                Optional:
                    - temperature: Temperature value
                    - max_tokens: Maximum tokens
                    - stream: Whether to stream
                    
        Yields:
            Response chunks
            
        Raises:
            ValueError: If required parameters are missing
        """
        # Extract parameters
        model = params.get("model")
        if not model:
            raise ValueError("model parameter is required")
            
        messages = params.get("messages")
        if not messages:
            raise ValueError("messages parameter is required")
            
        # Optional parameters
        temperature = params.get("temperature", 1.0)
        max_tokens = params.get("max_tokens")
        stream = params.get("stream", False)
        
        # Remove processed params and pass rest as kwargs
        known_params = {"model", "messages", "temperature", "max_tokens", "stream"}
        kwargs = {k: v for k, v in params.items() if k not in known_params}
        
        try:
            async for chunk in self.chat_completion(
                messages=messages,
                model=model,
                temperature=temperature,
                max_tokens=max_tokens,
                stream=stream,
                **kwargs
            ):
                yield chunk
        except Exception as e:
            logger.error(f"Request routing failed: {str(e)}")
            raise ValueError(f"Error routing request: {str(e)}")
    
    async def chat_completion(
        self,
        messages: List[Dict[str, Any]],
        model: str,
        temperature: float = 1.0,
        max_tokens: Optional[int] = None,
        stream: bool = False,
        **kwargs: Any
    ) -> AsyncGenerator[Dict[str, Any], None]:
        """Route chat completion request to appropriate provider.
        
        Args:
            messages: List of messages
            model: Model name
            temperature: Temperature value
            max_tokens: Maximum tokens to generate
            stream: Whether to stream responses
            **kwargs: Additional arguments
            
        Yields:
            Response chunks
            
        Raises:
            ValueError: If provider not found or validation fails
            InvalidAPIKeyError: If API key is not found
        """
        # Get provider for model
        provider_info = self.get_provider_for_model(model)
        if not provider_info:
            raise ValueError(f"No provider found for model: {model}")
            
        provider_name = provider_info["provider"]
        
        # Validate request
        self.validate_request(provider_name, model, temperature)
        
        # Get provider instance
        provider = self.get_provider(provider_name)
        if not provider:
            raise ValueError(f"Failed to initialize provider: {provider_name}")
            
        # Extract system message and filter messages
        messages, system = extract_system_message(messages)
        messages = filter_messages_by_role(messages, ["user", "assistant"])
        
        # Prepare request data
        data = format_request_data(
            messages=messages,
            model=model,
            temperature=temperature,
            stream=stream,
            max_tokens=max_tokens,
            system=system,
            **kwargs
        )
        
        # Log request information
        log_request_info(
            data=data,
            headers=provider.headers,
            base_url=provider.base_url,
            endpoint="/v1/chat/completions"
        )
        
        # Route request to provider
        try:
            async for chunk in provider.chat_completion(**data):
                yield chunk
        except Exception as e:
            logger.error(f"Provider {provider_name} request failed: {str(e)}")
            raise
    
    def validate_request(
        self,
        provider: str,
        model: str,
        temperature: float,
        **kwargs: Any
    ) -> None:
        """Validate request parameters.
        
        Args:
            provider: Provider name
            model: Model name
            temperature: Temperature value
            **kwargs: Additional arguments
            
        Raises:
            ValueError: If validation fails
        """
        errors = validate_model_config(provider, model, temperature)
        if errors:
            raise ValueError("\n".join(errors))

@lru_cache()
def get_router() -> LLMRouter:
    """Get singleton instance of LLM router.
    
    Returns:
        LLM router instance
    """
    return LLMRouter()
