"""Response format conversion utilities."""
from typing import Dict, Any, Optional

def create_chat_response(
    id: str,
    model: str,
    content: str,
    finish_reason: str = "stop",
    usage: Optional[Dict[str, int]] = None,
    created: str = ""
) -> Dict[str, Any]:
    """Create a standardized chat completion response.
    
    Args:
        id: Response ID
        model: Model name
        content: Response content
        finish_reason: Reason for completion
        usage: Token usage statistics
        created: Creation timestamp
        
    Returns:
        Standardized response format
    """
    return {
        "id": id,
        "object": "chat.completion",
        "created": created,
        "model": model,
        "choices": [{
            "index": 0,
            "message": {
                "role": "assistant",
                "content": content
            },
            "finish_reason": finish_reason
        }],
        "usage": usage or {
            "prompt_tokens": 0,
            "completion_tokens": 0,
            "total_tokens": 0
        }
    }

def create_streaming_chunk(
    id: str,
    model: str,
    content: Optional[str] = None,
    role: Optional[str] = None,
    finish_reason: Optional[str] = None,
    created: str = ""
) -> Dict[str, Any]:
    """Create a standardized streaming chunk.
    
    Args:
        id: Chunk ID
        model: Model name
        content: Content delta
        role: Role delta
        finish_reason: Finish reason
        created: Creation timestamp
        
    Returns:
        Standardized chunk format
    """
    chunk = {
        "id": id,
        "object": "chat.completion.chunk",
        "created": created,
        "model": model,
        "choices": [{
            "index": 0,
            "delta": {},
            "finish_reason": finish_reason
        }]
    }
    
    if role:
        chunk["choices"][0]["delta"]["role"] = role
    if content:
        chunk["choices"][0]["delta"]["content"] = content
        
    return chunk

def extract_usage_stats(
    data: Dict[str, Any],
    input_key: str = "input_tokens",
    output_key: str = "output_tokens",
    total_key: str = "total_tokens",
    usage_key: str = "usage"
) -> Dict[str, int]:
    """Extract token usage statistics from response data.
    
    Args:
        data: Response data
        input_key: Key for input tokens
        output_key: Key for output tokens
        total_key: Key for total tokens
        usage_key: Key for usage object
        
    Returns:
        Usage statistics in standard format
    """
    usage = data.get(usage_key, {})
    return {
        "prompt_tokens": usage.get(input_key, 0),
        "completion_tokens": usage.get(output_key, 0),
        "total_tokens": usage.get(total_key, 0)
    }
