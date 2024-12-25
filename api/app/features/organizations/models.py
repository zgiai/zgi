from sqlalchemy import Column, Integer, String, DateTime, Boolean, ForeignKey, Enum, Table
from sqlalchemy.orm import relationship
from sqlalchemy.sql import func
import enum
import uuid

from app.core.database import Base
from app.features.users.models import User

class OrganizationRole(str, enum.Enum):
    OWNER = "owner"
    ADMIN = "admin"
    MEMBER = "member"

def generate_uuid():
    return str(uuid.uuid4())

organization_member_roles = Table(
    'organization_member_roles',
    Base.metadata,
    Column('organization_member_id', Integer, ForeignKey('organization_members.id'), primary_key=True),
    Column('role_id', Integer, ForeignKey('roles.id'), primary_key=True)
)

class Organization(Base):
    """Organization model"""
    __tablename__ = "organizations"

    id = Column(Integer, primary_key=True, index=True, autoincrement=True)
    # uuid = Column(String(36), unique=True, index=True, default=generate_uuid, nullable=False)
    name = Column(String(255), nullable=False)
    description = Column(String(1000))
    created_by = Column(Integer, ForeignKey("users.id", ondelete="SET NULL"))
    is_active = Column(Boolean, default=True)
    created_at = Column(DateTime, server_default=func.now(), nullable=False)
    updated_at = Column(DateTime, server_default=func.now(), onupdate=func.now(), nullable=False)

    # Relationships
    members = relationship("OrganizationMember", back_populates="organization", cascade="all, delete-orphan")
    projects = relationship("Project", back_populates="organization", cascade="all, delete-orphan")
    roles = relationship("Role", back_populates="organization", cascade="all, delete-orphan")
    knowledge_bases = relationship("KnowledgeBase", back_populates="organization")

class OrganizationMember(Base):
    """Organization member model"""
    __tablename__ = "organization_members"

    id = Column(Integer, primary_key=True, index=True)
    organization_id = Column(Integer, ForeignKey("organizations.id", ondelete="CASCADE"), nullable=False)
    user_id = Column(Integer, ForeignKey("users.id", ondelete="CASCADE"), nullable=False)
    is_admin = Column(Boolean, default=False, nullable=False)
    created_at = Column(DateTime, server_default=func.now(), nullable=False)
    updated_at = Column(DateTime, server_default=func.now(), onupdate=func.now(), nullable=False)

    # Relationships
    organization = relationship("Organization", back_populates="members")
    user = relationship("User", back_populates="organization_members")
    roles = relationship("Role", secondary=organization_member_roles, back_populates="members")

class Role(Base):
    """Role model"""
    __tablename__ = "roles"

    id = Column(Integer, primary_key=True, index=True)
    organization_id = Column(Integer, ForeignKey("organizations.id", ondelete="CASCADE"), nullable=False)
    name = Column(String(255), nullable=False)
    description = Column(String(255))
    created_by = Column(Integer, ForeignKey("users.id", ondelete="SET NULL"))
    is_active = Column(Boolean, default=True)
    created_at = Column(DateTime, server_default=func.now(), nullable=False)
    updated_at = Column(DateTime, server_default=func.now(), onupdate=func.now(), nullable=False)

    # Relationships
    members = relationship("OrganizationMember", secondary=organization_member_roles, back_populates="roles")
    organization = relationship("Organization", back_populates="roles")
