"""HTTP utilities for provider communication."""
from typing import Dict, Any, Optional, AsyncGenerator
import httpx
import logging
import json
from ..exceptions.provider_errors import ProviderError

logger = logging.getLogger(__name__)

class HttpClient:
    """Generic HTTP client for provider communication."""
    
    def __init__(self, base_url: str, headers: Dict[str, str]):
        self.base_url = base_url
        self.headers = headers
        self.client = httpx.AsyncClient(timeout=30.0)
        
    async def post(self, endpoint: str, data: Dict[str, Any], stream: bool = False) -> Dict[str, Any] | AsyncGenerator[Dict[str, Any], None]:
        """Make POST request to provider API.
        
        Args:
            endpoint: API endpoint
            data: Request data
            stream: Whether to stream the response
            
        Returns:
            Response data or stream
            
        Raises:
            ProviderError: On API error
        """
        url = f"{self.base_url.rstrip('/')}/{endpoint.lstrip('/')}"
        try:
            async with self.client as client:
                response = await client.post(
                    url,
                    headers=self.headers,
                    json=data,
                    timeout=30.0
                )
                response.raise_for_status()
                
                if stream:
                    return self._handle_stream(response)
                return response.json()
                
        except httpx.HTTPError as e:
            logger.error(f"HTTP error occurred: {str(e)}")
            raise ProviderError(f"HTTP error: {str(e)}")
            
        except Exception as e:
            logger.error(f"Unexpected error: {str(e)}")
            raise ProviderError(f"Unexpected error: {str(e)}")
            
    async def _handle_stream(self, response: httpx.Response) -> AsyncGenerator[Dict[str, Any], None]:
        """Handle streaming response.
        
        Args:
            response: HTTP response
            
        Yields:
            Parsed response chunks
        """
        async for line in response.aiter_lines():
            if line:
                try:
                    yield json.loads(line)
                except json.JSONDecodeError as e:
                    logger.error(f"Failed to parse stream chunk: {str(e)}")
                    continue
