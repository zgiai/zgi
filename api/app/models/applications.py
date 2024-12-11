from sqlalchemy import Column, Integer, String, DateTime, Boolean, ForeignKey
from sqlalchemy.orm import relationship
from sqlalchemy.sql import func

from app.core.database import Base

class Application(Base):
    """Application model"""
    __tablename__ = "applications"

    id = Column(Integer, primary_key=True, index=True)
    name = Column(String(255), nullable=False)
    description = Column(String(1000))
    owner_id = Column(Integer, ForeignKey("users.id", ondelete="CASCADE"), nullable=False)
    is_active = Column(Boolean, default=True)
    created_at = Column(DateTime, server_default=func.now(), nullable=False)
    updated_at = Column(DateTime, server_default=func.now(), onupdate=func.now(), nullable=False)

    # Relationships
    owner = relationship("User", back_populates="applications")
    api_keys = relationship("APIKey", back_populates="application", cascade="all, delete-orphan")
    resource_usage = relationship("ResourceUsage", back_populates="application", cascade="all, delete-orphan")
    prompt_templates = relationship("PromptTemplate", back_populates="application", cascade="all, delete-orphan")
