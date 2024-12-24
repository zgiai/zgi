"""HTTP client utilities."""
from typing import Dict, Any, AsyncGenerator, Optional
import httpx
from contextlib import asynccontextmanager

@asynccontextmanager
async def create_http_client(
    base_url: str,
    headers: Dict[str, str],
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
        headers=headers,
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
            retry_statuses: Status codes to retry on
            retry_methods: HTTP methods to retry
            backoff_factor: Backoff factor for exponential backoff
        """
        self.max_retries = max_retries
        self.retry_statuses = retry_statuses
        self.retry_methods = retry_methods
        self.backoff_factor = backoff_factor
        
    def should_retry(self, response: httpx.Response, attempt: int) -> bool:
        """Determine if request should be retried.
        
        Args:
            response: Response object
            attempt: Current attempt number
            
        Returns:
            True if request should be retried
        """
        return (
            attempt < self.max_retries
            and response.status_code in self.retry_statuses
            and response.request.method in self.retry_methods
        )
