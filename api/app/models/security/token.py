from sqlalchemy import Column, Integer, String, DateTime, ForeignKey
from sqlalchemy.orm import relationship
from sqlalchemy.sql import func

from app.core.database import Base

class Token(Base):
    """Token model for storing refresh tokens"""
    __tablename__ = "tokens"
    __table_args__ = {'extend_existing': True}

    id = Column(Integer, primary_key=True, index=True)
    token = Column(String(255), unique=True, index=True)
    token_type = Column(String(50))
    user_id = Column(Integer, ForeignKey("users.id", ondelete="CASCADE"))
    expires_at = Column(DateTime)
    created_at = Column(DateTime, server_default=func.now())
    updated_at = Column(DateTime, server_default=func.now(), onupdate=func.now())

    # Relationships
    user = relationship("User", lazy="joined")
