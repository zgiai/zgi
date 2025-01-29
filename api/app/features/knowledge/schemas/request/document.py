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
    chunk_rule: Optional[Dict[str, Any]] = Field(
        None,
        description="Custom chunking rules for text splitting"
    )


class DocumentUpdate(BaseModel):
    """Schema for document update request"""
    title: Optional[str] = Field(None, description="Updated title")
    language: Optional[str] = Field(None, description="Updated language")
    author: Optional[str] = Field(None, description="Updated author")
    source_url: Optional[str] = Field(None, description="Updated source URL")
    metadata: Optional[Dict[str, Any]] = Field(None, description="Updated metadata")
    tags: Optional[List[str]] = Field(None, description="Updated tags")
    chunk_rule: Optional[Dict[str, Any]] = Field(
        None,
        description="Custom chunking rules for text splitting"
    )
