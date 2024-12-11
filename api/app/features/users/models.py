from datetime import datetime
from sqlalchemy import Column, Integer, String, DateTime, Boolean
from sqlalchemy.orm import relationship
from sqlalchemy.sql import func

from app.core.database import Base
from app.models.security.api_key import APIKey

__all__ = ['User']

class User(Base):
    """User model for authentication and authorization"""
    __tablename__ = "users"
    __table_args__ = {'extend_existing': True}

    id = Column(Integer, primary_key=True, index=True)
    email = Column(String(255), unique=True, index=True, nullable=False)
    username = Column(String(255), unique=True, index=True, nullable=False)
    hashed_password = Column(String(255), nullable=False)
    is_active = Column(Boolean, default=True, nullable=False)
    is_superuser = Column(Boolean, default=False, nullable=False)
    created_at = Column(DateTime, server_default=func.now(), nullable=False)
    updated_at = Column(DateTime, server_default=func.now(), onupdate=func.now(), nullable=False)

    # Relationships
    team_members = relationship("TeamMember", back_populates="user", overlaps="teams")
    teams = relationship("Team", secondary="team_members", back_populates="users", overlaps="team_members")
    api_keys = relationship("app.models.security.api_key.APIKey", back_populates="user", cascade="all, delete-orphan")
    invitations_sent = relationship("TeamInvitation", back_populates="inviter", foreign_keys="TeamInvitation.created_by")
    applications = relationship("Application", back_populates="owner", foreign_keys="Application.owner_id")
    prompt_templates = relationship("PromptTemplate", back_populates="creator", foreign_keys="PromptTemplate.created_by")
    prompt_scenarios = relationship("PromptScenario", back_populates="creator", foreign_keys="PromptScenario.created_by")
    chat_sessions = relationship("ChatSession", back_populates="user", cascade="all, delete-orphan")
