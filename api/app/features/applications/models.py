from sqlalchemy import Column, Integer, String, DateTime, Boolean, ForeignKey
from sqlalchemy.orm import relationship
from sqlalchemy.sql import func

from app.core.database import Base
from app.features.usage.models import ResourceUsage

class Application(Base):
    """Application model"""
    __tablename__ = "applications"

    id = Column(Integer, primary_key=True, index=True)
    name = Column(String(255), nullable=False)
    description = Column(String(1000))
    owner_id = Column(Integer, ForeignKey("users.id", ondelete="CASCADE"), nullable=False)
    max_tokens = Column(Integer, default=1000)
    max_requests_per_day = Column(Integer, default=1000)
    is_active = Column(Boolean, default=True)
    created_at = Column(DateTime, server_default=func.now(), nullable=False)
    updated_at = Column(DateTime, server_default=func.now(), onupdate=func.now(), nullable=False)

    # Relationships
    owner = relationship("app.features.users.models.User")
    resource_usage = relationship(ResourceUsage, back_populates="application", cascade="all, delete-orphan")
    # chat_sessions = relationship("app.features.chat.models.ChatSession", back_populates="application", cascade="all, delete-orphan")
