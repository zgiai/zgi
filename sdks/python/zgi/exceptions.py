class ZGIError(Exception):
    """Base exception class for ZGI SDK."""
    pass


class APIError(ZGIError):
    """Raised when the API returns an error response."""
    pass


class AuthenticationError(ZGIError):
    """Raised when there are issues with authentication."""
    pass


class RateLimitError(ZGIError):
    """Raised when rate limit is exceeded."""
    pass


class ValidationError(ZGIError):
    """Raised when request validation fails."""
    pass


class ResourceNotFoundError(ZGIError):
    """Raised when a requested resource is not found."""
    pass


class InvalidRequestError(ZGIError):
    """Raised when the request is invalid."""
    pass
