"""API key mapping models"""
from sqlalchemy import Column, Integer, String, DateTime, Float, JSON
from sqlalchemy.sql import func
from app.core.database import Base

class APIKeyMapping(Base):
    """API key mapping model for storing provider API keys"""
    __tablename__ = "api_key_mappings"

    id = Column(Integer, primary_key=True, index=True)
    api_key = Column(String(255), unique=True, index=True, nullable=False)
    provider_keys = Column(JSON, nullable=False)  # {"openai": "sk-xxx", "deepseek": "sk-yyy"}
    created_at = Column(DateTime, default=func.now())
    updated_at = Column(DateTime, default=func.now(), onupdate=func.now())
