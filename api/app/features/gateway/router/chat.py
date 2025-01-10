"""Chat completion router"""
from fastapi import APIRouter, HTTPException, Depends
from fastapi.responses import StreamingResponse, JSONResponse
from typing import Dict, Any, AsyncGenerator, List, Optional
import json
import logging
from sqlalchemy.orm import Session
from app.core.base import resp_200
from app.core.database import get_db, get_sync_db
from ..service.llm_service import LLMService
from ..schemas.chat import ChatCompletionRequest, ChatCompletionResponse, AddChatMessagesRequest, ChatMessagesResponse, \
    ConversationListResponse, ChatMessages, Message, ConversationResponse
from app.core.auth import get_api_key, get_current_user, get_current_user_or_none
from ... import User, Conversation, ChatMessage

# Configure logging
logger = logging.getLogger(__name__)
logger.setLevel(logging.DEBUG)

# OpenAI compatible router
router = APIRouter(prefix="/v1")

async def stream_response(generator: AsyncGenerator[Dict[str, Any], None]):
    """Stream response in SSE format"""
    try:
        async for chunk in generator:
            if chunk:
                yield f"data: {json.dumps(chunk)}\n\n"
        yield "data: [DONE]\n\n"
    except Exception as e:
        logger.error(f"Error streaming response: {str(e)}")
        yield f"data: {json.dumps({'error': str(e)})}\n\n"

async def collect_response(generator: AsyncGenerator[Dict[str, Any], None]) -> Dict[str, Any]:
    """Collect all chunks from generator into a single response."""
    chunks: List[Dict[str, Any]] = []
    try:
        async for chunk in generator:
            if chunk:
                chunks.append(chunk)
        return chunks[-1] if chunks else {}
    except Exception as e:
        logger.error(f"Error collecting response: {str(e)}")
        raise

@router.post("/chat/completions", response_model=ChatCompletionResponse)
async def create_chat_completion(
    request: ChatCompletionRequest,
    api_key: str = Depends(get_api_key),
    db: Session = Depends(get_db)
) -> ChatCompletionResponse:
    """
    Create a chat completion using the specified model.
    Compatible with OpenAI API specification.
    
    Args:
        request: Chat completion request parameters
        api_key: API key for authentication
        db: Database session
    
    Returns:
        Chat completion response from the provider
    
    Raises:
        HTTPException: If the request is invalid or the provider returns an error
    """
    try:
        # Convert request to dict
        params = request.dict(exclude_unset=True)
        params["api_key"] = api_key
        
        logger.debug(f"Processing chat completion request with params: {params}")

        # Get response generator from service
        response_generator = LLMService.create_chat_completion(params, db)
        
        # Handle streaming response
        if request.stream:
            logger.debug("Streaming response")
            return StreamingResponse(
                stream_response(response_generator),
                media_type="text/event-stream"
            )
            
        # For non-streaming, collect all chunks into a single response
        logger.debug("Collecting non-streaming response")
        response = await collect_response(response_generator)
        return JSONResponse(content=response)

    except ValueError as e:
        # Format error response according to OpenAI API spec
        error_message = str(e)
        logger.error(f"Invalid request error: {error_message}")
        
        error_response = {
            "status_code": 400,
            "status_message": {
                "message": error_message,
                "type": "invalid_request_error",
                "param": None,
                "code": "invalid_request_error"
            }
        }
        return JSONResponse(
            status_code=400,
            content=error_response
        )
        
    except Exception as e:
        # Format error response according to OpenAI API spec
        error_message = str(e)
        logger.error(f"Internal server error: {error_message}")
        
        error_response = {
            "status_code": 500,
            "status_message": {
                "message": error_message,
                "type": "internal_server_error",
                "param": None,
                "code": "internal_server_error"
            }
        }
        return JSONResponse(
            status_code=500,
            content=error_response
        )

