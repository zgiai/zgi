"""Service for handling LLM requests"""
from typing import Dict, Any, AsyncGenerator
from sqlalchemy.orm import Session
from ..providers.registry import get_provider, MODEL_REGISTRY
from ..service.api_key_service import APIKeyService
import logging

class LLMService:
    """Service for handling LLM requests"""
    
    @staticmethod
    async def create_chat_completion(
        params: Dict[str, Any],
        db: Session
    ) -> Dict[str, Any] | AsyncGenerator[Dict[str, Any], None]:
        """
        Handle a chat completion request.
        
        Args:
            params: Dictionary containing request parameters
                Required:
                    - model: Name of the model to use
                    - messages: List of message objects
                    - api_key: API key for authentication
                Optional:
                    - base_url: Base URL for the provider API
                    - temperature: Sampling temperature
                    - max_tokens: Maximum tokens to generate
                    - stream: Whether to stream the response
            db: Database session
        
        Returns:
            The chat completion response in unified format
        
        Raises:
            ValueError: If required parameters are missing or invalid
        """
        # 1. Request Validation
        if "model" not in params:
            raise ValueError("model parameter is required")
        if "messages" not in params:
            raise ValueError("messages parameter is required")
        if "api_key" not in params:
            raise ValueError("api_key is required")

        model = params["model"]
        api_key = params["api_key"]

        # Get provider name
        model_info = MODEL_REGISTRY.get(model)
        if not model_info:
            raise ValueError(f"Model {model} is not supported")
        provider_name = model_info["provider"]

        # Get provider API key
        provider_api_key = await APIKeyService.get_provider_key(db, api_key, provider_name)
        if not provider_api_key:
            raise ValueError(f"No API key found for provider {provider_name}")
            
        logging.debug(f"Using API key: {provider_api_key} for provider {provider_name}")
        logging.debug(f"Model: {model}")
        logging.debug(f"Provider: {provider_name}")

        # 2. Get Provider with correct API key
        provider = get_provider(
            model_name=model,
            api_key=provider_api_key,
            base_url=params.get("base_url")
        )

        # 3. Send Request to Provider
        return await provider.handle_request(params)
