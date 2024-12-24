"""Anthropic API provider implementation."""
from typing import Dict, Any, AsyncGenerator, Optional, List
import json
import logging
import httpx

from .base import LLMProvider
from ..utils.message_converter import convert_messages_to_provider_format, extract_system_message, filter_messages_by_role
from ..utils.response_formatter import create_chat_response, create_streaming_chunk, extract_usage_stats
from ..utils.http_client import create_http_client, stream_response, make_json_request
from ..utils.sse_parser import SSEBuffer

logger = logging.getLogger(__name__)

class AnthropicProvider(LLMProvider):
    """Anthropic API provider implementation."""
    
    MODEL_MAPPING = {
        "claude-3-opus-20240229": "claude-3-opus-20240229"
    }
    
    SUPPORTED_PREFIXES = ["claude-"]
    
    DEFAULT_MAX_TOKENS = 4096
    BASE_URL = "https://api.anthropic.com"
    API_VERSION = "2023-06-01"
    
    def __init__(self, provider_name: str = "anthropic", api_key: str = None, base_url: str = None):
        """Initialize Anthropic provider."""
        super().__init__(provider_name, api_key, base_url)
        self.base_url = base_url or self.BASE_URL
        self.api_version = self.API_VERSION
        self.headers = {
            "anthropic-version": self.api_version,
            "content-type": "application/json",
            "x-api-key": self.api_key
        }
        
        # Log configuration
        logger.debug(f"Initialized Anthropic provider with API version: {self.api_version}")
        logger.debug(f"Base URL: {self.base_url}")
        logger.debug(f"Headers: {json.dumps(self.headers, indent=2)}")
        
    def _create_content_block(self, text: str) -> List[Dict[str, str]]:
        """Create a content block for messages.
        
        Args:
            text: The text content
            
        Returns:
            A list containing the content block
        """
        return [
            {
                "type": "text",
                "text": text
            }
        ]
        
    def _create_message(self, role: str, content: str) -> Dict[str, Any]:
        """Create a message object.
        
        Args:
            role: Message role (user/assistant)
            content: Message content
            
        Returns:
            Message object
        """
        return {
            "role": role,
            "content": self._create_content_block(content)
        }
        
    def _create_system_message(self, system: str) -> Dict[str, List[Dict[str, str]]]:
        """Create a system message object.
        
        Args:
            system: System message content
            
        Returns:
            System message object
        """
        return {
            "content": self._create_content_block(system)
        }
        
    def _prepare_request_data(
        self,
        messages: List[Dict[str, str]],
        model: str,
        temperature: float,
        stream: bool,
        max_tokens: Optional[int] = None,
        system: Optional[str] = None,
        **kwargs: Any
    ) -> Dict[str, Any]:
        """Prepare request data for the API call.
        
        Args:
            messages: List of messages
            model: Model name
            temperature: Temperature value
            stream: Whether to stream responses
            max_tokens: Maximum tokens to generate
            system: System message
            **kwargs: Additional arguments
            
        Returns:
            Prepared request data
        """
        # Extract system message and filter messages
        messages, system = extract_system_message(messages)
        messages = filter_messages_by_role(messages, ["user", "assistant"])
        
        # Convert model name if needed
        model = self.MODEL_MAPPING.get(model, model)
        
        # Prepare request data
        data = {
            "model": model,
            "messages": [
                self._create_message(msg["role"], msg["content"])
                for msg in messages
            ],
            "temperature": temperature,
            "stream": stream,
            "max_tokens": max_tokens if max_tokens is not None else self.DEFAULT_MAX_TOKENS
        }
        
        if system:
            data["system"] = self._create_system_message(system)
            
        return data
        
    def _update_base_url(self, base_url: Optional[str] = None) -> None:
        """Update base URL if provided in request.
        
        Args:
            base_url: Optional base URL override
        """
        if base_url:
            logger.debug(f"Updating base URL to: {base_url}")
            self.base_url = base_url.rstrip("/")
        
    async def chat_completion(
        self,
        messages: List[Dict[str, Any]],
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
            temperature: Temperature value
            max_tokens: Maximum tokens to generate
            stream: Whether to stream responses
            **kwargs: Additional arguments
            
        Yields:
            Response chunks
            
        Raises:
            Exception: If the request fails
        """
        # Update base URL if provided
        self._update_base_url(kwargs.get("base_url"))
        
        # Prepare request data
        data = self._prepare_request_data(
            messages,
            model,
            temperature,
            stream,
            max_tokens,
            **kwargs
        )
        
        # Log request data for debugging
        logger.debug(f"Request data: {json.dumps(data, indent=2)}")
        logger.debug(f"Request headers: {json.dumps(self.headers, indent=2)}")
        logger.debug(f"Making request to: {self.base_url}/v1/messages")
        
        try:
            if stream:
                async for chunk in self._stream_chat_completion(data):
                    yield chunk
            else:
                result = await self._regular_chat_completion(data)
                yield result
                
        except httpx.HTTPError as e:
            logger.error(f"Anthropic API request failed: {str(e)}")
            logger.error(f"Request data: {json.dumps(data, indent=2)}")
            logger.error(f"Request headers: {json.dumps(self.headers, indent=2)}")
            raise
            
    async def _regular_chat_completion(self, data: Dict[str, Any]) -> Dict[str, Any]:
        """Make a regular chat completion request.
        
        Args:
            data: Request data
            
        Returns:
            Formatted response
        """
        logger.debug(f"Making regular chat completion request with data: {json.dumps(data, indent=2)}")
        async with create_http_client(self.base_url, self.headers) as client:
            result = await make_json_request(
                client,
                "POST",
                "/v1/messages",
                json=data
            )
            logger.debug(f"Got response: {json.dumps(result, indent=2)}")
            return create_chat_response(
                id=result["id"],
                model=result["model"],
                content=result["content"][0]["text"],
                usage=extract_usage_stats(
                    result,
                    input_key="usage.input_tokens",
                    output_key="usage.output_tokens",
                    total_key="usage.total_tokens"
                ),
                created=result.get("created_at", "")
            )
            
    async def _stream_chat_completion(
        self,
        data: Dict[str, Any]
    ) -> AsyncGenerator[Dict[str, Any], None]:
        """Stream chat completion response.
        
        Args:
            data: Request data
            
        Yields:
            Response chunks
        """
        logger.debug(f"Making streaming chat completion request with data: {json.dumps(data, indent=2)}")
        message_id = ""
        buffer = SSEBuffer()
        
        async with create_http_client(self.base_url, self.headers) as client:
            async for line in stream_response(
                client,
                "POST",
                "/v1/messages",
                json=data
            ):
                logger.debug(f"Got stream line: {line}")
                event = buffer.add_line(line)
                if not event:
                    continue
                    
                event_type, event_data = event
                if not event_data:
                    continue
                    
                # Handle different event types
                if event_type == "message_start":
                    message_id = event_data.get("message", {}).get("id", "")
                    yield create_streaming_chunk(
                        id=message_id,
                        model=data["model"],
                        role="assistant"
                    )
                elif event_type == "content_block_delta":
                    delta_text = event_data.get("delta", {}).get("text", "")
                    if delta_text:
                        yield create_streaming_chunk(
                            id=message_id,
                            model=data["model"],
                            content=delta_text
                        )
                elif event_type == "message_stop":
                    yield create_streaming_chunk(
                        id=message_id,
                        model=data["model"],
                        finish_reason="stop"
                    )
                    
            # Flush any remaining data
            event = buffer.flush()
            if event:
                event_type, event_data = event
                if event_data and event_type == "content_block_delta":
                    delta_text = event_data.get("delta", {}).get("text", "")
                    if delta_text:
                        yield create_streaming_chunk(
                            id=message_id,
                            model=data["model"],
                            content=delta_text
                        )
