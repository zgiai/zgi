from sqlalchemy import Column, Integer, String, DateTime, Text
from sqlalchemy.sql import func

from app.core.database import Base

class APILog(Base):
    """API call logging"""
    __tablename__ = "api_logs"
    __table_args__ = {'extend_existing': True}

    id = Column(Integer, primary_key=True, index=True)
    request_id = Column(String(36), nullable=False, index=True)  # UUID
    timestamp = Column(DateTime, nullable=False, index=True)
    method = Column(String(10), nullable=False)  # GET, POST, etc.
    path = Column(String(255), nullable=False)
    client_ip = Column(String(45), nullable=False)  # Support for IPv6
    request_data = Column(Text)  # JSON string of request data
