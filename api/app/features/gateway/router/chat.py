"""Chat completion router"""
from fastapi import APIRouter, HTTPException, Depends
from fastapi.responses import StreamingResponse, JSONResponse
from typing import Dict, Any, AsyncGenerator
import json
from sqlalchemy.orm import Session
from app.core.database import get_db
from ..service.llm_service import LLMService
from ..schemas.chat import ChatCompletionRequest, ChatCompletionResponse
from app.core.auth import get_api_key

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

        # Process the request through the service layer
        response = await LLMService.create_chat_completion(params, db)
        
        # Handle streaming response
        if request.stream:
            return StreamingResponse(
                stream_response(response),
                media_type="text/event-stream"
            )
            
        return JSONResponse(content=response)

    except ValueError as e:
        # Format error response according to OpenAI API spec
        error_response = {
            "error": {
                "message": str(e),
                "type": "invalid_request_error",
                "param": None,
                "code": "invalid_request_error"
            }
        }
        raise HTTPException(status_code=400, detail=error_response)
    except Exception as e:
        # Format error response according to OpenAI API spec
        error_response = {
            "error": {
                "message": str(e),
                "type": "internal_server_error",
                "param": None,
                "code": "internal_server_error"
            }
        }
        raise HTTPException(status_code=500, detail=error_response)
