"""Message format conversion utilities."""
from typing import Dict, Any, List, Tuple, Optional
import logging

logger = logging.getLogger(__name__)

def extract_system_message(messages: List[Dict[str, Any]]) -> Tuple[List[Dict[str, Any]], Optional[str]]:
    """Extract system message from a list of messages.
    
    Args:
        messages: List of message objects
        
    Returns:
        Tuple of (remaining_messages, system_message)
    """
    system = None
    remaining_messages = []
    
    for msg in messages:
        if msg.get("role") == "system":
            system = msg.get("content")
        else:
            remaining_messages.append(msg)
            
    return remaining_messages, system

def filter_messages_by_role(messages: List[Dict[str, Any]], allowed_roles: List[str]) -> List[Dict[str, Any]]:
    """Filter messages by role.
    
    Args:
        messages: List of message objects
        allowed_roles: List of allowed role names
        
    Returns:
        Filtered message list
    """
    filtered = []
    for msg in messages:
        role = msg.get("role")
        if role in allowed_roles:
            filtered.append(msg)
        else:
            logger.warning(f"Skipping message with unsupported role: {role}")
    return filtered

def validate_message_format(message: Dict[str, Any]) -> bool:
    """Validate message format.
    
    Args:
        message: Message object
        
    Returns:
        True if message format is valid
    """
    return (
        isinstance(message, dict) 
        and "role" in message 
        and "content" in message
        and isinstance(message["role"], str)
        and isinstance(message["content"], str)
    )

def convert_messages_to_provider_format(
    messages: List[Dict[str, Any]], 
    provider: str
) -> Tuple[List[Dict[str, Any]], Optional[str]]:
    """Convert messages to provider-specific format.
    
    Args:
        messages: List of message objects in OpenAI format
        provider: Provider name (e.g., "anthropic")
        
    Returns:
        Tuple of (formatted_messages, system_message)
    """
    if provider == "anthropic":
        return convert_to_anthropic_format(messages)
    # Add other providers here
    raise ValueError(f"Unsupported provider: {provider}")
    
def convert_to_anthropic_format(
    messages: List[Dict[str, Any]]
) -> Tuple[List[Dict[str, Any]], Optional[str]]:
    """Convert messages to Anthropic format.
    
    Args:
        messages: List of message objects in OpenAI format
        
    Returns:
        Tuple of (formatted_messages, system_message)
    """
    system = None
    messages_formatted = []
    
    for msg in messages:
        role = msg["role"]
        content = msg["content"]
        
        if role == "system":
            system = content
        elif role in ["user", "assistant"]:
            messages_formatted.append({"role": role, "content": content})
        else:
            logger.warning(f"Unsupported role: {role}, skipping message")
            
    return messages_formatted, system
