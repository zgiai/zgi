from datetime import datetime
from typing import Optional, List, Dict, Any
from pydantic import BaseModel, Field

class DocumentBase(BaseModel):
    filename: str
    file_type: str
    file_size: int
    metadata: Dict[str, Any] = Field(default_factory=dict)

class DocumentCreate(DocumentBase):
    pass

class DocumentResponse(DocumentBase):
    id: int
    user_id: int
    team_id: Optional[int] = None
    status: str
    chunk_count: int
    error: Optional[str] = None
    created_at: datetime
    processed_at: Optional[datetime] = None

    class Config:
        from_attributes = True

class DocumentListParams(BaseModel):
    page: int = Field(default=1, ge=1)
    page_size: int = Field(default=20, ge=1, le=100)
    status: Optional[str] = None
    file_type: Optional[str] = None
    team_id: Optional[int] = None

class DocumentListResponse(BaseModel):
    items: List[DocumentResponse]
    total: int
    page: int
    page_size: int

class SearchRequest(BaseModel):
    query: str = Field(..., min_length=1)
    document_ids: Optional[List[int]] = None
    team_id: Optional[int] = None
    top_k: int = Field(default=3, ge=1, le=10)
    min_score: float = Field(default=0.5, ge=0, le=1.0)

class ChunkResult(BaseModel):
    text: str
    score: float
    metadata: Dict[str, Any] = Field(default_factory=dict)
    document_id: int
    chunk_index: int

class SearchResponse(BaseModel):
    query: str
    results: List[ChunkResult]
    total_chunks: int
    duration_ms: int

class GenerateRequest(BaseModel):
    query: str = Field(..., min_length=1)
    context_chunks: List[ChunkResult]
    document_ids: Optional[List[int]] = None
    model: Optional[str] = None
    temperature: float = Field(default=0.7, ge=0, le=2.0)
    max_tokens: Optional[int] = None

class GenerateResponse(BaseModel):
    response: str
    tokens_used: int
    model_used: str
    duration_ms: int
    metadata: Dict[str, Any] = Field(default_factory=dict)

class QueryRequest(BaseModel):
    query: str = Field(..., min_length=1)
    document_ids: Optional[List[int]] = None
    team_id: Optional[int] = None
    model: Optional[str] = None
    temperature: float = Field(default=0.7, ge=0, le=2.0)
    max_tokens: Optional[int] = None
    top_k: int = Field(default=3, ge=1, le=10)
    min_score: float = Field(default=0.5, ge=0, le=1.0)

class QueryResponse(BaseModel):
    query: str
    response: str
    context_chunks: List[ChunkResult]
    tokens_used: int
    model_used: str
    duration_ms: int
    metadata: Dict[str, Any] = Field(default_factory=dict)
