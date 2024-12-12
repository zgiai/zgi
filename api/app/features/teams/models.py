from sqlalchemy import Column, Integer, String, DateTime, Boolean, ForeignKey, Enum
from sqlalchemy.orm import relationship
from sqlalchemy.sql import func
import enum

from app.core.database import Base

class TeamRole(str, enum.Enum):
    OWNER = "owner"
    ADMIN = "admin"
    MEMBER = "member"

class Team(Base):
    """Team model"""
    __tablename__ = "teams"

    id = Column(Integer, primary_key=True, index=True)
    name = Column(String(255), nullable=False)
    description = Column(String(1000))
    created_by = Column(Integer, ForeignKey("users.id", ondelete="SET NULL"))
    is_active = Column(Boolean, default=True)
    created_at = Column(DateTime, server_default=func.now(), nullable=False)
    updated_at = Column(DateTime, server_default=func.now(), onupdate=func.now(), nullable=False)

    # Relationships
    members = relationship("TeamMember", back_populates="team", cascade="all, delete-orphan", overlaps="teams,users")
    users = relationship("User", secondary="team_members", back_populates="teams", overlaps="members,team_members,teams")
    invitations = relationship("TeamInvitation", back_populates="team", cascade="all, delete-orphan")
    creator = relationship("User", foreign_keys=[created_by])
    applications = relationship("app.features.applications.models.Application", back_populates="team")

class TeamMember(Base):
    """Team member model"""
    __tablename__ = "team_members"

    id = Column(Integer, primary_key=True, index=True)
    team_id = Column(Integer, ForeignKey("teams.id", ondelete="CASCADE"), nullable=False)
    user_id = Column(Integer, ForeignKey("users.id", ondelete="CASCADE"), nullable=False)
    role = Column(Enum(TeamRole), nullable=False, default=TeamRole.MEMBER)
    created_at = Column(DateTime, server_default=func.now(), nullable=False)
    updated_at = Column(DateTime, server_default=func.now(), onupdate=func.now(), nullable=False)

    # Relationships
    team = relationship("Team", back_populates="members", overlaps="users,teams,users")
    user = relationship("User", back_populates="team_members", overlaps="users,teams,team_members")

class TeamInvitation(Base):
    """Team invitation model"""
    __tablename__ = "team_invitations"

    id = Column(Integer, primary_key=True, index=True)
    team_id = Column(Integer, ForeignKey("teams.id", ondelete="CASCADE"), nullable=False)
    inviter_id = Column(Integer, ForeignKey("users.id", ondelete="SET NULL"))
    invitee_email = Column(String(255), nullable=False)
    role = Column(Enum(TeamRole), nullable=False, default=TeamRole.MEMBER)
    message = Column(String(1000))
    status = Column(String(20), default="pending")
    created_at = Column(DateTime, server_default=func.now(), nullable=False)
    expires_at = Column(DateTime, nullable=False)
    
    # Relationships
    team = relationship("Team", back_populates="invitations")
    inviter = relationship("User", foreign_keys=[inviter_id])
