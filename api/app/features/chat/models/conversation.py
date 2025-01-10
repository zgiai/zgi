# app/models/conversation.py
from sqlalchemy import Column, Integer, String, Text, ForeignKey, DateTime, func
from sqlalchemy.orm import relationship
from app.core.database import Base


class Conversation(Base):
    __tablename__ = "conversations"

    id = Column(Integer, primary_key=True, index=True)
    user_id = Column(Integer, ForeignKey("users.id"), nullable=True)
    session_id = Column(
        String(100), nullable=False, comment="Unique conversation identifier"
    )
    created_at = Column(DateTime, server_default=func.now())

    messages = relationship("ChatMessage", back_populates="conversation")
    user = relationship(
        "app.features.users.models.User", back_populates="conversations"
    )


class ChatMessage(Base):
    __tablename__ = "chat_messages"

    id = Column(Integer, primary_key=True, index=True)
    conversation_id = Column(Integer, ForeignKey("conversations.id"))
    role = Column(String(50), nullable=False, comment="conversation role")
    content = Column(Text)
    timestamp = Column(DateTime, server_default=func.now())

    conversation = relationship("Conversation", back_populates="messages")
