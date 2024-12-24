"""HTTP client utilities."""
from typing import Dict, Any, AsyncGenerator, Optional, Union
import httpx
import json
import logging
from contextlib import asynccontextmanager
from ..exceptions.provider_errors import ProviderError

logger = logging.getLogger(__name__)

class HttpClient:
    """Generic HTTP client for provider communication."""
    
    def __init__(self, base_url: str, headers: Dict[str, str]):
        """Initialize HTTP client.
        
        Args:
            base_url: Base URL for requests
            headers: HTTP headers
        """
        self.base_url = base_url
        self.headers = headers
        self.client = httpx.AsyncClient(timeout=30.0)
        
    async def post(
        self,
        endpoint: str,
        data: Dict[str, Any],
        stream: bool = False
    ) -> Union[Dict[str, Any], AsyncGenerator[Dict[str, Any], None]]:
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

@asynccontextmanager
async def create_http_client(
    base_url: str = "",
    headers: Optional[Dict[str, str]] = None,
    timeout: float = 30.0,
    follow_redirects: bool = True,
    verify_ssl: bool = True
) -> httpx.AsyncClient:
    """Create an HTTP client with the given configuration.
    
    Args:
        base_url: Base URL for requests
        headers: HTTP headers
        timeout: Request timeout in seconds
        follow_redirects: Whether to follow redirects
        verify_ssl: Whether to verify SSL certificates
        
    Yields:
        Configured HTTP client
    """
    async with httpx.AsyncClient(
        base_url=base_url,
        headers=headers or {},
        timeout=timeout,
        follow_redirects=follow_redirects,
        verify=verify_ssl
    ) as client:
        yield client

async def stream_response(
    client: httpx.AsyncClient,
    method: str,
    url: str,
    **kwargs: Any
) -> AsyncGenerator[str, None]:
    """Stream response content line by line.
    
    Args:
        client: HTTP client
        method: HTTP method
        url: Request URL
        **kwargs: Additional arguments for the request
        
    Yields:
        Response content lines
    """
    async with client.stream(method, url, **kwargs) as response:
        response.raise_for_status()
        async for line in response.aiter_lines():
            yield line

async def make_json_request(
    client: httpx.AsyncClient,
    method: str,
    url: str,
    expected_status: Optional[int] = None,
    **kwargs: Any
) -> Dict[str, Any]:
    """Make a request and return JSON response.
    
    Args:
        client: HTTP client
        method: HTTP method
        url: Request URL
        expected_status: Expected response status code
        **kwargs: Additional arguments for the request
        
    Returns:
        JSON response data
        
    Raises:
        httpx.HTTPError: If request fails or status code doesn't match
    """
    response = await client.request(method, url, **kwargs)
    
    if expected_status and response.status_code != expected_status:
        raise httpx.HTTPError(
            f"Unexpected status code: {response.status_code}, expected: {expected_status}",
            request=response.request,
            response=response
        )
        
    response.raise_for_status()
    return response.json()

class RetryConfig:
    """Configuration for request retries."""
    
    def __init__(
        self,
        max_retries: int = 3,
        retry_statuses: set[int] = {408, 429, 500, 502, 503, 504},
        retry_methods: set[str] = {"GET", "HEAD", "PUT", "DELETE", "OPTIONS", "TRACE"},
        backoff_factor: float = 0.1
    ):
        """Initialize retry configuration.
        
        Args:
            max_retries: Maximum number of retries
            retry_statuses: HTTP status codes to retry on
            retry_methods: HTTP methods to retry
            backoff_factor: Backoff factor for exponential backoff
        """
        self.max_retries = max_retries
        self.retry_statuses = retry_statuses
        self.retry_methods = retry_methods
        self.backoff_factor = backoff_factor
        
    def should_retry(self, method: str, status_code: int, attempt: int) -> bool:
        """Check if request should be retried.
        
        Args:
            method: HTTP method
            status_code: Response status code
            attempt: Current attempt number
            
        Returns:
            Whether to retry the request
        """
        return (
            attempt < self.max_retries
            and method in self.retry_methods
            and status_code in self.retry_statuses
        )
