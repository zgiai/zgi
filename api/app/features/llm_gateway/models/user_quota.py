"""User quota and usage tracking models"""
from sqlalchemy import Column, Integer, String, DateTime, Float
from sqlalchemy.sql import func
from app.db.base_class import Base

class UserQuota(Base):
    """User quota model for tracking API usage limits"""
    __tablename__ = "user_quotas"

    id = Column(Integer, primary_key=True, index=True)
    api_key = Column(String, unique=True, index=True, nullable=False)
    total_tokens = Column(Integer, default=0)  # Total token limit
    used_tokens = Column(Integer, default=0)   # Used tokens
    reset_date = Column(DateTime, default=func.now())  # Quota reset date
    created_at = Column(DateTime, default=func.now())
    updated_at = Column(DateTime, default=func.now(), onupdate=func.now())

class UsageLog(Base):
    """Usage log model for tracking API requests"""
    __tablename__ = "usage_logs"

    id = Column(Integer, primary_key=True, index=True)
    api_key = Column(String, index=True, nullable=False)
    model = Column(String, nullable=False)
    prompt_tokens = Column(Integer, default=0)
    completion_tokens = Column(Integer, default=0)
    total_tokens = Column(Integer, default=0)
    cost = Column(Float, default=0.0)  # Cost in credits/currency
    created_at = Column(DateTime, default=func.now())
