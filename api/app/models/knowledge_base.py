from datetime import datetime
from enum import Enum
from typing import Optional, List

from sqlalchemy import Column, Integer, String, Text, Enum as SQLEnum, ForeignKey, DateTime
from sqlalchemy.orm import relationship

from app.core.database import Base


class Visibility(str, Enum):
    PUBLIC = "public"
    PRIVATE = "private"


class KnowledgeBase(Base):
    __tablename__ = "knowledge_bases"

    id = Column(Integer, primary_key=True, index=True)
    name = Column(String(255), nullable=False)
    description = Column(Text, nullable=True)
    visibility = Column(SQLEnum(Visibility), default=Visibility.PRIVATE)
    owner_id = Column(Integer, ForeignKey("users.id"), nullable=False)
    created_at = Column(DateTime, default=datetime.utcnow)
    updated_at = Column(DateTime, default=datetime.utcnow, onupdate=datetime.utcnow)

    # Relationships
    documents = relationship("Document", back_populates="knowledge_base", cascade="all, delete-orphan")
    owner = relationship("User", back_populates="knowledge_bases")


class Document(Base):
    __tablename__ = "kb_documents"

    id = Column(Integer, primary_key=True, index=True)
    kb_id = Column(Integer, ForeignKey("knowledge_bases.id"), nullable=False)
    file_name = Column(String(255), nullable=False)
    file_content = Column(Text, nullable=True)
    vector_id = Column(String(255), nullable=True)  # ID in vector database
    created_at = Column(DateTime, default=datetime.utcnow)
    updated_at = Column(DateTime, default=datetime.utcnow, onupdate=datetime.utcnow)

    # Relationships
    knowledge_base = relationship("KnowledgeBase", back_populates="documents")
