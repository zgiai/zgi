"""Response formatting utilities."""
from typing import Dict, Any, List
import json

def format_chat_messages(messages: List[Dict[str, Any]], provider: str) -> List[Dict[str, Any]]:
    """Format chat messages for specific provider.
    
    Args:
        messages: List of message objects
        provider: Provider name
        
    Returns:
        Formatted messages
    """
    if provider == "anthropic":
        return [
            {
                "role": msg["role"].replace("assistant", "assistant") if msg["role"] != "system" else "system",
                "content": msg["content"]
            }
            for msg in messages
        ]
    elif provider == "openai":
        return messages  # OpenAI format is our standard format
    elif provider == "deepseek":
        return [
            {
                "role": msg["role"],
                "content": msg["content"]
            }
            for msg in messages
        ]
    return messages

def format_completion_response(response: Dict[str, Any], provider: str) -> Dict[str, Any]:
    """Format completion response to standard format.
    
    Args:
        response: Provider response
        provider: Provider name
        
    Returns:
        Standardized response
    """
    if provider == "anthropic":
        return {
            "id": response.get("id", ""),
            "object": "chat.completion",
            "created": response.get("created", 0),
            "model": response.get("model", ""),
            "choices": [{
                "index": 0,
                "message": {
                    "role": "assistant",
                    "content": response.get("completion", "")
                },
                "finish_reason": response.get("stop_reason", "stop")
            }]
        }
    elif provider == "openai":
        return response  # OpenAI format is our standard
    elif provider == "deepseek":
        return {
            "id": response.get("id", ""),
            "object": "chat.completion",
            "created": response.get("created", 0),
            "model": response.get("model", ""),
            "choices": [{
                "index": 0,
                "message": {
                    "role": "assistant",
                    "content": response.get("choices", [{}])[0].get("message", {}).get("content", "")
                },
                "finish_reason": response.get("choices", [{}])[0].get("finish_reason", "stop")
            }]
        }
    return response
