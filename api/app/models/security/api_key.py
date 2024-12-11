from sqlalchemy import Column, Integer, String, DateTime, Boolean, ForeignKey
from sqlalchemy.orm import relationship
from sqlalchemy.sql import func

from app.core.database import Base

class APIKey(Base):
    """API key model with encrypted storage"""
    __tablename__ = "api_keys"
    __table_args__ = {'extend_existing': True}

    id = Column(Integer, primary_key=True, index=True)
    key_id = Column(String(255), unique=True, nullable=False, index=True)  # Public identifier
    hashed_key = Column(String(255), nullable=False)  # Hashed API key
    name = Column(String(255), nullable=False)
    user_id = Column(Integer, ForeignKey("users.id", ondelete="CASCADE"), nullable=False)
    application_id = Column(Integer, ForeignKey("applications.id", ondelete="CASCADE"), nullable=False)
    is_active = Column(Boolean, default=True)
    expires_at = Column(DateTime)
    created_at = Column(DateTime, server_default=func.now(), nullable=False)
    last_used_at = Column(DateTime)

    # Relationships
    user = relationship("User", back_populates="api_keys")
    application = relationship("Application", back_populates="api_keys")
    ip_whitelist = relationship("IPWhitelist", back_populates="api_key", uselist=False)
