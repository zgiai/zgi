"""DeepSeek provider implementation"""
from typing import Dict, Any, AsyncGenerator
import httpx
import json
import logging
import os
from .base import BaseProvider

logger = logging.getLogger(__name__)
logger.setLevel(logging.DEBUG)

class DeepSeekProvider(BaseProvider):
    """DeepSeek API provider implementation"""
    
    def __init__(self, api_key: str, base_url: str = None):
        if base_url and not base_url.startswith(("http://", "https://")):
            base_url = f"https://{base_url}"
        super().__init__(api_key, base_url)
        logger.debug(f"Initializing DeepSeek provider with base_url: {base_url}")
        
        # If API key doesn't start with "Bearer ", add it
        if not api_key.startswith("Bearer "):
            api_key = f"Bearer {api_key}"
            
        self.headers = {
            "Authorization": api_key,
            "Content-Type": "application/json",
            "Accept": "application/json"
        }
        # Log headers but mask the API key
        safe_headers = self.headers.copy()
        if "Authorization" in safe_headers:
            safe_headers["Authorization"] = "Bearer ***"
        logger.debug(f"Headers initialized: {safe_headers}")
        
        # Double check environment variable
        env_key = os.environ.get("DEEPSEEK_API_KEY")
        if env_key:
            logger.debug("DEEPSEEK_API_KEY is set in environment")
        else:
            logger.warning("DEEPSEEK_API_KEY is not set in environment")

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
        """
        logger.debug(f"Creating chat completion with params (excluding API key): {params}")
        
        # Copy request parameters and format them for DeepSeek API
        request_params = {
            "model": "deepseek-chat",  # DeepSeek API 需要固定的模型名称
            "messages": params["messages"],
            "stream": params.get("stream", False)
        }
        
        # Add optional parameters
        if "temperature" in params:
            request_params["temperature"] = float(params["temperature"])
        if "max_tokens" in params:
            request_params["max_length"] = int(params["max_tokens"])  # DeepSeek 使用 max_length
            
        logger.debug(f"Prepared request params: {request_params}")
        
        stream = request_params.get("stream", False)
        
        if stream:
            logger.debug("Handling streaming request")
            return self._handle_streaming_request(request_params)
        else:
            logger.debug("Handling non-streaming request")
            return await self._handle_request(request_params)
            
    async def _handle_request(self, request_params: Dict[str, Any]) -> Dict[str, Any]:
        """Handle a non-streaming request."""
        endpoint = f"{self.base_url}/chat/completions"
        logger.debug(f"Making request to endpoint: {endpoint}")
        
        # Log headers but mask the API key
        safe_headers = self.headers.copy()
        if "Authorization" in safe_headers:
            safe_headers["Authorization"] = "Bearer ***"
        logger.debug(f"Using headers: {safe_headers}")
        
        # Log full request for debugging
        logger.debug(f"Full request params: {json.dumps(request_params, ensure_ascii=False)}")
        
        async with httpx.AsyncClient() as client:
            try:
                response = await client.post(
                    endpoint,
                    headers=self.headers,
                    json=request_params,
                    timeout=30.0
                )
                
                # Log response details for debugging
                logger.debug(f"Response status: {response.status_code}")
                logger.debug(f"Response headers: {dict(response.headers)}")
                if response.status_code != 200:
                    logger.error(f"Response text: {response.text}")
                
                response.raise_for_status()
                result = response.json()
                logger.debug(f"Got response with status {response.status_code}")
                
                # Convert to unified format
                return {
                    "id": result.get("id", ""),
                    "model": result.get("model", "deepseek-chat"),
                    "choices": [{
                        "message": {
                            "role": "assistant",
                            "content": result["choices"][0]["message"]["content"]
                        },
                        "finish_reason": result["choices"][0].get("finish_reason", "stop")
                    }],
                    "usage": result.get("usage", {})
                }
            except httpx.HTTPStatusError as e:
                logger.error(f"HTTP error {e.response.status_code}: {e.response.text}")
                if e.response.status_code == 401:
                    logger.error("Authentication failed. Please check your API key.")
                    # Log the actual headers being sent (but mask the API key)
                    safe_headers = dict(e.request.headers)
                    if "Authorization" in safe_headers:
                        safe_headers["Authorization"] = "Bearer ***"
                    logger.error(f"Request headers: {safe_headers}")
                elif e.response.status_code == 400:
                    logger.error("Bad request. Check the request parameters.")
                    logger.error(f"Request params: {json.dumps(request_params, ensure_ascii=False)}")
                elif e.response.status_code == 422:
                    logger.error("Unprocessable Entity. The request format is incorrect.")
                    logger.error(f"Request params: {json.dumps(request_params, ensure_ascii=False)}")
                raise
            except Exception as e:
                logger.error(f"Unexpected error: {str(e)}")
                raise
                
    async def _handle_streaming_request(self, request_params: Dict[str, Any]) -> AsyncGenerator[Dict[str, Any], None]:
        """Handle a streaming request."""
        endpoint = f"{self.base_url}/chat/completions"
        logger.debug(f"Making streaming request to endpoint: {endpoint}")
        
        # Log headers but mask the API key
        safe_headers = self.headers.copy()
        if "Authorization" in safe_headers:
            safe_headers["Authorization"] = "Bearer ***"
        logger.debug(f"Using headers: {safe_headers}")
        
        # Log full request for debugging
        logger.debug(f"Full request params: {json.dumps(request_params, ensure_ascii=False)}")
        
        async with httpx.AsyncClient() as client:
            try:
                async with client.stream(
                    "POST",
                    endpoint,
                    headers=self.headers,
                    json=request_params,
                    timeout=30.0
                ) as response:
                    response.raise_for_status()
                    logger.debug(f"Stream connected with status {response.status_code}")
                    
                    # Stream the response
                    async for line in response.aiter_lines():
                        if not line.strip():
                            continue
                            
                        if line.startswith("data: "):
                            line = line[6:]  # Remove "data: " prefix
                            
                        if line == "[DONE]":
                            logger.debug("Stream completed")
                            break
                            
                        try:
                            chunk = json.loads(line)
                            # Convert to unified format
                            yield {
                                "id": chunk.get("id", ""),
                                "model": chunk.get("model", "deepseek-chat"),
                                "choices": [{
                                    "delta": {
                                        "role": chunk["choices"][0]["delta"].get("role", "assistant"),
                                        "content": chunk["choices"][0]["delta"].get("content", "")
                                    },
                                    "finish_reason": chunk["choices"][0].get("finish_reason")
                                }],
                                "usage": chunk.get("usage", {})
                            }
                        except json.JSONDecodeError as e:
                            logger.error(f"Failed to decode JSON: {e}")
                            logger.error(f"Raw line: {line}")
                            continue
            except httpx.HTTPStatusError as e:
                logger.error(f"HTTP error {e.response.status_code}: {e.response.text}")
                if e.response.status_code == 401:
                    logger.error("Authentication failed. Please check your API key.")
                elif e.response.status_code == 400:
                    logger.error("Bad request. Check the request parameters.")
                    logger.error(f"Request params: {json.dumps(request_params, ensure_ascii=False)}")
                elif e.response.status_code == 422:
                    logger.error("Unprocessable Entity. The request format is incorrect.")
                    logger.error(f"Request params: {json.dumps(request_params, ensure_ascii=False)}")
                raise
            except Exception as e:
                logger.error(f"Unexpected error: {str(e)}")
                raise

    async def handle_request(self, params: Dict[str, Any]) -> Dict[str, Any] | AsyncGenerator[Dict[str, Any], None]:
        """Handle a chat completion request"""
        logger.debug(f"DeepSeek request params: {params}")
        logger.debug(f"DeepSeek headers: {self.headers}")
        logger.debug(f"DeepSeek base URL: {self.base_url}")
        
        # Copy request parameters and keep the original model name
        request_params = {
            "model": "deepseek-chat",  # DeepSeek API 需要固定的模型名称
            "messages": params["messages"],
            "stream": params.get("stream", False)
        }
        
        # Add optional parameters
        if "temperature" in params:
            request_params["temperature"] = float(params["temperature"])
        if "max_tokens" in params:
            request_params["max_length"] = int(params["max_tokens"])  # DeepSeek 使用 max_length
            
        logger.debug(f"Request params: {request_params}")
        
        stream = request_params.get("stream", False)
        
        if stream:
            logger.debug("Handling streaming request")
            return self._handle_streaming_request(request_params)
        else:
            logger.debug("Handling non-streaming request")
            async with httpx.AsyncClient() as client:
                response = await client.post(
                    f"{self.base_url}/chat/completions",
                    headers=self.headers,
                    json=request_params,
                    timeout=60.0
                )
                if response.status_code == 400:
                    logger.error(f"Request failed with 400: {response.text}")
                    raise ValueError(f"DeepSeek API request failed: {response.text}")
                response.raise_for_status()
                logger.debug(f"Received response: {response.json()}")
                return response.json()
            
    async def validate_api_key(self) -> bool:
        """Validate the DeepSeek API key"""
        try:
            async with httpx.AsyncClient() as client:
                response = await client.get(
                    f"{self.base_url}/models",
                    headers=self.headers,
                    timeout=10.0
                )
                return response.status_code == 200
        except Exception:
            return False

    def get_model_info(self) -> Dict[str, Any]:
        """Get information about DeepSeek models"""
        return {
            "deepseek-chat": {
                "name": "DeepSeek Chat",
                "max_tokens": 4096,
                "supports_streaming": True,
                "supports_functions": False
            },
            "deepseek-coder": {
                "name": "DeepSeek Coder",
                "max_tokens": 8192,
                "supports_streaming": True,
                "supports_functions": False
            }
        }
