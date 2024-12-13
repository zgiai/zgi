from sqlalchemy import Column, BigInteger, String, Boolean, DateTime, Text
from sqlalchemy.orm import relationship
from sqlalchemy.sql import text
from app.db.base_class import Base
from app.features.providers.models.model import ProviderModel

class ModelProvider(Base):
    """Model provider table for storing provider information and credentials"""
    __tablename__ = "model_providers"

    id = Column(BigInteger, primary_key=True, index=True)
    provider_name = Column(String(100), nullable=False)
    enabled = Column(Boolean, default=True, comment="1 if enabled, 0 if disabled")
    api_key = Column(Text, nullable=True, comment="Encrypted API Key")
    org_id = Column(String(100), nullable=True, comment="Optional organization ID")
    base_url = Column(String(255), nullable=True, comment="Custom base URL for this provider")

    # Timestamps
    created_at = Column(DateTime(timezone=True), server_default=text('CURRENT_TIMESTAMP(6)'), nullable=False, comment="Creation timestamp")
    updated_at = Column(DateTime(timezone=True), server_default=text('CURRENT_TIMESTAMP(6)'), onupdate=text('CURRENT_TIMESTAMP(6)'), nullable=False, comment="Last update timestamp")
    deleted_at = Column(DateTime(timezone=True), nullable=True, comment="Soft delete timestamp (NULL if active)")

    # Relationships
    models = relationship("ProviderModel", back_populates="provider", cascade="all, delete-orphan")
