from sqlalchemy import Column, Integer, String, DateTime, ForeignKey, Float
from sqlalchemy.sql import func

from app.core.database import Base

class ResourceUsage(Base):
    __tablename__ = "resource_usage"

    id = Column(Integer, primary_key=True, index=True)
    application_id = Column(Integer, ForeignKey("applications.id", ondelete="CASCADE"), nullable=False, index=True)
    resource_type = Column(String(50), nullable=False)  # 'token', 'api_call'
    quantity = Column(Float, nullable=False)  # token count or number of calls
    endpoint = Column(String(255))  # API endpoint for api_calls
    model = Column(String(100))  # Model name for token usage
    timestamp = Column(DateTime, server_default=func.now(), nullable=False, index=True)
