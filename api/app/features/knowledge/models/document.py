from datetime import datetime
from typing import Dict, Any
from sqlalchemy import Column, Integer, String, Text, DateTime, JSON, ForeignKey, Index
from sqlalchemy.orm import relationship
from app.core.database import Base

import enum

class DocumentStatus(int, enum.Enum):
    """Document status"""
    PENDING = 0
    PROCESSING = 1
    COMPLETED = 2
    FAILED = -1
    DELETED = -2

class Document(Base):
    """Document model"""
    __tablename__ = "knowledge_documents"

    id = Column(Integer, primary_key=True, index=True)
    kb_id = Column(Integer, ForeignKey("knowledge_bases.id"), nullable=False)
    
    # File information
    file_name = Column(String(255), nullable=False)
    file_path = Column(String(512), nullable=False)
    file_type = Column(String(50), nullable=False)
    file_size = Column(Integer, nullable=False)
    file_hash = Column(String(64), nullable=False)
    
    # Processing information
    status = Column(Integer, default=DocumentStatus.PENDING)
    error_message = Column(Text, nullable=True)
    chunk_count = Column(Integer, default=0)
    token_count = Column(Integer, default=0)
    vector_count = Column(Integer, default=0)
    
    # Document metadata
    title = Column(String(255), nullable=True)
    language = Column(String(10), nullable=True)
    author = Column(String(255), nullable=True)
    source_url = Column(String(512), nullable=True)
    meta_info = Column(JSON, nullable=True)
    tags = Column(JSON, nullable=True)
    
    # Processing settings
    chunk_size = Column(Integer, default=1000)
    chunk_overlap = Column(Integer, default=200)
    embedding_model = Column(String(255), nullable=True)
    
    # Timestamps
    created_at = Column(DateTime, default=datetime.utcnow)
    updated_at = Column(DateTime, default=datetime.utcnow, onupdate=datetime.utcnow)
    processed_at = Column(DateTime, nullable=True)
    
    # Relationships
    knowledge_base = relationship("KnowledgeBase", back_populates="documents")
    chunks = relationship("DocumentChunk", back_populates="document", cascade="all, delete-orphan")

    # Indexes
    __table_args__ = (
        Index("idx_kb_status", kb_id, status),
        Index("idx_file_hash", file_hash),
    )

    def __repr__(self):
        return f"<Document {self.file_name}>"

    def to_dict(self) -> Dict[str, Any]:
        """Convert to dictionary"""
        return {
            "id": self.id,
            "kb_id": self.kb_id,
            "file_name": self.file_name,
            "file_type": self.file_type,
            "file_size": self.file_size,
            "status": self.status,
            "chunk_count": self.chunk_count,
            "token_count": self.token_count,
            "vector_count": self.vector_count,
            "title": self.title,
            "language": self.language,
            "author": self.author,
            "source_url": self.source_url,
            "meta_info": self.meta_info,
            "tags": self.tags,
            "created_at": self.created_at.isoformat(),
            "updated_at": self.updated_at.isoformat(),
            "processed_at": self.processed_at.isoformat() if self.processed_at else None
        }

class DocumentChunk(Base):
    """Document chunk model"""
    __tablename__ = "knowledge_document_chunks"

    id = Column(Integer, primary_key=True, index=True)
    document_id = Column(Integer, ForeignKey("knowledge_documents.id"), nullable=False)
    
    # Chunk information
    chunk_index = Column(Integer, nullable=False)
    content = Column(Text, nullable=False)
    token_count = Column(Integer, default=0)
    
    # Vector information
    vector_id = Column(String(255), nullable=True)
    embedding_model = Column(String(255), nullable=True)
    
    # Metadata
    chunk_meta_info = Column(JSON, nullable=True)
    
    # Timestamps
    created_at = Column(DateTime, default=datetime.utcnow)
    updated_at = Column(DateTime, default=datetime.utcnow, onupdate=datetime.utcnow)
    
    # Relationships
    document = relationship("Document", back_populates="chunks")

    # Indexes
    __table_args__ = (
        Index("idx_document_chunk", document_id, chunk_index),
    )

    def __repr__(self):
        return f"<DocumentChunk {self.document_id}:{self.chunk_index}>"
