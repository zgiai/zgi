from datetime import datetime
from typing import Optional
from pydantic import BaseModel, Field

class APIKeyBase(BaseModel):
    provider: str = Field(..., description="API provider (e.g., openai, anthropic)")
    key_name: str = Field(..., description="Key name for reference")
    key_value: str = Field(..., description="API key value")
    is_active: bool = Field(default=True, description="Whether the key is active")

class APIKeyCreate(APIKeyBase):
    pass

class APIKeyUpdate(BaseModel):
    provider: Optional[str] = None
    key_name: Optional[str] = None
    key_value: Optional[str] = None
    is_active: Optional[bool] = None

class APIKeyResponse(APIKeyBase):
    id: int
    user_id: int
    last_used_at: Optional[datetime]
    created_at: datetime
    updated_at: datetime

    class Config:
        orm_mode = True

# 用于列表展示，不显示敏感的key_value
class APIKeyListResponse(BaseModel):
    id: int
    user_id: int
    provider: str
    key_name: str
    is_active: bool
    last_used_at: Optional[datetime]
    created_at: datetime
    updated_at: datetime

    class Config:
        orm_mode = True
