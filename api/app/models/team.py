from datetime import datetime
from sqlalchemy import Column, Integer, String, DateTime, Boolean, ForeignKey, JSON
from sqlalchemy.orm import relationship
from app.core.database import Base
from app.features.users.models import User

class Team(Base):
    __tablename__ = "teams"
    __table_args__ = {'extend_existing': True}

    id = Column(Integer, primary_key=True)
    name = Column(String(255), nullable=False)
    description = Column(String(1024))
    owner_id = Column(Integer, ForeignKey("users.id"), nullable=False)
    is_active = Column(Boolean, default=True)
    created_at = Column(DateTime, default=datetime.utcnow)
    updated_at = Column(DateTime, default=datetime.utcnow, onupdate=datetime.utcnow)
    settings = Column(JSON, nullable=True)

    # Relationships
    owner = relationship(User, back_populates="owned_teams")
    members = relationship("TeamMember", back_populates="team")
    invitations = relationship("TeamInvitation", back_populates="team")

class TeamMember(Base):
    __tablename__ = "team_members"
    __table_args__ = {'extend_existing': True}

    id = Column(Integer, primary_key=True)
    team_id = Column(Integer, ForeignKey("teams.id"), nullable=False)
    user_id = Column(Integer, ForeignKey("users.id"), nullable=False)
    role = Column(String(50), nullable=False)  # admin, member
    joined_at = Column(DateTime, default=datetime.utcnow)
    is_active = Column(Boolean, default=True)

    # Relationships
    team = relationship("Team", back_populates="members")
    user = relationship(User, back_populates="team_memberships")

class TeamInvitation(Base):
    __tablename__ = "team_invitations"
    __table_args__ = {'extend_existing': True}

    id = Column(Integer, primary_key=True)
    team_id = Column(Integer, ForeignKey("teams.id"), nullable=False)
    inviter_id = Column(Integer, ForeignKey("users.id"), nullable=False)
    invitee_email = Column(String(255), nullable=False)
    role = Column(String(50), nullable=False)  # admin, member
    token = Column(String(255), unique=True, nullable=False)
    expires_at = Column(DateTime, nullable=False)
    created_at = Column(DateTime, default=datetime.utcnow)
    accepted_at = Column(DateTime, nullable=True)
    is_active = Column(Boolean, default=True)

    # Relationships
    team = relationship("Team", back_populates="invitations")
    inviter = relationship(User, foreign_keys=[inviter_id])
