"""
ZGI Python SDK
~~~~~~~~~~~~~

A Python SDK for interacting with ZGI AI services.

Basic usage:

    >>> from zgi import ZGIClient
    >>> client = ZGIClient(api_key="your-api-key")
    >>> response = client.chat([{"role": "user", "content": "Hello!"}])
    >>> print(response)

For more information, see https://docs.zgi.ai
"""

from .client import ZGIClient
from .exceptions import (
    ZGIError,
    APIError,
    AuthenticationError,
    RateLimitError,
    ValidationError,
    ResourceNotFoundError,
    InvalidRequestError,
)
from .utils import configure_logging

__version__ = "0.1.0"
__author__ = "ZGI Team"
__license__ = "MIT"

__all__ = [
    "ZGIClient",
    "ZGIError",
    "APIError",
    "AuthenticationError",
    "RateLimitError",
    "ValidationError",
    "ResourceNotFoundError",
    "InvalidRequestError",
    "configure_logging",
]
