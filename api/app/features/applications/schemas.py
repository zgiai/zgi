from pydantic import BaseModel, Field
from typing import Optional
from datetime import datetime

# Base Schema
class ApplicationBase(BaseModel):
    name: str = Field(..., min_length=1, max_length=255)
    description: Optional[str] = Field(None, max_length=1000)
    api_key_prefix: str = Field(..., min_length=1, max_length=50)
    max_tokens: Optional[int] = Field(default=1000, ge=0)
    max_requests_per_day: Optional[int] = Field(default=1000, ge=0)
    is_active: Optional[bool] = True

# Create Schema
class ApplicationCreate(ApplicationBase):
    pass

# Update Schema
class ApplicationUpdate(BaseModel):
    name: Optional[str] = Field(None, min_length=1, max_length=255)
    description: Optional[str] = None
    api_key_prefix: Optional[str] = Field(None, min_length=1, max_length=50)
    max_tokens: Optional[int] = Field(None, ge=0)
    max_requests_per_day: Optional[int] = Field(None, ge=0)
    is_active: Optional[bool] = None

# Response Schema
class ApplicationResponse(ApplicationBase):
    id: int
    owner_id: int
    created_at: datetime
    updated_at: datetime

    class Config:
        from_attributes = True

# List Schema
class ApplicationList(BaseModel):
    id: int
    name: str
    description: Optional[str]
    owner_id: int
    is_active: bool
    created_at: datetime

    class Config:
        from_attributes = True
