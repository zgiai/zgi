"""DeepSeek provider implementation."""
from typing import Dict, Any, AsyncGenerator, Optional
import json
import logging
import httpx

from .base import LLMProvider
from ..utils.message_converter import extract_system_message, filter_messages_by_role
from ..utils.response_formatter import create_chat_response, create_streaming_chunk, extract_usage_stats
from ..utils.http_client import create_http_client, stream_response, make_json_request

logger = logging.getLogger(__name__)

class DeepSeekProvider(LLMProvider):
    """DeepSeek provider implementation."""
    
    def __init__(self, provider_name: str = "deepseek"):
        """Initialize DeepSeek provider."""
        super().__init__(provider_name)
        self.base_url = "https://api.deepseek.com"
        self.headers = {
            "content-type": "application/json",
            "authorization": f"Bearer {self.api_key}"
        }
        
    async def chat_completion(
        self,
        messages: list[Dict[str, Any]],
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
            temperature: Sampling temperature
            max_tokens: Maximum tokens to generate
            stream: Whether to stream the response
            **kwargs: Additional arguments
            
        Yields:
            Response chunks
        """
        # Filter messages by role
        messages = filter_messages_by_role(messages, ["system", "user", "assistant"])
        
        # Prepare request data
        data = {
            "model": model,
            "messages": messages,
            "temperature": temperature,
            "stream": stream
        }
        
        if max_tokens:
            data["max_tokens"] = max_tokens
            
        # Add any additional parameters
        data.update(kwargs)
        
        try:
            if stream:
                async for chunk in self._stream_chat_completion(data):
                    yield chunk
            else:
                result = await self._regular_chat_completion(data)
                yield result
                
        except httpx.HTTPError as e:
            logger.error(f"DeepSeek API request failed: {str(e)}")
            raise
            
    async def _regular_chat_completion(self, data: Dict[str, Any]) -> Dict[str, Any]:
        """Make a regular chat completion request.
        
        Args:
            data: Request data
            
        Returns:
            Formatted response
        """
        async with create_http_client(self.base_url, self.headers) as client:
            result = await make_json_request(
                client,
                "POST",
                "/v1/chat/completions",
                json=data
            )
            
            return create_chat_response(
                id=result["id"],
                model=result["model"],
                content=result["choices"][0]["message"]["content"],
                usage=extract_usage_stats(
                    result,
                    input_key="prompt_tokens",
                    output_key="completion_tokens",
                    total_key="total_tokens"
                ),
                created=str(result.get("created", ""))
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
        async with create_http_client(self.base_url, self.headers) as client:
            async for line in stream_response(
                client,
                "POST",
                "/v1/chat/completions",
                json=data
            ):
                if not line.strip():
                    continue
                    
                try:
                    chunk = json.loads(line.replace("data: ", ""))
                except json.JSONDecodeError:
                    logger.warning(f"Failed to parse chunk: {line}")
                    continue
                    
                if chunk.get("object") == "chat.completion.chunk":
                    delta = chunk["choices"][0]["delta"]
                    if "role" in delta:
                        yield create_streaming_chunk(
                            id=chunk["id"],
                            model=data["model"],
                            role=delta["role"]
                        )
                    elif "content" in delta:
                        yield create_streaming_chunk(
                            id=chunk["id"],
                            model=data["model"],
                            content=delta["content"]
                        )
                    elif chunk["choices"][0].get("finish_reason"):
                        yield create_streaming_chunk(
                            id=chunk["id"],
                            model=data["model"],
                            finish_reason=chunk["choices"][0]["finish_reason"]
                        )
