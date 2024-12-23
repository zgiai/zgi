"""Anthropic provider implementation"""
import json
from typing import Dict, Any, AsyncGenerator, Optional, List
import httpx
import logging
from .base import BaseProvider

# Configure logging
logger = logging.getLogger(__name__)
logger.setLevel(logging.DEBUG)

class AnthropicProvider(BaseProvider):
    """Provider implementation for Anthropic's Claude models"""
    
    def __init__(self, api_key: str, base_url: Optional[str] = None):
        """Initialize the Anthropic provider.
        
        Args:
            api_key: Anthropic API key
            base_url: Optional base URL for the API
        """
        super().__init__(api_key, base_url)
        self.base_url = base_url or "https://api.anthropic.com"
        logger.debug(f"Initializing Anthropic provider with base URL: {self.base_url}")
        
    def get_headers(self) -> Dict[str, str]:
        """Get request headers.
        
        Returns:
            Headers dictionary
        """
        # Remove "Bearer " prefix if present
        api_key = self.api_key
        if api_key.startswith("Bearer "):
            api_key = api_key[7:]
            
        return {
            "x-api-key": api_key,
            "anthropic-version": "2023-06-01",
            "content-type": "application/json",
        }
        
    async def create_chat_completion(
        self,
        params: Dict[str, Any]
    ) -> Dict[str, Any] | AsyncGenerator[Dict[str, Any], None]:
        """Create a chat completion using Anthropic's API.
        
        Args:
            params: Dictionary containing request parameters
                Required:
                    - messages: List of message objects
                    - model: Model name (e.g., claude-3-opus-20240229)
                Optional:
                    - temperature: Sampling temperature
                    - max_tokens: Maximum tokens to generate
                    - stream: Whether to stream the response
                    
        Returns:
            Chat completion response
            
        Raises:
            httpx.HTTPError: If the request fails
        """
        logger.debug("Creating chat completion with params: %s", params)
        
        # Extract system message if present
        system = None
        messages = []
        for msg in params["messages"]:
            role = msg["role"]
            if role == "system":
                system = msg["content"]
            elif role == "assistant":
                messages.append({
                    "role": "assistant",
                    "content": msg["content"]
                })
            elif role == "user":
                messages.append({
                    "role": "user",
                    "content": msg["content"]
                })
            else:
                logger.warning(f"Unsupported role: {role}, treating as user")
                messages.append({
                    "role": "user",
                    "content": msg["content"]
                })
            
        # Prepare request data
        data = {
            "model": params["model"],
            "messages": messages,
            "temperature": params.get("temperature", 0.7),
            "max_tokens": params.get("max_tokens", 4096),
            "stream": params.get("stream", False)
        }
        
        # Add system message if present
        if system:
            data["system"] = system
        
        # Remove None values
        data = {k: v for k, v in data.items() if v is not None}
        
        logger.debug("Sending request to Anthropic API with data: %s", data)
        
        try:
            # Create HTTP client with authentication
            async with httpx.AsyncClient(
                base_url=self.base_url,
                headers=self.get_headers(),
                timeout=30.0
            ) as client:
                # Make request
                response = await client.post("/v1/messages", json=data)
                response.raise_for_status()
                
                # Parse response
                result = response.json()
                logger.debug("Received response from Anthropic API: %s", result)
                
                # Convert to unified format
                message_content = result.get("content", [{"text": ""}])[0].get("text", "")
                
                return {
                    "id": result.get("id", ""),
                    "object": "chat.completion",
                    "created": result.get("created", ""),
                    "model": result.get("model", params["model"]),
                    "choices": [{
                        "index": 0,
                        "message": {
                            "role": "assistant",
                            "content": message_content
                        },
                        "finish_reason": result.get("stop_reason", "stop")
                    }],
                    "usage": result.get("usage", {})
                }
                
        except httpx.HTTPError as e:
            logger.error("HTTP error occurred: %s", str(e))
            if hasattr(e, 'response'):
                logger.error("Response status code: %s", e.response.status_code)
                logger.error("Response text: %s", e.response.text)
            raise ValueError(f"Error calling Anthropic API: {str(e)}")
        except Exception as e:
            logger.error("Unexpected error: %s", str(e))
            raise ValueError(f"Unexpected error: {str(e)}")
