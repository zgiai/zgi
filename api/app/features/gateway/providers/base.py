"""Base provider class for LLM providers."""
from abc import ABC, abstractmethod
from typing import Dict, Any, AsyncGenerator, Optional
import logging
from ..utils.http import HttpClient
from ..utils.formatter import format_chat_messages, format_completion_response
from ..exceptions.provider_errors import ProviderError, InvalidAPIKeyError
from ..utils.model_validator import ModelValidator

logger = logging.getLogger(__name__)

class BaseProvider(ABC):
    """Base class for LLM providers."""
    
    @property
    @abstractmethod
    def provider_name(self) -> str:
        """Get provider name.
        
        Returns:
            Provider name
        """
        pass
    
    def __init__(self, api_key: str, base_url: Optional[str] = None):
        """Initialize the provider.
        
        Args:
            api_key: API key for authentication
            base_url: Optional base URL override
        """
        self.api_key = self.validate_api_key(api_key)
        self.base_url = base_url or self.default_base_url()
        self.http_client = HttpClient(self.base_url, self.get_headers())
        logger.debug(f"Initialized {self.__class__.__name__} with base_url: {base_url}")
    
    @abstractmethod
    def validate_api_key(self, api_key: str) -> str:
        """Validate and format API key.
        
        Args:
            api_key: Raw API key
            
        Returns:
            Validated and formatted API key
            
        Raises:
            InvalidAPIKeyError: If API key is invalid
        """
        pass
        
    @abstractmethod
    def default_base_url(self) -> str:
        """Get default base URL for provider.
        
        Returns:
            Default base URL
        """
        pass
        
    @abstractmethod
    def get_headers(self) -> Dict[str, str]:
        """Get HTTP headers for provider.
        
        Returns:
            HTTP headers
        """
        pass
        
    def validate_model(self, model: str) -> None:
        """Validate if model is supported.
        
        Args:
            model: Model name
            
        Raises:
            InvalidRequestError: If model is not supported
        """
        ModelValidator.validate_model(self.provider_name, model)
        
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
            
        Raises:
            ProviderError: On provider error
        """
        # Validate model before proceeding
        self.validate_model(params["model"])
        pass
        
    def format_messages(self, messages: Dict[str, Any]) -> Dict[str, Any]:
        """Format messages for provider.
        
        Args:
            messages: Raw messages
            
        Returns:
            Formatted messages
        """
        return format_chat_messages(messages, self.provider_name)
        
    def format_response(self, response: Dict[str, Any]) -> Dict[str, Any]:
        """Format provider response.
        
        Args:
            response: Raw provider response
            
        Returns:
            Formatted response
        """
        return format_completion_response(response, self.provider_name)
