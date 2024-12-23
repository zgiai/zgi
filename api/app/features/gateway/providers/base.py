"""Base provider module."""
from abc import ABC, abstractmethod
from typing import Dict, Any, Optional, AsyncGenerator
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
            base_url: Optional base URL for the API
        """
        self.api_key = api_key
        self.base_url = base_url
        self.client = httpx.AsyncClient(timeout=30.0)
        logger.debug(f"Initialized {self.__class__.__name__} with base_url: {base_url}")
        
    def get_auth_headers(self, user_api_key: Optional[str] = None) -> Dict[str, str]:
        """Get authentication headers.
        
        Args:
            user_api_key: Optional user-provided API key
            
        Returns:
            Dictionary of authentication headers
        """
        api_key = user_api_key or self.api_key
        return {
            "Authorization": f"Bearer {api_key}",
            "Content-Type": "application/json"
        }
    
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
            Chat completion response
        """
        pass
