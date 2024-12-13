from datetime import datetime, date
from typing import Optional, List, Dict, Any
from pydantic import BaseModel, Field, condecimal
from decimal import Decimal

class ModelBase(BaseModel):
    """Base schema for model data"""
    provider_id: int = Field(..., description="Foreign key to model_providers.id")
    model_name: str = Field(..., max_length=100, description="Model name")
    model_version: Optional[str] = Field(None, max_length=50, description="Model version")
    description: Optional[str] = Field(None, max_length=255, description="Brief model description")
    type: str = Field(..., max_length=50, description="Model type (e.g., LLM, VISION)")
    modalities: Optional[Dict[str, Any]] = Field(None, description="Supported modalities (input/output)")
    
    # Technical specifications
    max_context_length: Optional[int] = Field(None, description="Maximum context length in tokens")
    supports_streaming: bool = Field(False, description="1 if streaming is supported")
    supports_function_calling: bool = Field(False, description="1 if function calling is supported")
    supports_roles: bool = Field(False, description="1 if multi-role messages supported")
    supports_functions: bool = Field(False, description="1 if tool functions are supported")
    embedding_dimensions: Optional[int] = Field(None, description="Vector dimensions if embedding model")
    
    # Default parameters
    default_temperature: Optional[condecimal(max_digits=3, decimal_places=2)] = Field(None, description="Default temperature parameter")
    default_top_p: Optional[condecimal(max_digits=3, decimal_places=2)] = Field(None, description="Default top_p parameter")
    default_max_tokens: Optional[int] = Field(None, description="Default max_tokens setting")
    
    # Pricing
    price_per_1k_tokens: Optional[condecimal(max_digits=10, decimal_places=4)] = Field(None, description="Price per 1000 tokens (if unified)")
    input_cost_per_1k_tokens: Optional[condecimal(max_digits=10, decimal_places=4)] = Field(None, description="Input cost per 1000 tokens (if split)")
    output_cost_per_1k_tokens: Optional[condecimal(max_digits=10, decimal_places=4)] = Field(None, description="Output cost per 1000 tokens")
    
    # API and performance
    api_call_name: Optional[str] = Field(None, max_length=100, description="API call endpoint name")
    latency_ms_estimate: Optional[int] = Field(None, description="Estimated latency in ms")
    rate_limit_per_minute: Optional[int] = Field(None, description="Rate limit per minute")
    
    # Features and capabilities
    fine_tuning_available: bool = Field(False, description="1 if finetune is supported")
    multi_lang_support: bool = Field(False, description="1 if multi-language supported")
    
    # Model information
    release_date: Optional[date] = Field(None, description="Model release date")
    developer_name: Optional[str] = Field(None, max_length=100, description="Developer/organization name")
    developer_website: Optional[str] = Field(None, max_length=255, description="Developer website URL")
    training_data_sources: Optional[str] = Field(None, description="Training data source info")
    parameters_count: Optional[str] = Field(None, max_length=50, description="Parameter count description")
    model_architecture: Optional[str] = Field(None, max_length=100, description="Model architecture description")
    documentation_url: Optional[str] = Field(None, max_length=255, description="Documentation URL")
    demo_url: Optional[str] = Field(None, max_length=255, description="Demo URL")
    tags: Optional[List[str]] = Field(None, description="Array of strings for tags")

class ModelCreate(ModelBase):
    """Schema for creating a new model"""
    pass

class ModelUpdate(BaseModel):
    """Schema for updating an existing model"""
    model_name: Optional[str] = Field(None, max_length=100)
    model_version: Optional[str] = Field(None, max_length=50)
    description: Optional[str] = Field(None, max_length=255)
    type: Optional[str] = Field(None, max_length=50)
    modalities: Optional[Dict[str, Any]] = None
    max_context_length: Optional[int] = None
    supports_streaming: Optional[bool] = None
    supports_function_calling: Optional[bool] = None
    supports_roles: Optional[bool] = None
    supports_functions: Optional[bool] = None
    embedding_dimensions: Optional[int] = None
    default_temperature: Optional[Decimal] = None
    default_top_p: Optional[Decimal] = None
    default_max_tokens: Optional[int] = None
    price_per_1k_tokens: Optional[Decimal] = None
    input_cost_per_1k_tokens: Optional[Decimal] = None
    output_cost_per_1k_tokens: Optional[Decimal] = None
    api_call_name: Optional[str] = Field(None, max_length=100)
    latency_ms_estimate: Optional[int] = None
    rate_limit_per_minute: Optional[int] = None
    fine_tuning_available: Optional[bool] = None
    multi_lang_support: Optional[bool] = None
    release_date: Optional[date] = None
    developer_name: Optional[str] = Field(None, max_length=100)
    developer_website: Optional[str] = Field(None, max_length=255)
    training_data_sources: Optional[str] = None
    parameters_count: Optional[str] = Field(None, max_length=50)
    model_architecture: Optional[str] = Field(None, max_length=100)
    documentation_url: Optional[str] = Field(None, max_length=255)
    demo_url: Optional[str] = Field(None, max_length=255)
    tags: Optional[List[str]] = None

class ModelInDB(ModelBase):
    """Schema for model data as stored in database"""
    id: int
    created_at: datetime
    updated_at: datetime
    deleted_at: Optional[datetime] = None
    last_updated: Optional[datetime] = None

    class Config:
        from_attributes = True

class ModelResponse(ModelInDB):
    """Schema for model response data"""
    pass

class ModelFilter(BaseModel):
    """Schema for filtering models in list endpoint"""
    provider_id: Optional[int] = None
    type: Optional[str] = None
    supports_streaming: Optional[bool] = None
    supports_function_calling: Optional[bool] = None
    supports_roles: Optional[bool] = None
    fine_tuning_available: Optional[bool] = None
    multi_lang_support: Optional[bool] = None
    max_price_per_1k_tokens: Optional[Decimal] = None
    min_context_length: Optional[int] = None
    tags: Optional[List[str]] = None
    search: Optional[str] = None