@router.post("/chat/add_chat_messages")
async def add_chat_messages(
        request: AddChatMessagesRequest,
        user: User = Depends(get_current_user_or_none),
        db: Session = Depends(get_sync_db)
):
    """
    Add chat messages to a conversation session.

    Args:
        request: Request containing session ID and messages
        api_key: API key for authentication
        db: Database session

    Returns:
        Success message

    Raises:
        HTTPException: If there is an error saving the messages
    """
    try:
        conversation = LLMService.add_chat_messages(request, user, db)

        return resp_200(data=ConversationResponse(session_id=conversation.session_id),
                        message="SUCCESS")
    except Exception as e:
        error_response = {
            "error": {
                "message": str(e),
                "type": "internal_server_error",
                "param": None,
                "code": "internal_server_error"
            }
        }
        raise HTTPException(status_code=500, detail=error_response)


@router.get("/chat/conversations")
async def get_conversations(
        page_size: Optional[int] = 10,
        page_num: Optional[int] = 1,
        user: User = Depends(get_current_user_or_none),
        db: Session = Depends(get_sync_db)
):
    """
    Get conversations for the current user.

    Args:
        page_num: Page number
        page_size: Number of conversations per page
        user: Current user
        db: Database session

    Returns:
        List of conversations with UUID and first message

    Raises:
        HTTPException: If there is an error retrieving the conversations
    """
    try:

        # Query conversations for the current user
        count_query = (
            db.query(Conversation)
            .order_by(Conversation.created_at.desc())
        )
        if user:
            count_query = count_query.filter(Conversation.user_id == user.id)
        else:
            count_query = count_query.filter(Conversation.user_id.is_(None))
        total = count_query.count()
        if page_num and page_size:
            # Calculate offset
            offset = (page_num - 1) * page_size
            count_query = count_query.offset(offset).limit(page_size)
        conversations = count_query.all()

        # Prepare response data
        response_data = []
        for conversation in conversations:
            # Get the first message of the conversation
            first_message = (
                db.query(ChatMessage)
                .filter(ChatMessage.conversation_id == conversation.id)
                .order_by(ChatMessage.timestamp.asc())
                .first()
            )
            message_data = ChatMessages(
                session_id=conversation.session_id
            )
            if first_message:
                message_data.messages = [first_message]
            response_data.append(message_data)

        return resp_200(ConversationListResponse(messages=response_data, total=total))

    except Exception as e:
        error_response = {
            "error": {
                "message": str(e),
                "type": "internal_server_error",
                "param": None,
                "code": "internal_server_error"
            }
        }
        raise HTTPException(status_code=500, detail=error_response)


@router.get("/chat/chat_history")
async def get_chat_history(
        session_id: str,
        page_size: Optional[int] = 10,
        page_num: Optional[int] = 1,
        user: User = Depends(get_current_user_or_none),
        db: Session = Depends(get_sync_db)
):
    """
    Get chat history for a specific session.

    Args:
        session_id: Unique identifier for the conversation session
        page_num: Page number
        page_size: Number of conversations per page
        user: Current user
        db: Database session

    Returns:
        List of messages in the conversation

    Raises:
        HTTPException: If there is an error retrieving the messages
    """
    try:
        conversation = db.query(Conversation).filter_by(session_id=session_id).first()
        if not conversation:
            raise ValueError(f"No conversation found with session_id: {session_id}")
        user_id = user.id if user else None
        user_type = user.user_type if user else 0
        if conversation.user_id != user_id and user_type == 0:
            raise ValueError("You are not authorized to access this conversation")
        count_query = (
            db.query(ChatMessage)
            .filter(ChatMessage.conversation_id == conversation.id)
            .order_by(ChatMessage.timestamp.desc())
        )
        total = count_query.count()
        if page_num and page_size:
            # Calculate offset
            offset = (page_num - 1) * page_size
            count_query = count_query.offset(offset).limit(page_size)
        messages = count_query.all()
        # messages = [Message(content=message.content, role=message.role) for message in result]
        return resp_200(ChatMessagesResponse(messages=messages, session_id=session_id, total=total))
    except ValueError as e:
        error_response = {
            "error": {
                "message": str(e),
                "type": "not_found_error",
                "param": None,
                "code": "not_found_error"
            }
        }
        raise HTTPException(status_code=404, detail=error_response)
    except Exception as e:
        error_response = {
            "error": {
                "message": str(e),
                "type": "internal_server_error",
                "param": None,
                "code": "internal_server_error"
            }
        }
        raise HTTPException(status_code=500, detail=error_response)
