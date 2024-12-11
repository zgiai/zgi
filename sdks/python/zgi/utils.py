import logging
from typing import Optional, Dict
import json


def configure_logging(
    level: int = logging.INFO,
    format: str = "%(asctime)s - %(name)s - %(levelname)s - %(message)s"
) -> None:
    """Configure logging for the SDK.
    
    Args:
        level: Logging level (default: logging.INFO)
        format: Log message format
    """
    logging.basicConfig(level=level, format=format)


def validate_api_key(api_key: str) -> bool:
    """Validate API key format.
    
    Args:
        api_key: API key to validate
        
    Returns:
        bool: True if valid, False otherwise
    """
    if not api_key or not isinstance(api_key, str):
        return False
    return len(api_key) > 0


def format_error_message(response: Dict) -> str:
    """Format error message from API response.
    
    Args:
        response: API error response
        
    Returns:
        str: Formatted error message
    """
    if isinstance(response, dict):
        error = response.get("error", {})
        if isinstance(error, dict):
            message = error.get("message", "Unknown error")
            code = error.get("code", "")
            return f"{code}: {message}" if code else message
        return str(error)
    return str(response)


def safe_json_loads(data: str) -> Optional[Dict]:
    """Safely parse JSON string.
    
    Args:
        data: JSON string to parse
        
    Returns:
        Dict or None: Parsed JSON data or None if parsing fails
    """
    try:
        return json.loads(data)
    except (json.JSONDecodeError, TypeError):
        return None


def format_request_data(data: Dict) -> Dict:
    """Format request data for API.
    
    Args:
        data: Request data to format
        
    Returns:
        Dict: Formatted request data
    """
    formatted = {}
    for key, value in data.items():
        if value is not None:  # Skip None values
            formatted[key] = value
    return formatted


def validate_file_type(file_path: str, allowed_types: set) -> bool:
    """Validate file type against allowed types.
    
    Args:
        file_path: Path to file
        allowed_types: Set of allowed file extensions
        
    Returns:
        bool: True if valid, False otherwise
    """
    ext = file_path.lower().split('.')[-1]
    return ext in allowed_types
