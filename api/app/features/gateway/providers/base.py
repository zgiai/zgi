"""Base provider class for LLM providers."""
from abc import ABC, abstractmethod
from typing import Dict, Any, AsyncGenerator, Optional
import logging
import os

logger = logging.getLogger(__name__)

class LLMProvider(ABC):
    """Base class for LLM providers."""
    
    def __init__(self, provider_name: str):
        """Initialize the provider.
        
        Args:
            provider_name: Name of the provider
        """
        self.provider_name = provider_name
        self.api_key = self._get_api_key()
        
    def _get_api_key(self) -> str:
        """Get API key from environment variable.
        
        Returns:
            API key
            
        Raises:
            ValueError: If API key is not found
        """
        env_var = f"{self.provider_name.upper()}_API_KEY"
        api_key = os.getenv(env_var)
        if not api_key:
            raise ValueError(f"No API key found in environment variable {env_var}")
        return api_key
        
    @abstractmethod
    async def chat_completion(
        self,
        messages: list[Dict[str, Any]],
        model: str,
        temperature: float = 1.0,
        max_tokens: Optional[int] = None,
        stream: bool = False,
        **kwargs: Any
    ) -> AsyncGenerator[Dict[str, Any], None]:
        """Generate chat completion.
        
        Args:
            messages: List of messages
            model: Model name
            temperature: Sampling temperature
            max_tokens: Maximum tokens to generate
            stream: Whether to stream the response
            **kwargs: Additional arguments
            
        Yields:
            Response chunks
        """
        pass
