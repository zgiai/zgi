"""Chat router module."""
from typing import Dict, Any, Optional
from fastapi import APIRouter, HTTPException, Header, Depends
from pydantic import BaseModel, Field
from sqlalchemy.orm import Session
from ..providers.router import LLMRouter
from ..auth.api_key_manager import APIKeyManager
from app.core.database import get_db
import logging

# Create router with prefix
router = APIRouter(prefix="/v1")
llm_router = LLMRouter()
api_key_manager = APIKeyManager()

class Message(BaseModel):
    """Chat message."""
    role: str = Field(..., description="The role of the message sender (e.g., user, assistant)")
    content: str = Field(..., description="The content of the message")

class ChatCompletionRequest(BaseModel):
    """Chat completion request."""
    model: str = Field(..., description="The model to use for completion")
    messages: list[Message] = Field(..., description="The messages to generate completion for")
    temperature: Optional[float] = Field(0.7, description="Sampling temperature")
    max_tokens: Optional[int] = Field(None, description="Maximum number of tokens to generate")
    stream: Optional[bool] = Field(False, description="Whether to stream the response")

async def get_user_info(
    authorization: str = Header(None),
    db: Session = Depends(get_db)
) -> Optional[Dict[str, Any]]:
    """Get user info from authorization header.
    
    Args:
        authorization: Authorization header value
        db: Database session
        
    Returns:
        User info if authorized
        
    Raises:
        HTTPException: If authorization is invalid
    """
    if not authorization:
        return None
        
    parts = authorization.split()
    if len(parts) != 2 or parts[0].lower() != "bearer":
        raise HTTPException(
            status_code=401,
            detail="Invalid authorization header. Use 'Bearer YOUR_API_KEY'"
        )
        
    api_key = parts[1]
    user_info = await api_key_manager.validate_api_key(api_key, db)
    
    if not user_info:
        raise HTTPException(
            status_code=401,
            detail="Invalid API key"
        )
        
    return user_info

@router.post("/chat/completions")
async def create_chat_completion(
    request: ChatCompletionRequest,
    user_info: Optional[Dict[str, Any]] = Depends(get_user_info),
    db: Session = Depends(get_db)
) -> Dict[str, Any]:
    """Create a chat completion.
    
    Args:
        request: Chat completion request
        user_info: Optional user info from authorization
        db: Database session
        
    Returns:
        Chat completion response
        
    Raises:
        HTTPException: If request is invalid or processing fails
    """
    try:
        # Get provider name from model
        provider_name = llm_router.get_provider_name(request.model)
        
        # If user is authenticated, get provider key
        provider_key = None
        if user_info:
            provider_key = api_key_manager.get_provider_key(provider_name, user_info)
            if not provider_key:
                raise HTTPException(
                    status_code=403,
                    detail=f"Not authorized to use provider: {provider_name}"
                )
        
        # Convert request to provider format
        provider_request = {
            "model": request.model,
            "messages": [msg.dict() for msg in request.messages],
            "temperature": request.temperature,
            "max_tokens": request.max_tokens,
            "stream": request.stream
        }
        
        if provider_key:
            provider_request["api_key"] = provider_key
            
        # Route request to appropriate provider
        response = await llm_router.route_request(provider_request)
        
        # Update usage if user is authenticated
        if user_info and "usage" in response:
            tokens_used = sum(response["usage"].values())
            await api_key_manager.update_usage(user_info, tokens_used, db)
            
        return response
        
    except ValueError as e:
        logging.error(f"Validation error: {str(e)}")
        raise HTTPException(
            status_code=400,
            detail=str(e)
        )
    except Exception as e:
        logging.error(f"Error processing request: {str(e)}")
        raise HTTPException(
            status_code=500,
            detail=f"Error processing request: {str(e)}"
        )
