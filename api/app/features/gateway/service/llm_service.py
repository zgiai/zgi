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
        logger.debug(f"Processing chat completion request with params: {params}")
        
        # Request validation
        if "model" not in params:
            logger.error("Missing required parameter: model")
            raise ValueError("model parameter is required")
            
        if "messages" not in params or not params["messages"]:
            logger.error("Missing required parameter: messages")
            raise ValueError("messages parameter is required and cannot be empty")
            
        # Get router instance
        router = cls.get_router()
        
        try:
            # Route request to appropriate provider
            logger.debug(f"Routing request to provider for model: {params['model']}")
            response = await router.route_request(params)
            logger.debug(f"Got response from provider: {response}")
            return response
            
        except ValueError as e:
            # Re-raise provider errors
            logger.error(f"Provider error: {str(e)}")
            raise
            
        except Exception as e:
            # Log unexpected errors
            logger.error(f"Unexpected error in chat completion: {str(e)}")
            raise ValueError(f"Error processing request: {str(e)}")
