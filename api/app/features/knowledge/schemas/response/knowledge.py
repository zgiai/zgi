from datetime import datetime
from typing import List, Optional, Dict, Any
from pydantic import BaseModel, Field
from app.features.knowledge.models.knowledge import Visibility, Status

class KnowledgeBaseResponse(BaseModel):
    """Schema for knowledge base response"""
    id: int
    name: str
    description: Optional[str]
    visibility: str
    status: int
    collection_name: str
    model: str
    dimension: int
    
    # Statistics
    document_count: int = Field(default=0)
    total_chunks: int = Field(default=0)
    total_tokens: int = Field(default=0)
    
    # Metadata
    meta_info: Optional[Dict[str, Any]]
    tags: Optional[List[str]]
    
    # Ownership
    owner_id: int
    organization_id: Optional[int]
    
    # Timestamps
    created_at: str
    updated_at: str

    model_config = {
        "from_attributes": True,
        "json_encoders": {
            datetime: lambda dt: dt.isoformat()
        }
    }

class KnowledgeBaseList(BaseModel):
    """Schema for knowledge base list response"""
    total: int
    items: List[KnowledgeBaseResponse]

    model_config = {
        "from_attributes": True,
        "arbitrary_types_allowed": True
    }

class SearchResult(BaseModel):
    """Schema for search result"""
    text: str
    score: float
    document_id: int
    chunk_index: int
    metadata: Dict[str, Any]
    
    # Document information
    file_name: str
    file_type: str
    title: Optional[str]
    source_url: Optional[str]
    
    # Chunk context
    context_before: Optional[str]
    context_after: Optional[str]

class SearchResponse(BaseModel):
    """Schema for search response"""
    query: str
    results: List[SearchResult]
    total_chunks: int
    processing_time: float
