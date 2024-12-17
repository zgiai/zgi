from sqlalchemy import Column, Integer, String, ForeignKey, Boolean, JSON, DateTime, Float, BigInteger
from sqlalchemy.orm import relationship
from sqlalchemy.sql import func
from app.core.database import Base
from app.features.providers.models.category import category_model_association

class ProviderModel(Base):
    """Model details table for storing information about each model"""
    __tablename__ = "model_provider_models"

    id = Column(BigInteger, primary_key=True, index=True)
    provider_id = Column(BigInteger, ForeignKey("model_providers.id"), nullable=False, comment="Foreign key to model_providers.id")
    model_name = Column(String(255), nullable=False, comment="Model name")
    model_version = Column(String(50), nullable=True, comment="Model version")
    description = Column(String(1000), nullable=True, comment="Brief model description")
    type = Column(String(50), nullable=False, comment="Model type (e.g., LLM, VISION)")
    modalities = Column(JSON, nullable=True, comment="Supported modalities (input/output)")
    
    # Technical specifications
    max_context_length = Column(Integer, nullable=True, comment="Maximum context length in tokens")
    supports_streaming = Column(Boolean, default=False, comment="1 if streaming is supported")
    supports_function_calling = Column(Boolean, default=False, comment="1 if function calling is supported")
    supports_roles = Column(Boolean, default=False, comment="1 if multi-role messages supported")
    supports_functions = Column(Boolean, default=False, comment="1 if tool functions are supported")
    embedding_dimensions = Column(Integer, nullable=True, comment="Vector dimensions if embedding model")
    
    # Default parameters
    default_temperature = Column(Float, nullable=True, comment="Default temperature parameter")
    default_top_p = Column(Float, nullable=True, comment="Default top_p parameter")
    default_max_tokens = Column(Integer, nullable=True, comment="Default max_tokens setting")
    
    # Pricing
    price_per_1k_tokens = Column(Float, nullable=True, comment="Price per 1000 tokens (if unified)")
    input_cost_per_1k_tokens = Column(Float, nullable=True, comment="Input cost per 1000 tokens (if split)")
    output_cost_per_1k_tokens = Column(Float, nullable=True, comment="Output cost per 1000 tokens")
    
    # API and performance
    api_call_name = Column(String(255), nullable=True, comment="API call endpoint name")
    latency_ms_estimate = Column(Integer, nullable=True, comment="Estimated latency in milliseconds")
    rate_limit_per_minute = Column(Integer, nullable=True, comment="Rate limit per minute")
    
    # Features and capabilities
    fine_tuning_available = Column(Boolean, default=False, comment="1 if finetune is supported")
    multi_lang_support = Column(Boolean, default=False, comment="1 if multi-language supported")
    
    # Model information
    release_date = Column(DateTime(timezone=True), nullable=True, comment="Model release date")
    developer_name = Column(String(255), nullable=True, comment="Developer/organization name")
    developer_website = Column(String(255), nullable=True, comment="Developer website URL")
    training_data_sources = Column(String(1000), nullable=True, comment="Training data source info")
    parameters_count = Column(String(50), nullable=True, comment="Parameter count description")
    model_architecture = Column(String(255), nullable=True, comment="Model architecture description")
    documentation_url = Column(String(255), nullable=True, comment="Documentation URL")
    demo_url = Column(String(255), nullable=True, comment="Demo URL")
    tags = Column(JSON, nullable=True, comment="Array of strings for tags")
    last_updated = Column(DateTime(timezone=True), nullable=True, comment="Last updated timestamp")
    
    # Timestamps and soft delete
    created_at = Column(DateTime(timezone=True), server_default=func.now(), nullable=False, comment="Creation timestamp")
    updated_at = Column(DateTime(timezone=True), server_default=func.now(), onupdate=func.now(), nullable=False, comment="Last update timestamp")
    deleted_at = Column(DateTime(timezone=True), nullable=True, comment="Soft delete timestamp (NULL if active)")
    
    # Relationships
    provider = relationship("ModelProvider", back_populates="models")
    categories = relationship(
        "ModelCategory",
        secondary=category_model_association,
        back_populates="models"
    )
