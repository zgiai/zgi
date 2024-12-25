from sqlalchemy import Column, Integer, String, DateTime, Boolean, JSON
from sqlalchemy.orm import relationship
from sqlalchemy.sql import func
from datetime import datetime
from app.core.database import Base
from app.features.applications.models import Application
from app.features.projects.models import Project
from app.features.api_keys.models import APIKey

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
    
    preferences = Column(JSON, nullable=True)
    created_at = Column(DateTime, nullable=False, default=datetime.utcnow)
    updated_at = Column(DateTime, nullable=False, default=datetime.utcnow, onupdate=datetime.utcnow)

    # Relationships
    organization_members = relationship("OrganizationMember", back_populates="user")
    created_projects = relationship("Project", back_populates="creator")
    api_keys = relationship("APIKey", back_populates="creator")
    knowledge_bases = relationship("KnowledgeBase", back_populates="owner")
