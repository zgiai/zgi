from typing import Optional, Dict, Any, List
from pydantic import BaseModel, Field, constr
from app.features.knowledge.models.knowledge import Visibility, Status

class KnowledgeBaseCreate(BaseModel):
    """Schema for creating a knowledge base"""
    name: constr(min_length=1, max_length=255) = Field(..., description="Name of the knowledge base")
    description: Optional[str] = Field(None, description="Description of the knowledge base")
    visibility: Visibility = Field(
        default=Visibility.PRIVATE,
        description="Visibility of the knowledge base"
    )
    model: Optional[str] = Field(
        default="text-embedding-3-small",
        description="Embedding model to use"
    )
    organization_id: Optional[int] = Field(
        None,
        description="Organization ID if visibility is organization"
    )
    metadata: Optional[Dict[str, Any]] = Field(
        None,
        description="Additional metadata"
    )
    tags: Optional[List[str]] = Field(
        None,
        description="Knowledge base tags"
    )

class KnowledgeBaseUpdate(BaseModel):
    """Schema for updating a knowledge base"""
    name: Optional[constr(min_length=1, max_length=255)] = Field(None, description="Updated name")
    description: Optional[str] = Field(None, description="Updated description")
    visibility: Optional[Visibility] = Field(None, description="Updated visibility")
    status: Optional[Status] = Field(None, description="Updated status")
    metadata: Optional[Dict[str, Any]] = Field(None, description="Updated metadata")
    tags: Optional[List[str]] = Field(None, description="Updated tags")

class SearchQuery(BaseModel):
    """Schema for search query"""
    text: str = Field(..., description="Search query text")
    top_k: int = Field(
        default=5,
        ge=1,
        le=20,
        description="Number of results to return"
    )
    score_threshold: Optional[float] = Field(
        default=0.7,
        ge=0,
        le=1,
        description="Minimum similarity score"
    )
    metadata_filter: Optional[Dict[str, Any]] = Field(
        None,
        description="Filter results by metadata"
    )
    file_types: Optional[List[str]] = Field(
        None,
        description="Filter by file types"
    )
    time_range: Optional[Dict[str, str]] = Field(
        None,
        description="Filter by time range (created_at or updated_at)"
    )
