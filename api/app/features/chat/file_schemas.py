from datetime import datetime
from typing import Optional, Dict, Any

from pydantic import BaseModel, Field

class ChatFileBase(BaseModel):
    filename: str
    mime_type: str
    file_size: int
    content_hash: str
    metadata: Dict[str, Any] = Field(default_factory=dict)

class ChatFileCreate(ChatFileBase):
    user_id: int
    session_id: int
    file_path: str
    extracted_text: Optional[str] = None

class ChatFileResponse(ChatFileBase):
    id: int
    user_id: int
    session_id: int
    created_at: datetime
    updated_at: datetime

    class Config:
        from_attributes = True

class FileUploadResponse(BaseModel):
    file: ChatFileResponse
    message: str = "File uploaded successfully"
