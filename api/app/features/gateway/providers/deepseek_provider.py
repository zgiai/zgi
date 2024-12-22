from typing import Dict, Any, AsyncGenerator
import httpx
import json
import logging
from .base import BaseProvider

class DeepSeekProvider(BaseProvider):
    """DeepSeek API provider implementation"""
    
    def __init__(self, api_key: str, base_url: str = "https://api.deepseek.com/v1"):
        super().__init__(api_key, base_url)
        logging.debug(f"Initializing DeepSeek provider with API key: {api_key}")
        self.headers = {
            "Authorization": f"Bearer {api_key}",  # DeepSeek expects API key in Bearer format
            "Content-Type": "application/json",
            "Accept": "application/json"
        }

    async def handle_request(self, params: Dict[str, Any]) -> Dict[str, Any] | AsyncGenerator[Dict[str, Any], None]:
        """Handle a chat completion request"""
        logging.debug(f"DeepSeek request params: {params}")
        logging.debug(f"DeepSeek headers: {self.headers}")
        logging.debug(f"DeepSeek base URL: {self.base_url}")
        
        # Remove provider-specific parameters and map model names
        request_params = {
            "model": "deepseek-chat",  # DeepSeek 的模型名称就是 deepseek-chat
            "messages": params["messages"],
            "stream": params.get("stream", False)
        }
        
        # Add optional parameters
        if "temperature" in params:
            request_params["temperature"] = float(params["temperature"])
        if "max_tokens" in params:
            request_params["max_tokens"] = int(params["max_tokens"])
            
        logging.debug(f"Request params: {request_params}")
        
        stream = request_params.get("stream", False)
        
        if stream:
            logging.debug("Handling streaming request")
            return self._handle_streaming_request(request_params)
        else:
            logging.debug("Handling non-streaming request")
            async with httpx.AsyncClient() as client:
                response = await client.post(
                    f"{self.base_url}/chat/completions",
                    headers=self.headers,
                    json=request_params,
                    timeout=60.0
                )
                if response.status_code == 400:
                    logging.error(f"Request failed with 400: {response.text}")
                    raise ValueError(f"DeepSeek API request failed: {response.text}")
                response.raise_for_status()
                logging.debug(f"Received response: {response.json()}")
                return response.json()
            
    async def _handle_streaming_request(self, params: Dict[str, Any]) -> AsyncGenerator[Dict[str, Any], None]:
        """Handle a streaming chat completion request"""
        async with httpx.AsyncClient() as client:
            async with client.stream(
                "POST",
                f"{self.base_url}/chat/completions",
                headers=self.headers,
                json=params,
                timeout=60.0
            ) as response:
                response.raise_for_status()
                async for line in response.aiter_lines():
                    if line.startswith("data: "):
                        line = line.removeprefix("data: ")
                        if line.strip() == "[DONE]":
                            break
                        try:
                            yield json.loads(line)
                        except json.JSONDecodeError:
                            continue

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
