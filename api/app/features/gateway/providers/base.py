"""Base provider class for LLM providers."""
from abc import ABC, abstractmethod
from typing import Dict, Any, AsyncGenerator, Optional
import httpx
import logging

logger = logging.getLogger(__name__)
logger.setLevel(logging.DEBUG)

class BaseProvider(ABC):
    """Base class for LLM providers."""
    
    def __init__(self, api_key: str, base_url: Optional[str] = None):
        """Initialize the provider.
        
        Args:
            api_key: API key for authentication
            base_url: Optional base URL override
        """
        self.api_key = api_key
        self.base_url = base_url
        logger.debug(f"Initialized {self.__class__.__name__} with base_url: {base_url}")
        
    @abstractmethod
    async def create_chat_completion(
        self,
        params: Dict[str, Any]
    ) -> Dict[str, Any] | AsyncGenerator[Dict[str, Any], None]:
        """Create a chat completion.
        
        Args:
            params: Dictionary containing request parameters
                Required:
                    - messages: List of message objects
                    - model: Model name
                Optional:
                    - temperature: Sampling temperature
                    - max_tokens: Maximum tokens to generate
                    - stream: Whether to stream the response
                    
        Returns:
            Chat completion response in unified format
        """
        raise NotImplementedError("Subclasses must implement create_chat_completion")
