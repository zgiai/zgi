from sqlalchemy import Column, Integer, String, DateTime, Boolean, JSON
from sqlalchemy.orm import relationship
from sqlalchemy.sql import func
from datetime import datetime

from app.core.database import Base
from app.features.chat.models import Conversation

class User(Base):
    """User model for authentication and authorization"""
    __tablename__ = "users"
    __table_args__ = {'extend_existing': True}

    id = Column(Integer, primary_key=True, index=True)
    email = Column(String(255), unique=True, index=True, nullable=False)
    username = Column(String(255), unique=True, index=True, nullable=False)
    full_name = Column(String(255), nullable=False)
    hashed_password = Column(String(255), nullable=False)
    is_active = Column(Boolean, default=True, nullable=False)
    is_superuser = Column(Boolean, default=False, nullable=False)
    user_type = Column(Integer, default=0, nullable=False)

    preferences = Column(JSON, nullable=True)
    created_at = Column(DateTime, nullable=False, default=datetime.utcnow)
    updated_at = Column(DateTime, nullable=False, default=datetime.utcnow, onupdate=datetime.utcnow)

    # Relationships
    organization_members = relationship("app.features.organizations.models.OrganizationMember", back_populates="user")
    applications = relationship("app.features.applications.models.Application", back_populates="owner")
    # chat_sessions = relationship("app.features.chat.models.ChatSession", back_populates="user", cascade="all, delete-orphan")
    created_projects = relationship("app.features.projects.models.Project", back_populates="creator")
    api_keys = relationship("app.features.api_keys.models.APIKey", back_populates="creator")
    knowledge_bases = relationship("app.models.knowledge_base.KnowledgeBase", back_populates="owner")
    conversations = relationship("app.features.chat.models.conversation.Conversation", back_populates="user")
