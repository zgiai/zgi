from fastapi import APIRouter, Depends, HTTPException, status, Query, UploadFile, File
from fastapi.responses import StreamingResponse
from sqlalchemy.orm import Session
from datetime import datetime
from typing import Optional, List

from app.core.database import get_db
from app.core.auth import get_current_user
from app.features.users.models import User
from app.features.applications.models import Application
from app.features.chat.schemas import (
    ChatRequest, SwitchModelRequest, SwitchModelResponse,
    ChatHistoryParams, ChatHistoryResponse, TagsUpdateRequest,
    SessionMetadata
)
from app.features.chat.file_schemas import FileUploadResponse
from app.features.chat.service import ChatService
from app.features.chat.file_service import ChatFileService
from app.features.chat.prompt_service import PromptService
from app.features.chat.prompt_schemas import (
    PromptCreate,
    PromptUpdate,
    PromptResponse,
    PromptListParams,
    PromptListResponse,
    PromptPreviewRequest,
    PromptPreviewResponse
)

router = APIRouter(prefix="/v1/chat", tags=["chat"])

@router.post("/stream")
async def stream_chat(
    request: ChatRequest,
    db: Session = Depends(get_db),
    current_user: User = Depends(get_current_user)
):
    """Stream chat responses from the model"""
    chat_service = ChatService(db)
    
    # Create generator for streaming response
    async def response_generator():
        async for token in chat_service.stream_chat(
            messages=request.messages,
            model=request.model,
            temperature=request.temperature,
            max_tokens=request.max_tokens,
            user=current_user,
            session_id=request.session_id
        ):
            yield f"data: {token}\n\n"
        
        # Send end of stream
        yield "data: [DONE]\n\n"
    
    return StreamingResponse(
        response_generator(),
        media_type="text/event-stream"
    )

@router.get("/history", response_model=ChatHistoryResponse)
async def get_chat_history(
    page: int = Query(1, ge=1),
    page_size: int = Query(10, ge=1, le=100),
    start_date: Optional[datetime] = None,
    end_date: Optional[datetime] = None,
    model: Optional[str] = None,
    search_term: Optional[str] = None,
    tags: Optional[List[str]] = Query(None),
    db: Session = Depends(get_db),
    current_user: User = Depends(get_current_user)
):
    """Get chat history for the current user"""
    chat_service = ChatService(db)
    params = ChatHistoryParams(
        page=page,
        page_size=page_size,
        start_date=start_date,
        end_date=end_date,
        model=model,
        search_term=search_term,
        tags=tags
    )
    return chat_service.get_chat_history(current_user.id, params)

@router.post("/sessions/{session_id}/tags")
async def add_tags(
    session_id: int,
    request: TagsUpdateRequest,
    db: Session = Depends(get_db),
    current_user: User = Depends(get_current_user)
):
    """Add tags to a chat session"""
    chat_service = ChatService(db)
    session = chat_service.add_tags(session_id, current_user.id, request.tags)
    return {"message": "Tags added successfully", "tags": session.tags}

@router.delete("/sessions/{session_id}/tags")
async def remove_tags(
    session_id: int,
    request: TagsUpdateRequest,
    db: Session = Depends(get_db),
    current_user: User = Depends(get_current_user)
):
    """Remove tags from a chat session"""
    chat_service = ChatService(db)
    session = chat_service.remove_tags(session_id, current_user.id, request.tags)
    return {"message": "Tags removed successfully", "tags": session.tags}

@router.delete("/sessions/{session_id}")
async def delete_chat_session(
    session_id: int,
    archive: bool = Query(False, description="Archive instead of hard delete"),
    db: Session = Depends(get_db),
    current_user: User = Depends(get_current_user)
):
    """Delete or archive a chat session"""
    chat_service = ChatService(db)
    if archive:
        chat_service.archive_session(session_id, current_user.id)
        return {"message": "Session archived successfully"}
    else:
        chat_service.delete_session(session_id, current_user.id)
        return {"message": "Session deleted successfully"}

@router.get("/sessions/{session_id}/metadata", response_model=SessionMetadata)
async def get_session_metadata(
    session_id: int,
    db: Session = Depends(get_db),
    current_user: User = Depends(get_current_user)
):
    """Get metadata for a chat session"""
    chat_service = ChatService(db)
    session = chat_service.get_session(session_id, current_user.id)
    if not session:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Chat session not found"
        )
    
    return SessionMetadata(
        message_count=session.message_count,
        total_tokens=session.total_tokens,
        last_message_at=session.last_message_at,
        tags=session.tags,
        summary=session.summary
    )

@router.post("/sessions/{session_id}/upload", response_model=FileUploadResponse)
async def upload_file(
    session_id: int,
    file: UploadFile = File(...),
    current_user: User = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """Upload a PDF file to attach to the chat session"""
    # Verify session exists and belongs to user
    chat_service = ChatService(db)
    session = chat_service.get_session(session_id)
    if not session or session.user_id != current_user.id:
        raise HTTPException(status_code=404, detail="Chat session not found")

    # Handle file upload
    file_service = ChatFileService(db)
    chat_file = await file_service.save_file(file, current_user.id, session_id)
    
    return FileUploadResponse(
        file=chat_file,
        message="File uploaded successfully"
    )

# Prompt Management Endpoints
@router.post("/prompts", response_model=PromptResponse)
async def create_prompt(
    prompt: PromptCreate,
    current_user: User = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """Create a new prompt"""
    prompt_service = PromptService(db)
    return prompt_service.create_prompt(current_user.id, prompt)

@router.get("/prompts", response_model=PromptListResponse)
async def list_prompts(
    params: PromptListParams = Depends(),
    current_user: User = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """List prompts with filtering and pagination"""
    prompt_service = PromptService(db)
    prompts, total = prompt_service.list_prompts(current_user.id, params)
    return PromptListResponse(
        items=prompts,
        total=total,
        page=params.page,
        page_size=params.page_size
    )

@router.get("/prompts/{prompt_id}", response_model=PromptResponse)
async def get_prompt(
    prompt_id: int,
    current_user: User = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """Get a specific prompt"""
    prompt_service = PromptService(db)
    prompt = prompt_service.get_prompt(prompt_id, current_user.id)
    if not prompt:
        raise HTTPException(status_code=404, detail="Prompt not found")
    return prompt

@router.put("/prompts/{prompt_id}", response_model=PromptResponse)
async def update_prompt(
    prompt_id: int,
    prompt_update: PromptUpdate,
    current_user: User = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """Update a prompt"""
    prompt_service = PromptService(db)
    prompt = prompt_service.update_prompt(prompt_id, current_user.id, prompt_update)
    if not prompt:
        raise HTTPException(status_code=404, detail="Prompt not found")
    return prompt

@router.delete("/prompts/{prompt_id}")
async def delete_prompt(
    prompt_id: int,
    current_user: User = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """Delete a prompt"""
    prompt_service = PromptService(db)
    if not prompt_service.delete_prompt(prompt_id, current_user.id):
        raise HTTPException(status_code=404, detail="Prompt not found")
    return {"message": "Prompt deleted successfully"}

@router.post("/prompts/preview", response_model=PromptPreviewResponse)
async def preview_prompt(
    preview_request: PromptPreviewRequest,
    current_user: User = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """Preview a prompt with the chat model"""
    prompt_service = PromptService(db)
    preview = await prompt_service.preview_prompt(current_user.id, preview_request)
    return PromptPreviewResponse(**preview)
