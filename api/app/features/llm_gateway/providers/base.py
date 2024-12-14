from abc import ABC, abstractmethod
from typing import Dict, Any, Optional

class BaseProvider(ABC):
    """Base class for all LLM providers"""
    
    def __init__(self, api_key: str, base_url: Optional[str] = None):
        self.api_key = api_key
        self.base_url = base_url

    @abstractmethod
    async def handle_request(self, params: Dict[str, Any]) -> Dict[str, Any]:
        """Handle a chat completion request"""
        pass

    @abstractmethod
    async def validate_api_key(self) -> bool:
        """Validate the API key"""
        pass

    @abstractmethod
    def get_model_info(self) -> Dict[str, Any]:
        """Get information about the model"""
        pass
