from typing import Optional, List
from fastapi import APIRouter, Depends, Request, HTTPException, Security
from fastapi.responses import StreamingResponse
from sqlalchemy.ext.asyncio import AsyncSession
from fastapi.security import HTTPBearer, HTTPAuthorizationCredentials
import json

from app.db.session import get_db
from app.core.security import verify_token, get_current_user
from app.features.chat.schemas.chat import (
    ChatCompletionRequest,
    ChatHistoryResponse,
    ChatDetailResponse
)
from app.features.chat.service.chat import ChatService

router = APIRouter(
    prefix="/v1/chat",
    tags=["chat"],
    responses={401: {"description": "Unauthorized"}}
)
security = HTTPBearer()

@router.post("/x", 
    summary="Create a chat completion",
    description="""
    Creates a completion for the chat message. This endpoint mirrors the OpenAI chat completions API.
    
    The endpoint supports both streaming and non-streaming responses:
    - When stream=false (default), returns a single JSON response
    - When stream=true, returns multiple chunks in Server-Sent Events format
    
    Authentication is required via Bearer token.
    """,
    response_description="Chat completion response in OpenAI format"
)
async def create_chat_completion(
    request: ChatCompletionRequest,
    raw_request: Request,
    credentials: HTTPAuthorizationCredentials = Security(security),
    db: AsyncSession = Depends(get_db)
):
    # Verify token and get user
    user = await get_current_user(credentials.credentials)
    if not user:
        raise HTTPException(status_code=401, detail="Invalid authentication token")
    
    # Get client IP address
    ip_address = raw_request.client.host
    
    chat_service = ChatService(db)
    
    if request.stream:
        async def generate_stream():
            async for chunk in chat_service.create_chat_completion(
                request,
                user.id,
                ip_address
            ):
                if "error" in chunk:
                    raise HTTPException(status_code=500, detail=chunk["error"])
                yield f"data: {json.dumps(chunk)}\n\n"
            yield "data: [DONE]\n\n"
        
        return StreamingResponse(
            generate_stream(),
            media_type="text/event-stream"
        )
    else:
        async for response in chat_service.create_chat_completion(
            request,
            user.id,
            ip_address
        ):
            if "error" in response:
                raise HTTPException(status_code=500, detail=response["error"])
            return response

@router.get("/history",
    response_model=List[ChatHistoryResponse],
    summary="Get chat history",
    description="""
    Retrieves the chat history for the authenticated user.
    
    Parameters:
    - conversation_id (optional): Filter by specific conversation
    - limit: Maximum number of records to return (default: 10)
    - offset: Number of records to skip (default: 0)
    
    Results are ordered by creation time (newest first).
    """
)
async def get_chat_history(
    conversation_id: Optional[str] = None,
    limit: int = 10,
    offset: int = 0,
    credentials: HTTPAuthorizationCredentials = Security(security),
    db: AsyncSession = Depends(get_db)
):
    user = await get_current_user(credentials.credentials)
    if not user:
        raise HTTPException(status_code=401, detail="Invalid authentication token")
    
    chat_service = ChatService(db)
    history = await chat_service.get_chat_history(
        user.id,
        conversation_id,
        limit,
        offset
    )
    return history

@router.get("/detail/{chat_id}",
    response_model=ChatDetailResponse,
    summary="Get chat detail",
    description="""
    Retrieves detailed information about a specific chat session.
    
    Includes:
    - Complete request and response data
    - Token usage and cost information
    - Timestamps and status information
    
    Returns 404 if the chat session is not found or belongs to another user.
    """
)
async def get_chat_detail(
    chat_id: int,
    credentials: HTTPAuthorizationCredentials = Security(security),
    db: AsyncSession = Depends(get_db)
):
    user = await get_current_user(credentials.credentials)
    if not user:
        raise HTTPException(status_code=401, detail="Invalid authentication token")
    
    chat_service = ChatService(db)
    chat_detail = await chat_service.get_chat_detail(chat_id, user.id)
    
    if not chat_detail:
        raise HTTPException(status_code=404, detail="Chat session not found")
    
    return chat_detail
