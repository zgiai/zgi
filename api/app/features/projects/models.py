from sqlalchemy import Column, Integer, String, DateTime, Boolean, ForeignKey, Enum
from sqlalchemy.orm import relationship
from sqlalchemy.sql import func
import enum
import uuid

from app.core.database import Base

def generate_uuid():
    return str(uuid.uuid4())

class ProjectStatus(str, enum.Enum):
    ACTIVE = "active"
    ARCHIVED = "archived"
    DELETED = "deleted"

class Project(Base):
    """Project model"""
    __tablename__ = "projects"

    id = Column(Integer, primary_key=True, index=True, autoincrement=True)
    uuid = Column(String(36), unique=True, index=True, default=generate_uuid, nullable=False)
    name = Column(String(255), nullable=False)
    description = Column(String(1000))
    organization_id = Column(Integer, ForeignKey("organizations.id", ondelete="CASCADE"), nullable=False)
    created_by = Column(Integer, ForeignKey("users.id", ondelete="SET NULL"))
    status = Column(Enum(ProjectStatus), nullable=False, default=ProjectStatus.ACTIVE)
    created_at = Column(DateTime, server_default=func.now(), nullable=False)
    updated_at = Column(DateTime, server_default=func.now(), onupdate=func.now(), nullable=False)

    # Relationships
    organization = relationship("app.features.organizations.models.Organization", back_populates="projects")
    creator = relationship("app.features.users.models.User", back_populates="created_projects")
    api_keys = relationship("app.features.api_keys.models.APIKey", back_populates="project", cascade="all, delete-orphan")
