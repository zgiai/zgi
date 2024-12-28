from datetime import datetime
from typing import Dict, Any
from sqlalchemy import Column, Integer, String, Text, DateTime, Enum, JSON, ForeignKey
from sqlalchemy.orm import relationship
from app.core.database import Base
from app.features.knowledge.models.document import Document
from app.features.organizations.models import Organization

import enum

class Visibility(str, enum.Enum):
    """Knowledge base visibility"""
    PRIVATE = "PRIVATE"
    PUBLIC = "PUBLIC"
    ORGANIZATION = "ORGANIZATION"

class Status(int, enum.Enum):
    """Knowledge base status"""
    ACTIVE = 1
    DISABLED = 0
    DELETED = -1

class KnowledgeBase(Base):
    """Knowledge base model"""
    __tablename__ = "knowledge_bases"

    id = Column(Integer, primary_key=True, index=True)
    name = Column(String(255), nullable=False)
    description = Column(Text, nullable=True)
    visibility = Column(Enum(Visibility), default=Visibility.PRIVATE)
    status = Column(Integer, default=Status.ACTIVE)
    
    # Vector database settings
    collection_name = Column(String(255), unique=True, nullable=False)
    model = Column(String(255), nullable=False, default="text-embedding-3-small")
    dimension = Column(Integer, nullable=False, default=1536)
    
    # Statistics
    document_count = Column(Integer, default=0)
    total_chunks = Column(Integer, default=0)
    total_tokens = Column(Integer, default=0)
    
    # Metadata
    meta_info = Column(JSON, nullable=True)
    tags = Column(JSON, nullable=True)
    
    # Ownership and timestamps
    owner_id = Column(Integer, ForeignKey("users.id"), nullable=False)
    organization_id = Column(Integer, ForeignKey("organizations.id"), nullable=True)
    created_at = Column(DateTime, default=datetime.utcnow)
    updated_at = Column(DateTime, default=datetime.utcnow, onupdate=datetime.utcnow)
    
    # Relationships
    owner = relationship("app.features.users.models.User", back_populates="knowledge_bases")
    documents = relationship("Document", back_populates="knowledge_base", cascade="all, delete-orphan")

    def __repr__(self):
        return f"<KnowledgeBase {self.name}>"

    def to_dict(self) -> Dict[str, Any]:
        """Convert to dictionary"""
        return {
            "id": self.id,
            "name": self.name,
            "description": self.description,
            "visibility": self.visibility,
            "status": self.status,
            "collection_name": self.collection_name,
            "model": self.model,
            "dimension": self.dimension,
            "document_count": self.document_count,
            "total_chunks": self.total_chunks,
            "total_tokens": self.total_tokens,
            "meta_info": self.meta_info,
            "tags": self.tags,
            "owner_id": self.owner_id,
            "organization_id": self.organization_id,
            "created_at": self.created_at.isoformat(),
            "updated_at": self.updated_at.isoformat()
        }
