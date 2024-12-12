from sqlalchemy import Column, Integer, String, DateTime
from sqlalchemy.sql import func

from app.core.database import Base

class IPWhitelist(Base):
    """IP whitelist model"""
    __tablename__ = "ip_whitelists"

    id = Column(Integer, primary_key=True, index=True)
    ip_address = Column(String(255), nullable=False)
    description = Column(String(1000))
    created_at = Column(DateTime, server_default=func.now(), nullable=False)
    updated_at = Column(DateTime, server_default=func.now(), onupdate=func.now(), nullable=False)
