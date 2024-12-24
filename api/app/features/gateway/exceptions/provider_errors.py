"""Provider-related exceptions."""

class ProviderError(Exception):
    """Base exception for provider errors."""
    pass

class InvalidAPIKeyError(ProviderError):
    """Raised when API key is invalid."""
    pass

class RateLimitError(ProviderError):
    """Raised when rate limit is exceeded."""
    pass

class TokenLimitError(ProviderError):
    """Raised when token limit is exceeded."""
    pass

class InvalidRequestError(ProviderError):
    """Raised when request is invalid."""
    pass

class ProviderTimeoutError(ProviderError):
    """Raised when provider request times out."""
    pass
