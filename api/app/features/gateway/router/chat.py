"""Chat completion router"""
from fastapi import APIRouter, HTTPException, Depends
from fastapi.responses import StreamingResponse, JSONResponse
from typing import Dict, Any, AsyncGenerator
import json
import logging
from sqlalchemy.orm import Session
from app.core.database import get_db
from ..service.llm_service import LLMService
from ..schemas.chat import ChatCompletionRequest, ChatCompletionResponse
from app.core.auth import get_api_key

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

        # Process the request through the service layer
        response = await LLMService.create_chat_completion(params, db)
        
        # Handle streaming response
        if request.stream:
            logger.debug("Streaming response")
            return StreamingResponse(
                stream_response(response),
                media_type="text/event-stream"
            )
            
        logger.debug("Returning non-streaming response")
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
