from sqlalchemy import Column, Integer, String, DateTime, ForeignKey, JSON, Text, BigInteger
from sqlalchemy.orm import relationship
from sqlalchemy.sql import func

from app.core.database import Base

class ChatFile(Base):
    """Model for storing chat-related files"""
    __tablename__ = "chat_files"

    id = Column(Integer, primary_key=True, index=True)
    user_id = Column(Integer, ForeignKey("users.id", ondelete="CASCADE"), nullable=False)
    session_id = Column(Integer, ForeignKey("chat_sessions.id", ondelete="CASCADE"), nullable=False)
    filename = Column(String(255), nullable=False)
    file_path = Column(String(512), nullable=False)
    file_size = Column(BigInteger, nullable=False)  # in bytes
    mime_type = Column(String(128), nullable=False)
    content_hash = Column(String(64), nullable=False)  # SHA-256 hash
    extracted_text = Column(Text)  # Extracted text content
    metadata = Column(JSON, default=dict)  # Additional metadata (pages, title, etc.)
    created_at = Column(DateTime, server_default=func.now(), nullable=False)
    updated_at = Column(DateTime, server_default=func.now(), onupdate=func.now(), nullable=False)

    # Relationships
    user = relationship("User", back_populates="chat_files")
    session = relationship("ChatSession", back_populates="files")
