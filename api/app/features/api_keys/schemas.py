from datetime import datetime
from typing import Optional
from pydantic import BaseModel, Field

class APIKeyBase(BaseModel):
    name: str = Field(..., description="Name of the API key")

class APIKeyCreate(APIKeyBase):
    pass

class APIKey(APIKeyBase):
    uuid: str
    key: str
    project_uuid: str
    created_by: int
    is_active: bool
    created_at: datetime
    updated_at: datetime

    class Config:
        from_attributes = True
