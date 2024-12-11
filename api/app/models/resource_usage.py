from sqlalchemy import Column, Integer, String, DateTime, Float, ForeignKey
from sqlalchemy.orm import relationship
from sqlalchemy.sql import func

from app.core.database import Base

class ResourceUsage(Base):
    """Resource Usage model"""
    __tablename__ = "resource_usage"

    id = Column(Integer, primary_key=True, index=True)
    application_id = Column(Integer, ForeignKey("applications.id", ondelete="CASCADE"), nullable=False, index=True)
    resource_type = Column(String(50), nullable=False)
    quantity = Column(Float, nullable=False)
    endpoint = Column(String(255))
    model = Column(String(100))
    timestamp = Column(DateTime, server_default=func.now(), nullable=False, index=True)

    # Relationships
    application = relationship("Application", back_populates="resource_usage")
