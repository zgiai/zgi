"""Anthropic provider implementation"""
import json
from typing import Dict, Any, AsyncGenerator, Optional, List
import httpx
import logging

# Configure logging
logger = logging.getLogger(__name__)
logger.setLevel(logging.DEBUG)

class AnthropicProvider:
    """Provider implementation for Anthropic's Claude models"""
    
    def __init__(self, api_key: str, base_url: Optional[str] = None):
        """Initialize the Anthropic provider.
        
        Args:
            api_key: Anthropic API key
            base_url: Optional base URL for the API
        """
        self.api_key = api_key
        if base_url and not base_url.startswith(("http://", "https://")):
            base_url = f"https://{base_url}"
        self.base_url = base_url
        logger.debug(f"Initializing Anthropic provider with base URL: {self.base_url}")
        
        self.headers = {
            "x-api-key": self.api_key,
            "anthropic-version": "2023-06-01",
            "content-type": "application/json",
        }
        
        self.client = httpx.AsyncClient(
            base_url=self.base_url,
            headers=self.headers
        )
        logger.debug("Client initialized with headers: %s", self.client.headers)
        
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
            Chat completion response in unified format
        """
        logger.debug(f"Creating chat completion with params: {params}")
        
        messages = params["messages"]
        model = params["model"]
        temperature = params.get("temperature", 0.7)
        max_tokens = params.get("max_tokens", 4096)
        stream = params.get("stream", False)
        
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
        
        try:
            if stream:
                return self._stream_response(anthropic_data)
            else:
                return await self._send_request(anthropic_data)
        except Exception as e:
            logger.error("Error in Anthropic request: %s", str(e))
            if isinstance(e, httpx.HTTPError):
                logger.error("HTTP Error response: %s", e.response.text if hasattr(e, 'response') else 'No response')
            raise
            
    async def _send_request(self, data: Dict[str, Any]) -> Dict[str, Any]:
        """Send a request to Anthropic's API.
        
        Args:
            data: Request data
            
        Returns:
            Response from Anthropic in unified format
        """
        logger.debug(f"Making request to {self.base_url}/v1/messages")
        async with self.client as client:
            try:
                response = await client.post("/v1/messages", json=data)
                response.raise_for_status()
                logger.debug(f"Got response: {response.status_code}")
                result = response.json()
                logger.debug("Received response: %s", json.dumps(result, indent=2))
                
                # Convert to unified format
                formatted_response = {
                    "id": result.get("id", ""),
                    "model": result.get("model", data["model"]),
                    "choices": [{
                        "message": {
                            "role": "assistant",
                            "content": result["content"][0]["text"]
                        },
                        "finish_reason": result.get("stop_reason", "stop")
                    }],
                    "usage": result.get("usage", {})
                }
                logger.debug("Formatted response: %s", json.dumps(formatted_response, indent=2))
                return formatted_response
            except httpx.HTTPError as e:
                logger.error(f"HTTP error: {e}")
                logger.error("Response status code: %s", e.response.status_code if hasattr(e, 'response') else 'Unknown')
                logger.error("Response text: %s", e.response.text if hasattr(e, 'response') else 'No response')
                raise
            except Exception as e:
                logger.error(f"Error making request: {e}")
                raise
            
    async def _stream_response(self, data: Dict[str, Any]) -> AsyncGenerator[Dict[str, Any], None]:
        """Stream a response from Anthropic's API.
        
        Args:
            data: Request data
            
        Yields:
            Response chunks in unified format
        """
        # Add Accept header for streaming
        headers = {**self.headers, "Accept": "text/event-stream"}
        
        async with httpx.AsyncClient(base_url=self.base_url, headers=headers) as client:
            try:
                logger.debug("Starting streaming request to /v1/messages")
                async with client.stream("POST", "/v1/messages", json=data) as response:
                    response.raise_for_status()
                    logger.debug("Stream connection established")
                    
                    async for line in response.aiter_lines():
                        if not line.strip():
                            continue
                            
                        logger.debug("Received line: %s", line)
                        
                        if line.startswith("event: "):
                            event_type = line[7:].strip()
                            logger.debug("Event type: %s", event_type)
                            continue
                            
                        if line.startswith("data: "):
                            line = line[6:]  # Remove "data: " prefix
                            logger.debug("Processed data line: %s", line)
                            
                            if line == "[DONE]":
                                logger.debug("Received DONE signal")
                                return
                            
                        try:
                            result = json.loads(line)
                            logger.debug("Parsed JSON result: %s", json.dumps(result, indent=2))
                            
                            # Handle different event types
                            if result.get("type") == "message_start":
                                # Skip the initial message
                                continue
                            elif result.get("type") == "content_block_start":
                                # Skip content block start
                                continue
                            elif result.get("type") == "content_block_delta":
                                # Extract the delta text
                                delta_text = result.get("delta", {}).get("text", "")
                                formatted_chunk = {
                                    "id": result.get("message_id", ""),
                                    "model": data["model"],
                                    "choices": [{
                                        "delta": {
                                            "role": "assistant",
                                            "content": delta_text
                                        },
                                        "finish_reason": None
                                    }],
                                    "usage": {}
                                }
                            elif result.get("type") == "content_block_stop":
                                # Send a final chunk with finish_reason
                                formatted_chunk = {
                                    "id": result.get("message_id", ""),
                                    "model": data["model"],
                                    "choices": [{
                                        "delta": {
                                            "role": "assistant",
                                            "content": ""
                                        },
                                        "finish_reason": "stop"
                                    }],
                                    "usage": result.get("message", {}).get("usage", {})
                                }
                            else:
                                # Skip other event types
                                continue
                                
                            logger.debug("Yielding formatted chunk: %s", json.dumps(formatted_chunk, indent=2))
                            yield formatted_chunk
                            
                        except json.JSONDecodeError as e:
                            logger.error("JSON decode error: %s", str(e))
                            continue
            except httpx.HTTPError as e:
                logger.error(f"HTTP error: {e}")
                logger.error("Response status code: %s", e.response.status_code if hasattr(e, 'response') else 'Unknown')
                logger.error("Response text: %s", e.response.text if hasattr(e, 'response') else 'No response')
                raise
            except Exception as e:
                logger.error(f"Error making request: {e}")
                raise
