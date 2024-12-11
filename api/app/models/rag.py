from datetime import datetime
from sqlalchemy import Column, Integer, String, DateTime, ForeignKey, Float, Text, JSON, Table
from sqlalchemy.orm import relationship

from app.core.database import Base

class Document(Base):
    """Model for storing uploaded documents"""
    __tablename__ = "documents"

    id = Column(Integer, primary_key=True, index=True)
    user_id = Column(Integer, ForeignKey("users.id", ondelete="CASCADE"), nullable=False)
    team_id = Column(Integer, ForeignKey("teams.id", ondelete="CASCADE"), nullable=True)
    
    filename = Column(String(255), nullable=False)
    file_path = Column(String(512), nullable=False)
    file_type = Column(String(50), nullable=False)  # pdf, txt, etc.
    file_size = Column(Integer, nullable=False)  # in bytes
    
    content_hash = Column(String(64), nullable=False)  # SHA-256 hash
    extracted_text = Column(Text)  # Raw extracted text
    metadata = Column(JSON, default=dict)  # Additional metadata (pages, title, etc.)
    
    # Vector storage info
    vector_ids = Column(JSON, default=list)  # List of vector IDs in the vector DB
    chunk_count = Column(Integer, default=0)  # Number of text chunks
    embedding_model = Column(String(100))  # Model used for embeddings
    
    status = Column(String(50), default="pending")  # pending, processing, completed, failed
    error = Column(Text)  # Error message if processing failed
    
    created_at = Column(DateTime, default=datetime.utcnow)
    updated_at = Column(DateTime, default=datetime.utcnow, onupdate=datetime.utcnow)
    processed_at = Column(DateTime)  # When document processing completed

    # Relationships
    user = relationship("User", back_populates="documents")
    team = relationship("Team", back_populates="documents")
    queries = relationship("QueryLog", back_populates="document")

class QueryLog(Base):
    """Model for tracking RAG queries and results"""
    __tablename__ = "query_logs"

    id = Column(Integer, primary_key=True, index=True)
    user_id = Column(Integer, ForeignKey("users.id", ondelete="CASCADE"), nullable=False)
    document_id = Column(Integer, ForeignKey("documents.id", ondelete="CASCADE"), nullable=True)
    
    query_text = Column(Text, nullable=False)
    retrieved_chunks = Column(JSON)  # List of retrieved chunk IDs and scores
    response_text = Column(Text)  # Generated response
    
    tokens_used = Column(Integer, default=0)
    duration_ms = Column(Integer)  # Total processing time in milliseconds
    
    metadata = Column(JSON, default=dict)  # Additional query metadata
    created_at = Column(DateTime, default=datetime.utcnow)

    # Relationships
    user = relationship("User", back_populates="rag_queries")
    document = relationship("Document", back_populates="queries")
