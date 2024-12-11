from sqlalchemy import Column, Integer, String, DateTime, ForeignKey, Text, JSON, Boolean
from sqlalchemy.orm import relationship
from sqlalchemy.sql import func

from app.core.database import Base

class Prompt(Base):
    """Model for storing chat prompts"""
    __tablename__ = "prompts"

    id = Column(Integer, primary_key=True, index=True)
    user_id = Column(Integer, ForeignKey("users.id", ondelete="CASCADE"), nullable=False)
    title = Column(String(255), nullable=False)
    content = Column(Text, nullable=False)
    scenario = Column(String(100), nullable=False)
    description = Column(Text)
    is_template = Column(Boolean, default=False)  # Whether this is a system template
    metadata = Column(JSON, default=dict)  # Additional metadata like tags, variables, etc.
    usage_count = Column(Integer, default=0)  # Track how often this prompt is used
    created_at = Column(DateTime, server_default=func.now(), nullable=False)
    updated_at = Column(DateTime, server_default=func.now(), onupdate=func.now(), nullable=False)

    # Relationships
    user = relationship("User", back_populates="prompts")
