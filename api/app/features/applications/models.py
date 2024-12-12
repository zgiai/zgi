from datetime import datetime
from sqlalchemy import Column, Integer, String, DateTime, Boolean, ForeignKey, Enum, JSON
from sqlalchemy.orm import relationship
from sqlalchemy.sql import func
import enum

from app.core.database import Base

class ApplicationType(str, enum.Enum):
    """应用类型枚举"""
    CHAT = "chat"
    COMPLETION = "completion"
    EMBEDDING = "embedding"
    IMAGE = "image"
    AUDIO = "audio"
    CUSTOM = "custom"

class Application(Base):
    """应用模型"""
    __tablename__ = "applications"
    __table_args__ = {'extend_existing': True}

    id = Column(Integer, primary_key=True, index=True)
    name = Column(String(255), nullable=False)
    description = Column(String(1000))
    type = Column(Enum(ApplicationType), nullable=False, default=ApplicationType.CHAT)
    owner_id = Column(Integer, ForeignKey("users.id", ondelete="CASCADE"), nullable=False)
    team_id = Column(Integer, ForeignKey("teams.id", ondelete="SET NULL"))
    is_active = Column(Boolean, default=True)
    config = Column(JSON)
    created_at = Column(DateTime, server_default=func.now(), nullable=False)
    updated_at = Column(DateTime, server_default=func.now(), onupdate=func.now(), nullable=False)

    # Relationships
    owner = relationship("app.features.users.models.User", foreign_keys=[owner_id], back_populates="applications")
    team = relationship("app.features.teams.models.Team", back_populates="applications")
