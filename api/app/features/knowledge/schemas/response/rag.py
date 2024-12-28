from typing import List, Dict, Any, Optional
from pydantic import BaseModel, Field

class SearchResult(BaseModel):
    """Search result from vector database"""
    
    id: str = Field(..., description="Result ID")
    score: float = Field(..., description="Similarity score")
    metadata: Dict[str, Any] = Field(..., description="Result metadata")

class RAGResponse(BaseModel):
    """Response from RAG generation"""
    
    response: str = Field(..., description="Generated response")
    context: List[str] = Field(..., description="Retrieved context passages")
    search_results: List[SearchResult] = Field(..., description="Raw search results")
    
    # Optional debug information
    prompt: Optional[str] = Field(None, description="Generated prompt")
    tokens_used: Optional[int] = Field(None, description="Number of tokens used")
    processing_time: Optional[float] = Field(None, description="Processing time in seconds")
