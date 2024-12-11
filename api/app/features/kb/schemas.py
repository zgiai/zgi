from datetime import datetime
from typing import List, Optional
from pydantic import BaseModel, Field

from app.models.knowledge_base import Visibility


class KnowledgeBaseCreate(BaseModel):
    name: str = Field(..., description="Name of the knowledge base")
    description: Optional[str] = Field(None, description="Description of the knowledge base")
    visibility: Visibility = Field(default=Visibility.PRIVATE, description="Visibility of the knowledge base")


class KnowledgeBaseUpdate(BaseModel):
    name: Optional[str] = Field(None, description="Updated name of the knowledge base")
    description: Optional[str] = Field(None, description="Updated description of the knowledge base")
    visibility: Optional[Visibility] = Field(None, description="Updated visibility of the knowledge base")


class KnowledgeBaseResponse(BaseModel):
    id: int
    name: str
    description: Optional[str]
    visibility: Visibility
    owner_id: int
    created_at: datetime
    updated_at: datetime

    class Config:
        orm_mode = True


class KnowledgeBaseList(BaseModel):
    total: int
    items: List[KnowledgeBaseResponse]


class DocumentUpload(BaseModel):
    kb_id: int = Field(..., description="ID of the knowledge base")


class DocumentResponse(BaseModel):
    id: int
    kb_id: int
    file_name: str
    created_at: datetime
    updated_at: datetime

    class Config:
        orm_mode = True


class DocumentList(BaseModel):
    total: int
    items: List[DocumentResponse]


class SearchQuery(BaseModel):
    kb_id: int = Field(..., description="ID of the knowledge base to search in")
    query: str = Field(..., description="Search query text")
    top_k: int = Field(default=5, description="Number of results to return", ge=1, le=20)


class SearchResult(BaseModel):
    text: str
    score: float
    document_id: int
    file_name: str


class SearchResponse(BaseModel):
    results: List[SearchResult]
