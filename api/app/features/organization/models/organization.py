from datetime import datetime
from typing import Dict, Any
from sqlalchemy import Column, Integer, String, Boolean, DateTime, JSON, ForeignKey
from sqlalchemy.orm import relationship

from app.core.database import Base

class Organization(Base):
    """Organization model"""
    __tablename__ = "organizations"

    id = Column(Integer, primary_key=True, index=True)
    name = Column(String(255), nullable=False)
    description = Column(String(512), nullable=True)
    
    # Organization information
    logo_url = Column(String(512), nullable=True)
    website = Column(String(512), nullable=True)
    
    # Organization settings
    settings = Column(JSON, nullable=True)
    preferences = Column(JSON, nullable=True)
    
    # Organization status
    is_active = Column(Boolean, default=True)
    is_verified = Column(Boolean, default=False)
    
    # Timestamps
    created_at = Column(DateTime, default=datetime.utcnow)
    updated_at = Column(DateTime, default=datetime.utcnow, onupdate=datetime.utcnow)
    
    # Relationships
    knowledge_bases = relationship("app.features.knowledge.models.knowledge.KnowledgeBase", back_populates="organization")
    
    def __repr__(self):
        return f"<Organization {self.name}>"
    
    def to_dict(self) -> Dict[str, Any]:
        """Convert to dictionary"""
        return {
            "id": self.id,
            "name": self.name,
            "description": self.description,
            "logo_url": self.logo_url,
            "website": self.website,
            "settings": self.settings,
            "preferences": self.preferences,
            "is_active": self.is_active,
            "is_verified": self.is_verified,
            "created_at": self.created_at.isoformat(),
            "updated_at": self.updated_at.isoformat()
        }
