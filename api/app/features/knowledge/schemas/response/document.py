from datetime import datetime
from typing import List, Optional, Dict, Any
from pydantic import BaseModel, ConfigDict
from app.features.knowledge.models.document import DocumentStatus

class DocumentResponse(BaseModel):
    """Schema for document response"""
    id: int
    kb_id: int
    
    # File information
    file_name: str
    file_type: str
    file_size: int
    file_hash: str
    
    # Processing information
    status: DocumentStatus
    error_message: Optional[str]
    chunk_count: int
    token_count: int
    vector_count: int
    
    # Document metadata
    title: Optional[str]
    language: Optional[str]
    author: Optional[str]
    source_url: Optional[str]
    meta_info: Optional[Dict[str, Any]]
    tags: Optional[List[str]]
    
    # Processing settings
    chunk_size: int
    chunk_overlap: int
    embedding_model: Optional[str]
    
    # Timestamps
    created_at: datetime
    updated_at: datetime
    processed_at: Optional[datetime]

    # class Config:
    #     orm_mode = True

    model_config = ConfigDict(
        from_attributes=True,
        json_encoders={
            datetime: lambda v: v.isoformat() if v else None,
            bytes: lambda v: v.decode() if v else None
        }
    )

class DocumentList(BaseModel):
    """Schema for document list response"""
    total: int
    items: List[DocumentResponse]

    class Config:
        from_attributes = True

class DocumentChunkResponse(BaseModel):
    """Schema for document chunk response"""
    id: int
    document_id: int
    chunk_index: int
    content: str
    token_count: int
    vector_id: Optional[str]
    embedding_model: Optional[str]
    metadata: Optional[Dict[str, Any]]
    created_at: datetime
    updated_at: datetime

    class Config:
        orm_mode = True

class DocumentChunkList(BaseModel):
    """Schema for document chunk list response"""
    total: int
    items: List[DocumentChunkResponse]
