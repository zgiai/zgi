from sqlalchemy import Column, Integer, String, DateTime, Boolean, ForeignKey, Enum
from sqlalchemy.orm import relationship
from sqlalchemy.sql import func
import enum
import uuid

from app.core.database import Base

class OrganizationRole(str, enum.Enum):
    OWNER = "owner"
    ADMIN = "admin"
    MEMBER = "member"

def generate_org_id():
    return str(uuid.uuid4())

class Organization(Base):
    """Organization model"""
    __tablename__ = "organizations"

    id = Column(String(36), primary_key=True, index=True, default=generate_org_id)
    name = Column(String(255), nullable=False)
    description = Column(String(1000))
    created_by = Column(Integer, ForeignKey("users.id", ondelete="SET NULL"))
    is_active = Column(Boolean, default=True)
    created_at = Column(DateTime, server_default=func.now(), nullable=False)
    updated_at = Column(DateTime, server_default=func.now(), onupdate=func.now(), nullable=False)

    # Relationships
    members = relationship("OrganizationMember", back_populates="organization", cascade="all, delete-orphan")

class OrganizationMember(Base):
    """Organization member model"""
    __tablename__ = "organization_members"

    id = Column(Integer, primary_key=True, index=True)
    organization_id = Column(String(36), ForeignKey("organizations.id", ondelete="CASCADE"), nullable=False)
    user_id = Column(Integer, ForeignKey("users.id", ondelete="CASCADE"), nullable=False)
    role = Column(Enum(OrganizationRole), nullable=False, default=OrganizationRole.MEMBER)
    created_at = Column(DateTime, server_default=func.now(), nullable=False)
    updated_at = Column(DateTime, server_default=func.now(), onupdate=func.now(), nullable=False)

    # Relationships
    organization = relationship("Organization", back_populates="members")
    user = relationship("app.features.users.models.User", back_populates="organization_members")
