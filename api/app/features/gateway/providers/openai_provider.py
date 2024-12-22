from typing import Dict, Any
import httpx
from .base import BaseProvider

class OpenAIProvider(BaseProvider):
    """OpenAI API provider implementation"""
    
    def __init__(self, api_key: str, base_url: str = "https://api.openai.com/v1"):
        super().__init__(api_key, base_url)
        self.headers = {
            "Authorization": f"Bearer {api_key}",
            "Content-Type": "application/json"
        }

    async def handle_request(self, params: Dict[str, Any]) -> Dict[str, Any]:
        """Handle a chat completion request"""
        async with httpx.AsyncClient() as client:
            response = await client.post(
                f"{self.base_url}/chat/completions",
                headers=self.headers,
                json=params,
                timeout=60.0
            )
            response.raise_for_status()
            return response.json()

    async def validate_api_key(self) -> bool:
        """Validate the OpenAI API key"""
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
        """Get information about OpenAI models"""
        return {
            "gpt-4": {
                "name": "GPT-4",
                "max_tokens": 8192,
                "supports_streaming": True,
                "supports_functions": True
            },
            "gpt-3.5-turbo": {
                "name": "GPT-3.5 Turbo",
                "max_tokens": 4096,
                "supports_streaming": True,
                "supports_functions": True
            }
        }
