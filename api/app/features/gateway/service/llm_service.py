"""Service for handling LLM requests"""
from typing import Dict, Any, AsyncGenerator
from sqlalchemy.orm import Session
import logging

from ..providers.router import LLMRouter

logger = logging.getLogger(__name__)
logger.setLevel(logging.DEBUG)

class LLMService:
    """Service for handling LLM requests"""
    
    _router = None
    
    @classmethod
    def get_router(cls) -> LLMRouter:
        """Get or create the LLM router instance."""
        if cls._router is None:
            cls._router = LLMRouter()
        return cls._router
    
    @classmethod
    async def create_chat_completion(
        cls,
        params: Dict[str, Any],
        db: Session
    ) -> Dict[str, Any] | AsyncGenerator[Dict[str, Any], None]:
        """
        Handle a chat completion request.
        
        Args:
            params: Dictionary containing request parameters
                Required:
                    - model: Name of the model
                    - messages: List of message objects
                Optional:
                    - temperature: Sampling temperature
                    - max_tokens: Maximum tokens to generate
                    - stream: Whether to stream the response
            db: Database session
        
        Returns:
            The chat completion response in unified format
        
        Raises:
            ValueError: If required parameters are missing or invalid
        """
        # Request validation
        if "model" not in params:
            raise ValueError("model parameter is required")
        if "messages" not in params:
            raise ValueError("messages parameter is required")
            
        # Route the request to the appropriate provider
        router = cls.get_router()
        try:
            return await router.route_request(params)
        except Exception as e:
            logger.error(f"Error routing request: {e}", exc_info=True)
            raise
