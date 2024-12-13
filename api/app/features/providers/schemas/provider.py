from typing import Optional
from datetime import datetime
from pydantic import BaseModel, Field

class ProviderBase(BaseModel):
    """Base schema for provider data"""
    provider_name: str = Field(..., description="Provider name (e.g., OpenAI)")
    enabled: bool = Field(True, description="1 if enabled, 0 if disabled")
    api_key: Optional[str] = Field(None, description="Encrypted API Key")
    org_id: Optional[str] = Field(None, max_length=100, description="Optional organization ID")
    base_url: Optional[str] = Field(None, max_length=255, description="Custom base URL for this provider")

class ProviderCreate(ProviderBase):
    pass

class ProviderUpdate(BaseModel):
    """Schema for updating an existing provider"""
    provider_name: Optional[str] = Field(None, description="Provider name (e.g., OpenAI)")
    enabled: Optional[bool] = None
    api_key: Optional[str] = None
    org_id: Optional[str] = Field(None, max_length=100)
    base_url: Optional[str] = Field(None, max_length=255)

class ProviderInDBBase(ProviderBase):
    id: int
    created_at: datetime
    updated_at: datetime
    deleted_at: Optional[datetime] = None

    class Config:
        from_attributes = True

class Provider(ProviderInDBBase):
    pass

class ProviderResponse(ProviderInDBBase):
    pass
