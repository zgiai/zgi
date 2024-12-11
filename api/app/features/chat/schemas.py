from typing import Optional, List
from pydantic import BaseModel, Field
from datetime import datetime

class ChatMessage(BaseModel):
    role: str
    content: str

class ChatRequest(BaseModel):
    model: str
    messages: List[ChatMessage]
    temperature: Optional[float] = 0.7
    max_tokens: Optional[int] = None
    stream: Optional[bool] = True
    session_id: Optional[int] = None
    tags: Optional[List[str]] = None

class ChatResponse(BaseModel):
    id: str
    choices: List[dict]
    created: int
    model: str
    object: str

class ChatSession(BaseModel):
    id: int
    user_id: int
    application_id: Optional[int]
    model: str
    title: Optional[str]
    messages: List[ChatMessage]
    message_count: int
    total_tokens: int
    tags: List[str] = Field(default_factory=list)
    summary: Optional[str]
    created_at: datetime
    updated_at: datetime
    last_message_at: Optional[datetime]

    class Config:
        from_attributes = True

class SwitchModelRequest(BaseModel):
    session_id: int
    model: str

class SwitchModelResponse(BaseModel):
    session_id: int
    model: str
    messages: List[ChatMessage]
    title: Optional[str]

class ChatHistoryParams(BaseModel):
    page: int = 1
    page_size: int = 10
    start_date: Optional[datetime] = None
    end_date: Optional[datetime] = None
    model: Optional[str] = None
    search_term: Optional[str] = None
    tags: Optional[List[str]] = None

class ChatHistoryResponse(BaseModel):
    total: int
    page: int
    page_size: int
    sessions: List[ChatSession]

    class Config:
        from_attributes = True

class TagsUpdateRequest(BaseModel):
    tags: List[str]

class SessionMetadata(BaseModel):
    message_count: int
    total_tokens: int
    last_message_at: Optional[datetime]
    tags: List[str]
    summary: Optional[str]
