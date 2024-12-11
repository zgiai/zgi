from datetime import datetime
from sqlalchemy import Boolean, Column, Integer, String, DateTime, ForeignKey, Text, UniqueConstraint
from sqlalchemy.orm import relationship
from sqlalchemy.sql import func

from app.core.database import Base

class User(Base):
    __tablename__ = "users"

    id = Column(Integer, primary_key=True, index=True)
    email = Column(String(255), unique=True, index=True, nullable=False)
    username = Column(String(50), unique=True, index=True, nullable=False)
    hashed_password = Column(String(255), nullable=False)
    is_active = Column(Boolean, default=True, nullable=False)
    is_superadmin = Column(Boolean, default=False, nullable=False)
    created_at = Column(DateTime, server_default=func.now(), nullable=False)
    updated_at = Column(DateTime, server_default=func.now(), onupdate=func.now(), nullable=False)

    # Relationships
    team_members = relationship("TeamMember", back_populates="user", overlaps="teams,members")
    teams = relationship("Team", secondary="team_members", back_populates="members", overlaps="team_members")
    api_keys = relationship("APIKey", back_populates="user", cascade="all, delete-orphan")
    invitations_sent = relationship("TeamInvitation", back_populates="inviter", foreign_keys="TeamInvitation.inviter_id")
    applications = relationship("Application", back_populates="creator", foreign_keys="Application.created_by")
    prompt_templates = relationship("PromptTemplate", back_populates="creator", foreign_keys="PromptTemplate.created_by")
    prompt_scenarios = relationship("PromptScenario", back_populates="creator", foreign_keys="PromptScenario.created_by")

class Team(Base):
    __tablename__ = "teams"

    id = Column(Integer, primary_key=True, index=True)
    name = Column(String(100), nullable=False)
    description = Column(String(500))
    max_members = Column(Integer, default=5)
    allow_member_invite = Column(Boolean, default=False)
    default_member_role = Column(String(50), default="member")
    isolated_data = Column(Boolean, default=True)
    shared_api_keys = Column(Boolean, default=False)
    owner_id = Column(Integer, ForeignKey("users.id", ondelete="CASCADE"), nullable=False)
    created_at = Column(DateTime, server_default=func.now(), nullable=False)
    updated_at = Column(DateTime, server_default=func.now(), onupdate=func.now(), nullable=False)

    # Relationships
    team_members = relationship("TeamMember", back_populates="team", overlaps="members,teams")
    members = relationship("User", secondary="team_members", back_populates="teams", overlaps="team_members")
    api_keys = relationship("APIKey", back_populates="team", cascade="all, delete-orphan")
    invitations = relationship("TeamInvitation", back_populates="team", cascade="all, delete-orphan")
    applications = relationship("Application", back_populates="team", cascade="all, delete-orphan")

class TeamMember(Base):
    __tablename__ = "team_members"

    id = Column(Integer, primary_key=True, index=True)
    team_id = Column(Integer, ForeignKey("teams.id", ondelete="CASCADE"), nullable=False)
    user_id = Column(Integer, ForeignKey("users.id", ondelete="CASCADE"), nullable=False)
    role = Column(String(50), nullable=False)
    created_at = Column(DateTime, server_default=func.now(), nullable=False)
    updated_at = Column(DateTime, server_default=func.now(), onupdate=func.now(), nullable=False)

    # Relationships
    team = relationship("Team", back_populates="team_members", overlaps="members,teams")
    user = relationship("User", back_populates="team_members", overlaps="members,teams")

    __table_args__ = (
        UniqueConstraint('team_id', 'user_id', name='_team_member_uc'),
    )

class TeamInvitation(Base):
    __tablename__ = "team_invitations"

    id = Column(Integer, primary_key=True, index=True)
    team_id = Column(Integer, ForeignKey("teams.id", ondelete="CASCADE"), nullable=False)
    inviter_id = Column(Integer, ForeignKey("users.id", ondelete="CASCADE"), nullable=False)
    invitee_email = Column(String(255), nullable=False)
    role = Column(String(50), nullable=False, default="member")
    status = Column(String(20), nullable=False, default="pending")
    created_at = Column(DateTime, server_default=func.now(), nullable=False)
    expires_at = Column(DateTime, nullable=False)

    # Relationships
    team = relationship("Team", back_populates="invitations")
    inviter = relationship("User", back_populates="invitations_sent")

class APIKey(Base):
    __tablename__ = "api_keys"

    id = Column(Integer, primary_key=True, index=True)
    user_id = Column(Integer, ForeignKey("users.id", ondelete="CASCADE"), nullable=False)
    team_id = Column(Integer, ForeignKey("teams.id", ondelete="CASCADE"))
    provider = Column(String(50), nullable=False)
    key_name = Column(String(100), nullable=False)
    key_value = Column(String(255), nullable=False)
    is_active = Column(Boolean, default=True, nullable=False)
    last_used_at = Column(DateTime)
    created_at = Column(DateTime, server_default=func.now(), nullable=False)
    updated_at = Column(DateTime, server_default=func.now(), onupdate=func.now(), nullable=False)

    # Relationships
    user = relationship("User", back_populates="api_keys")
    team = relationship("Team", back_populates="api_keys")

class Application(Base):
    __tablename__ = "applications"

    id = Column(Integer, primary_key=True, index=True, autoincrement=True)
    name = Column(String(255), nullable=False, index=True)
    description = Column(String(1000))
    type = Column(String(50), nullable=False)  # conversational, generative, function, workflow
    access_level = Column(String(20), nullable=False, default="private")  # private, team, public
    team_id = Column(Integer, ForeignKey("teams.id", ondelete="SET NULL"), index=True)
    created_by = Column(Integer, ForeignKey("users.id", ondelete="CASCADE"), nullable=False, index=True)
    is_active = Column(Boolean, default=True, nullable=False)
    created_at = Column(DateTime, server_default=func.now(), nullable=False)
    updated_at = Column(DateTime, server_default=func.now(), onupdate=func.now(), nullable=False)

    # Relationships
    creator = relationship("User", back_populates="applications")
    team = relationship("Team", back_populates="applications")
    prompt_templates = relationship("PromptTemplate", back_populates="application", cascade="all, delete-orphan")

class PromptTemplate(Base):
    __tablename__ = "prompt_templates"

    id = Column(Integer, primary_key=True, index=True)
    name = Column(String(255), nullable=False, index=True)
    description = Column(String(1000))
    prompt = Column(Text, nullable=False)
    created_by = Column(Integer, ForeignKey("users.id", ondelete="CASCADE"), nullable=False, index=True)
    application_id = Column(Integer, ForeignKey("applications.id", ondelete="CASCADE"), nullable=False, index=True)
    created_at = Column(DateTime, server_default=func.now(), nullable=False)
    updated_at = Column(DateTime, server_default=func.now(), onupdate=func.now(), nullable=False)

    # Relationships
    creator = relationship("User", back_populates="prompt_templates")
    application = relationship("Application", back_populates="prompt_templates")

class PromptScenario(Base):
    __tablename__ = "prompt_scenarios"

    id = Column(Integer, primary_key=True, index=True)
    name = Column(String(255), nullable=False, index=True)
    description = Column(String(1000))
    scenario = Column(Text, nullable=False)
    created_by = Column(Integer, ForeignKey("users.id", ondelete="CASCADE"), nullable=False, index=True)
    created_at = Column(DateTime, server_default=func.now(), nullable=False)
    updated_at = Column(DateTime, server_default=func.now(), onupdate=func.now(), nullable=False)

    # Relationships
    creator = relationship("User", back_populates="prompt_scenarios")
