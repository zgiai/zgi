from sqlalchemy import Column, Integer, String, DateTime, JSON, ForeignKey, Text
from sqlalchemy.orm import relationship
from sqlalchemy.sql import func

from app.core.database import Base

class SecurityAuditLog(Base):
    """Security-related event logging"""
    __tablename__ = "security_audit_logs"
    __table_args__ = {'extend_existing': True}

    id = Column(Integer, primary_key=True, index=True)
    timestamp = Column(DateTime, server_default=func.now(), nullable=False, index=True)
    event_type = Column(String(50), nullable=False)
    user_id = Column(Integer, ForeignKey("users.id", ondelete="SET NULL"))
    api_key_id = Column(String(255))
    client_ip = Column(String(45))
    event_data = Column(JSON)
    description = Column(Text)

    # Relationships
    user = relationship("User", lazy="joined")
