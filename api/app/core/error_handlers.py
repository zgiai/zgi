from fastapi import HTTPException, Request
from fastapi.responses import JSONResponse
from sqlalchemy.exc import SQLAlchemyError, InvalidRequestError
import traceback
import logging
import sys

logger = logging.getLogger(__name__)

def format_traceback(tb):
    """Format traceback into a readable format"""
    formatted = []
    for frame in traceback.extract_tb(tb):
        formatted.append({
            "filename": frame.filename,
            "line": frame.lineno,
            "function": frame.name,
            "code": frame.line
        })
    return formatted

async def database_error_handler(request: Request, exc: SQLAlchemyError):
    """Handle SQLAlchemy errors with detailed information"""
    # Debug log to confirm handler is called
    logger.error(f"Database error occurred: {str(exc)}")
    logger.error(f"Exception type: {type(exc)}")
    
    # Get the full exception info
    exc_type, exc_value, exc_traceback = sys.exc_info()
    
    # Extract the original error message and traceback
    error_message = str(exc)
    formatted_traceback = format_traceback(exc_traceback)
    
    # Log the full traceback
    logger.error("Full traceback:")
    for frame in formatted_traceback:
        logger.error(f"  File {frame['filename']}, line {frame['line']}, in {frame['function']}")
        if frame['code']:
            logger.error(f"    {frame['code']}")
    
    # Base error response structure
    error_response = {
        "detail": {
            "message": "Internal server error",
            "error_type": exc.__class__.__name__,
            "error_details": error_message
        }
    }
    
    # Add specific details for different types of SQLAlchemy errors
    if isinstance(exc, InvalidRequestError):
        error_response["detail"].update({
            "mapper_error": True,
            "failed_mapper": str(getattr(exc, 'mapper', 'Unknown mapper')),
            "original_exception": str(getattr(exc, '_message', error_message))
        })
    
    # Return error response
    return JSONResponse(
        status_code=500,
        content=error_response
    )

def setup_error_handlers(app):
    """Setup error handlers for the FastAPI application"""
    app.add_exception_handler(SQLAlchemyError, database_error_handler)
