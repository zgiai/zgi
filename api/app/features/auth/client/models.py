from sqlalchemy import Column, Integer, String, Boolean, DateTime, ForeignKey
from sqlalchemy.sql import func
from sqlalchemy.orm import relationship
from datetime import datetime
from app.core.database import Base
from app.models.security import APIKey  # Import APIKey from security models

# Define relationships
APIKey.user = relationship("User", back_populates="api_keys")
