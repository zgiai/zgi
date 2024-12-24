"""Anthropic provider implementation"""
import json
from typing import Dict, Any, AsyncGenerator, Optional, List, Union
import httpx
import logging
from .base import BaseProvider
from ..config.models import ModelConfig
from ..exceptions.provider_errors import InvalidAPIKeyError

logger = logging.getLogger(__name__)

class AnthropicProvider(BaseProvider):
    """Provider implementation for Anthropic's Claude models"""
    
    API_VERSION = "2023-06-01"  # Stable version supported by Anthropic
    
    def __init__(self, api_key: str, base_url: Optional[str] = None):
        """Initialize the Anthropic provider.
        
        Args:
            api_key: Anthropic API key
            base_url: Optional base URL for the API
        """
        # Validate API key before calling super().__init__
        if not api_key or api_key == "undefined":
            raise InvalidAPIKeyError(
                "No valid Anthropic API key provided. Please set the ANTHROPIC_API_KEY environment variable "
                "or provide the key in the request."
            )
            
        super().__init__(api_key, base_url)
        
        if base_url and not base_url.startswith(("http://", "https://")):
            base_url = f"https://{base_url}"
            
        # Remove /v1 from base URL if present
        if base_url and base_url.endswith("/v1"):
            base_url = base_url[:-3]
            
        self.base_url = base_url or ModelConfig.get_default_base_url("anthropic")
        logger.debug(f"Initializing Anthropic provider with base URL: {self.base_url}")
        
        self.headers = {
            "x-api-key": self.api_key,
            "anthropic-version": self.API_VERSION,
            "content-type": "application/json",
        }
        
    @property
    def provider_name(self) -> str:
        return "anthropic"
        
    def validate_api_key(self, api_key: str) -> str:
        """Validate Anthropic API key.
        
        Args:
            api_key: Raw API key
            
        Returns:
            Validated API key
            
        Raises:
            InvalidAPIKeyError: If API key is invalid
        """
        if not api_key or not isinstance(api_key, str):
            raise InvalidAPIKeyError("API key must be a non-empty string")
        return api_key
        
    def default_base_url(self) -> str:
        """Get default base URL.
        
        Returns:
            Default base URL
        """
        return ModelConfig.get_default_base_url(self.provider_name)
        
    def get_headers(self) -> Dict[str, str]:
        """Get HTTP headers.
        
        Returns:
            HTTP headers
        """
        return {
            "x-api-key": self.api_key,
            "anthropic-version": self.API_VERSION,
            "content-type": "application/json",
        }
        
    async def create_chat_completion(
        self,
        params: Dict[str, Any]
    ) -> Union[Dict[str, Any], AsyncGenerator[Dict[str, Any], None]]:
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
            Chat completion response in unified format
            
        Raises:
            ValueError: If the request parameters are invalid
            httpx.HTTPError: If the API request fails
        """
        logger.debug(f"Creating chat completion with params: {params}")
        
        messages = params["messages"]
        model = params["model"]
        temperature = params.get("temperature", 0.7)
        max_tokens = params.get("max_tokens", 4096)
        stream = params.get("stream", False)
        
        # Validate model
        if model not in ModelConfig.get_supported_models("anthropic"):
            logger.error(f"Unsupported model: {model}")
            logger.error(f"Supported models: {', '.join(sorted(ModelConfig.get_supported_models('anthropic')))}")
            raise ValueError(f"Model {model} is not supported. Supported models: {', '.join(sorted(ModelConfig.get_supported_models('anthropic')))}")
        
        logger.debug("Processing request parameters:")
        logger.debug("Model: %s", model)
        logger.debug("Temperature: %s", temperature)
        logger.debug("Max tokens: %s", max_tokens)
        logger.debug("Stream: %s", stream)
        logger.debug("Messages: %s", json.dumps(messages, indent=2))
        
        # Convert messages to Anthropic format
        system = None
        messages_formatted = []
        
        for msg in messages:
            role = msg["role"]
            content = msg["content"]
            
            if role == "system":
                system = content
            elif role == "user":
                messages_formatted.append({"role": "user", "content": content})
            elif role == "assistant":
                messages_formatted.append({"role": "assistant", "content": content})
            else:
                logger.warning(f"Unsupported role: {role}, skipping message")
        
        logger.debug("Formatted messages: %s", json.dumps(messages_formatted, indent=2))
        if system:
            logger.debug("System message: %s", system)
        
        # Prepare request data
        anthropic_data = {
            "model": model,
            "messages": messages_formatted,
            "max_tokens": max_tokens,
            "temperature": temperature,
            "stream": stream
        }
        
        if system:
            anthropic_data["system"] = system
            
        logger.debug("Sending request to Anthropic with data: %s", json.dumps(anthropic_data, indent=2))
        logger.debug("Using headers: %s", json.dumps({k: '***' if k == 'x-api-key' else v for k, v in self.headers.items()}, indent=2))
        
        try:
            if stream:
                return self._stream_response(anthropic_data)
            else:
                result = await self._send_request(anthropic_data)
                return result
        except Exception as e:
            logger.error("Error in Anthropic request: %s", str(e))
            if isinstance(e, httpx.HTTPError) and hasattr(e, 'response'):
                logger.error("HTTP Error response: %s", e.response.text)
                logger.error("Request URL: %s", e.response.request.url)
                logger.error("Request method: %s", e.response.request.method)
                logger.error("Request headers: %s", json.dumps({k: '***' if k == 'x-api-key' else v for k, v in dict(e.response.request.headers).items()}, indent=2))
                logger.error("Request body: %s", e.response.request.content.decode())
            raise
            
    async def _send_request(self, data: Dict[str, Any]) -> Dict[str, Any]:
        """Send a request to Anthropic's API.
        
        Args:
            data: Request data
            
        Returns:
            Response from Anthropic in unified format
        """
        logger.debug(f"Making request to {self.base_url}/v1/messages")
        
        # Create a new client for each request
        async with httpx.AsyncClient(
            base_url=self.base_url,
            headers=self.headers,
            timeout=30.0
        ) as client:
            try:
                response = await client.post("/v1/messages", json=data)
                logger.debug(f"Response status: {response.status_code}")
                logger.debug(f"Response headers: {json.dumps(dict(response.headers), indent=2)}")
                logger.debug(f"Response body: {response.text}")
                
                response.raise_for_status()
                result = response.json()
                
                # Convert to unified format (OpenAI-like)
                formatted_response = {
                    "id": result.get("id", ""),
                    "object": "chat.completion",
                    "created": result.get("created_at", ""),
                    "model": result.get("model", data["model"]),
                    "choices": [{
                        "index": 0,
                        "message": {
                            "role": "assistant",
                            "content": result["content"][0]["text"]
                        },
                        "finish_reason": result.get("stop_reason", "stop")
                    }],
                    "usage": {
                        "prompt_tokens": result.get("usage", {}).get("input_tokens", 0),
                        "completion_tokens": result.get("usage", {}).get("output_tokens", 0),
                        "total_tokens": result.get("usage", {}).get("total_tokens", 0)
                    }
                }
                
                return formatted_response
                
            except httpx.HTTPError as e:
                logger.error(f"HTTP error occurred: {str(e)}")
                if hasattr(e, 'response'):
                    logger.error(f"Response: {e.response.text}")
                raise
                
    async def _stream_response(self, data: Dict[str, Any]) -> AsyncGenerator[Dict[str, Any], None]:
        """Stream response from Anthropic's API.
        
        Args:
            data: Request data
            
        Yields:
            Response chunks in OpenAI-compatible format
        """
        logger.debug(f"Making streaming request to {self.base_url}/v1/messages")
        
        # Create a new client for streaming
        async with httpx.AsyncClient(
            base_url=self.base_url,
            headers=self.headers,
            timeout=30.0
        ) as client:
            try:
                async with client.stream("POST", "/v1/messages", json=data) as response:
                    response.raise_for_status()
                    
                    # Yield the initial chunk with model info
                    yield {
                        "id": "",
                        "object": "chat.completion.chunk",
                        "created": "",
                        "model": data["model"],
                        "choices": [{
                            "index": 0,
                            "delta": {
                                "role": "assistant"
                            },
                            "finish_reason": None
                        }]
                    }
                    
                    buffer = ""
                    current_event = None
                    
                    async for line in response.aiter_lines():
                        line = line.strip()
                        if not line:
                            continue
                            
                        if line.startswith("event: "):
                            current_event = line[7:]
                        elif line.startswith("data: "):
                            data_line = line[6:]
                            try:
                                chunk = json.loads(data_line)
                                if chunk.get("type") == "content_block_delta":
                                    delta_text = chunk.get("delta", {}).get("text", "")
                                    if delta_text:
                                        yield {
                                            "id": chunk.get("message_id", ""),
                                            "object": "chat.completion.chunk",
                                            "created": "",
                                            "model": data["model"],
                                            "choices": [{
                                                "index": 0,
                                                "delta": {
                                                    "content": delta_text
                                                },
                                                "finish_reason": None
                                            }]
                                        }
                                elif chunk.get("type") == "message_stop":
                                    yield {
                                        "id": chunk.get("message_id", ""),
                                        "object": "chat.completion.chunk",
                                        "created": "",
                                        "model": data["model"],
                                        "choices": [{
                                            "index": 0,
                                            "delta": {},
                                            "finish_reason": "stop"
                                        }]
                                    }
                            except json.JSONDecodeError:
                                logger.warning(f"Failed to parse data: {data_line}")
                                continue
                            
            except httpx.HTTPError as e:
                logger.error(f"HTTP error occurred during streaming: {str(e)}")
                if hasattr(e, 'response'):
                    logger.error(f"Response: {e.response.text}")
                raise
