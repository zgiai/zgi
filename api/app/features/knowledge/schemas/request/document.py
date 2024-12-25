from typing import Optional, Dict, Any, List
from pydantic import BaseModel, Field, constr

class DocumentUpload(BaseModel):
    """Schema for document upload request"""
    title: Optional[str] = Field(None, description="Document title")
    language: Optional[str] = Field(None, description="Document language")
    author: Optional[str] = Field(None, description="Document author")
    source_url: Optional[str] = Field(None, description="Document source URL")
    metadata: Optional[Dict[str, Any]] = Field(
        None,
        description="Additional document metadata"
    )
    tags: Optional[List[str]] = Field(
        None,
        description="Document tags"
    )
    chunk_size: Optional[int] = Field(
        default=1000,
        ge=100,
        le=2000,
        description="Text chunk size"
    )
    chunk_overlap: Optional[int] = Field(
        default=200,
        ge=0,
        le=500,
        description="Text chunk overlap"
    )
    embedding_model: Optional[str] = Field(
        None,
        description="Override default embedding model"
    )

class DocumentUpdate(BaseModel):
    """Schema for document update request"""
    title: Optional[str] = Field(None, description="Updated title")
    language: Optional[str] = Field(None, description="Updated language")
    author: Optional[str] = Field(None, description="Updated author")
    source_url: Optional[str] = Field(None, description="Updated source URL")
    metadata: Optional[Dict[str, Any]] = Field(None, description="Updated metadata")
    tags: Optional[List[str]] = Field(None, description="Updated tags")
