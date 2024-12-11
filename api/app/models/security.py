from sqlalchemy import Column, Integer, String, DateTime, Boolean, JSON, ForeignKey, Text
from sqlalchemy.orm import relationship
from sqlalchemy.sql import func

from app.core.database import Base

class APIKey(Base):
    """API key model with encrypted storage"""
    __tablename__ = "api_keys"

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

class IPWhitelist(Base):
    """IP whitelist for API keys"""
    __tablename__ = "ip_whitelists"

    id = Column(Integer, primary_key=True, index=True)
    api_key_id = Column(Integer, ForeignKey("api_keys.id", ondelete="CASCADE"), nullable=False, unique=True)
    allowed_ips = Column(JSON, nullable=False)  # List of allowed IP addresses/ranges
    is_active = Column(Boolean, default=True)
    created_at = Column(DateTime, server_default=func.now(), nullable=False)
    updated_at = Column(DateTime, server_default=func.now(), onupdate=func.now(), nullable=False)

    # Relationships
    api_key = relationship("APIKey", back_populates="ip_whitelist")

class APILog(Base):
    """API call logging"""
    __tablename__ = "api_logs"

    id = Column(Integer, primary_key=True, index=True)
    request_id = Column(String(36), nullable=False, index=True)  # UUID
    timestamp = Column(DateTime, nullable=False, index=True)
    method = Column(String(10), nullable=False)  # GET, POST, etc.
    path = Column(String(255), nullable=False)
    client_ip = Column(String(45), nullable=False)  # Support for IPv6
    request_data = Column(Text)  # JSON string of request data
    response_data = Column(Text)  # JSON string of response data
    status_code = Column(Integer)
    api_key_id = Column(String(255), index=True)
    error_message = Column(Text)
    duration_ms = Column(Integer)  # Request duration in milliseconds

class SecurityAuditLog(Base):
    """Security-related event logging"""
    __tablename__ = "security_audit_logs"

    id = Column(Integer, primary_key=True, index=True)
    timestamp = Column(DateTime, server_default=func.now(), nullable=False, index=True)
    event_type = Column(String(50), nullable=False)  # login, logout, key_creation, etc.
    user_id = Column(Integer, ForeignKey("users.id", ondelete="SET NULL"))
    api_key_id = Column(String(255))
    client_ip = Column(String(45))
    event_data = Column(JSON)  # Additional event details
    description = Column(Text)

    # Relationships
    user = relationship("User")
