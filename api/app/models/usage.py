from sqlalchemy import Column, Integer, String, DateTime, ForeignKey, Float
from sqlalchemy.orm import relationship
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

    # Relationships
    application = relationship("Application", back_populates="resource_usage")

# Update Application model relationship in models.py
from app.models.usage import ResourceUsage  # Add this import to models.py
# Add this line to Application class:
# resource_usage = relationship("ResourceUsage", back_populates="application", cascade="all, delete-orphan")
