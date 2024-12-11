from sqlalchemy import Column, Integer, JSON, DateTime, Boolean, ForeignKey
from sqlalchemy.orm import relationship
from sqlalchemy.sql import func

from app.core.database import Base

class IPWhitelist(Base):
    """IP whitelist for API keys"""
    __tablename__ = "ip_whitelists"
    __table_args__ = {'extend_existing': True}

    id = Column(Integer, primary_key=True, index=True)
    api_key_id = Column(Integer, ForeignKey("api_keys.id", ondelete="CASCADE"), nullable=False, unique=True)
    allowed_ips = Column(JSON, nullable=False)  # List of allowed IP addresses/ranges
    is_active = Column(Boolean, default=True)
    created_at = Column(DateTime, server_default=func.now(), nullable=False)
    updated_at = Column(DateTime, server_default=func.now(), onupdate=func.now(), nullable=False)

    # Relationships
    api_key = relationship("APIKey", back_populates="ip_whitelist")
