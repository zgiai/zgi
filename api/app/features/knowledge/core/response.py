from typing import Optional, TypeVar, Generic, Any, Dict

from fastapi import HTTPException
from pydantic import BaseModel, ConfigDict
from enum import Enum

T = TypeVar('T')

class ErrorCode(int, Enum):
    """Error codes for knowledge base service"""
    SUCCESS = 200
    CREATED = 201
    BAD_REQUEST = 400
    UNAUTHORIZED = 401
    FORBIDDEN = 403
    NOT_FOUND = 404
    CONFLICT = 409
    INTERNAL_ERROR = 500
    SERVICE_UNAVAILABLE = 503

class ServiceError(HTTPException):
    """Base exception for service errors"""
    def __init__(
        self,
        message: str,
        code: ErrorCode = ErrorCode.INTERNAL_ERROR,
        details: Optional[Dict[str, Any]] = None
    ):
        self.message = message
        self.code = code
        self.details = details or {}
        super().__init__(
            status_code=code,
            detail=message
        )

class ServiceResponse(BaseModel, Generic[T]):
    """Standard service response wrapper"""
    model_config = ConfigDict(arbitrary_types_allowed=True)
    
    success: bool
    code: ErrorCode
    message: Optional[str] = None
    data: Optional[T] = None
    details: Optional[Dict[str, Any]] = None

    @classmethod
    def ok(cls, data: Optional[T] = None, message: str = "Success") -> "ServiceResponse[T]":
        """Create a successful response"""
        return cls(
            success=True,
            code=ErrorCode.SUCCESS,
            message=message,
            data=data
        )
    
    @classmethod
    def created(cls, data: Optional[T] = None, message: str = "Created") -> "ServiceResponse[T]":
        """Create a response for resource creation"""
        return cls(
            success=True,
            code=ErrorCode.CREATED,
            message=message,
            data=data
        )
    
    @classmethod
    def error(
        cls,
        error: ServiceError,
    ) -> "ServiceResponse[T]":
        """Create an error response"""
        return cls(
            success=False,
            code=error.code,
            message=error.message,
            details=error.details
        )

# Common errors
class NotFoundError(ServiceError):
    def __init__(self, message: str, details: Optional[Dict[str, Any]] = None):
        super().__init__(message, ErrorCode.NOT_FOUND, details)

class ValidationError(ServiceError):
    def __init__(self, message: str, details: Optional[Dict[str, Any]] = None):
        super().__init__(message, ErrorCode.BAD_REQUEST, details)

class AuthorizationError(ServiceError):
    def __init__(self, message: str, details: Optional[Dict[str, Any]] = None):
        super().__init__(message, ErrorCode.FORBIDDEN, details)

class ConflictError(ServiceError):
    def __init__(self, message: str, details: Optional[Dict[str, Any]] = None):
        super().__init__(message, ErrorCode.CONFLICT, details)
