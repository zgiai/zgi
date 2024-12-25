from typing import Optional, Dict, Any
from pydantic import BaseModel, Field

class RAGConfig(BaseModel):
    """Configuration for RAG"""
    
    # Retrieval settings
    top_k: int = Field(5, description="Number of documents to retrieve")
    score_threshold: float = Field(0.7, description="Minimum similarity score")
    
    # Generation settings
    max_tokens: int = Field(500, description="Maximum tokens in response")
    temperature: float = Field(0.7, description="Temperature for generation")
    prompt_template: Optional[str] = Field(
        None,
        description="Custom prompt template. Use {query} and {context} placeholders"
    )
    
    # Advanced settings
    use_reranking: bool = Field(False, description="Whether to use cross-encoder reranking")
    use_query_expansion: bool = Field(False, description="Whether to use query expansion")
    chunk_overlap: int = Field(200, description="Overlap between context chunks")
    max_context_length: int = Field(2000, description="Maximum context length in tokens")

class RAGRequest(BaseModel):
    """Request for RAG generation"""
    
    query: str = Field(..., description="User query")
    metadata_filter: Optional[Dict[str, Any]] = Field(
        None,
        description="Filter for document metadata"
    )
    config: Optional[RAGConfig] = Field(
        None,
        description="RAG configuration"
    )
