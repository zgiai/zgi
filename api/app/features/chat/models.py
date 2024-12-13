from sqlalchemy import Column, Integer, String, DateTime, ForeignKey, JSON, Text, Boolean
from sqlalchemy.orm import relationship
from sqlalchemy.sql import func

from app.core.database import Base

class ChatSession(Base):
    """Chat session model for storing conversations"""
    __tablename__ = "chat_sessions"

    id = Column(Integer, primary_key=True, index=True)
    user_id = Column(Integer, ForeignKey("users.id", ondelete="CASCADE"), nullable=False)
    application_id = Column(Integer, ForeignKey("applications.id", ondelete="CASCADE"), nullable=True)
    model = Column(String(50), nullable=False)
    title = Column(String(255))
    messages = Column(JSON, default=list)  # List of message objects
    message_count = Column(Integer, default=0)  # Track number of messages
    total_tokens = Column(Integer, default=0)  # Track token usage
    is_archived = Column(Boolean, default=False)  # Soft delete support
    last_message_at = Column(DateTime)  # Track last message time
    created_at = Column(DateTime, server_default=func.now(), nullable=False)
    updated_at = Column(DateTime, server_default=func.now(), onupdate=func.now(), nullable=False)

    # Metadata for better organization
    tags = Column(JSON, default=list)  # Custom tags for filtering
    summary = Column(Text)  # Auto-generated summary of the conversation
    
    # Relationships
    user = relationship("User", back_populates="chat_sessions")
    application = relationship("Application", back_populates="chat_sessions")
    files = relationship("ChatFile", back_populates="session", cascade="all, delete-orphan")

class ChatFile(Base):
    """Chat file model for storing files associated with chat sessions"""
    __tablename__ = "chat_files"

    id = Column(Integer, primary_key=True, index=True)
    session_id = Column(Integer, ForeignKey("chat_sessions.id", ondelete="CASCADE"), nullable=False)
    filename = Column(String(255), nullable=False)
    file_type = Column(String(50))  # MIME type or file extension
    file_size = Column(Integer)  # Size in bytes
    file_path = Column(String(1000))  # Path to the stored file
    created_at = Column(DateTime, server_default=func.now(), nullable=False)
    updated_at = Column(DateTime, server_default=func.now(), onupdate=func.now(), nullable=False)

    # Relationships
    session = relationship("ChatSession", back_populates="files")
