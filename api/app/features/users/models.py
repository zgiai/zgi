from datetime import datetime
from sqlalchemy import Column, Integer, String, DateTime, Boolean, JSON
from sqlalchemy.orm import relationship
from sqlalchemy.sql import func

from app.core.database import Base

class User(Base):
    """User model for authentication and authorization"""
    __tablename__ = "users"
    __table_args__ = {'extend_existing': True}

    id = Column(Integer, primary_key=True, index=True)
    email = Column(String(255), unique=True, index=True, nullable=False)
    username = Column(String(255), unique=True, index=True, nullable=False)
    full_name = Column(String(255), nullable=True)
    hashed_password = Column(String(255), nullable=False)
    is_active = Column(Boolean, default=True, nullable=False)
    is_superuser = Column(Boolean, default=False, nullable=False)
    is_verified = Column(Boolean, default=False, nullable=False)
    is_admin = Column(Boolean, default=False, nullable=False)
    
    # 新增字段
    avatar_url = Column(String(500), nullable=True)
    bio = Column(String(500), nullable=True)
    phone = Column(String(20), nullable=True)
    location = Column(String(100), nullable=True)
    company = Column(String(100), nullable=True)
    website = Column(String(200), nullable=True)
    preferences = Column(JSON, nullable=True)
    last_login = Column(DateTime, nullable=True)
    login_count = Column(Integer, default=0, nullable=False)
    
    created_at = Column(DateTime, server_default=func.now(), nullable=False)
    updated_at = Column(DateTime, server_default=func.now(), onupdate=func.now(), nullable=False)

    # Relationships - 使用字符串引用来避免循环导入
    team_members = relationship("app.features.teams.models.TeamMember", back_populates="user", overlaps="teams")
    teams = relationship("app.features.teams.models.Team", secondary="team_members", back_populates="users", overlaps="team_members")
    api_keys = relationship("app.features.api_keys.models.APIKey", back_populates="user", cascade="all, delete-orphan")
    invitations_sent = relationship("app.features.teams.models.TeamInvitation", back_populates="inviter", foreign_keys="app.features.teams.models.TeamInvitation.inviter_id")
    applications = relationship("app.features.applications.models.Application", back_populates="owner", foreign_keys="app.features.applications.models.Application.owner_id")
    prompt_templates = relationship("app.features.prompts.models.PromptTemplate", back_populates="creator", foreign_keys="app.features.prompts.models.PromptTemplate.creator_id")
    prompt_scenarios = relationship("app.features.prompts.models.PromptScenario", back_populates="creator", foreign_keys="app.features.prompts.models.PromptScenario.created_by")
    chat_sessions = relationship("app.features.chat.models.ChatSession", back_populates="user", cascade="all, delete-orphan")

    # Team relationships
    teams = relationship("app.features.teams.models.Team", secondary="team_members", back_populates="users", overlaps="members,team_members,teams")
    team_members = relationship("app.features.teams.models.TeamMember", back_populates="user", overlaps="users,teams,team_members")
    owned_teams = relationship("app.features.teams.models.Team", back_populates="owner", foreign_keys="[Team.owner_id]")
    team_memberships = relationship("app.features.teams.models.TeamMember", back_populates="user")
