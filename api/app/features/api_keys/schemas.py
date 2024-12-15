from datetime import datetime
from typing import Optional, List
from pydantic import BaseModel, Field

class APIKeyBase(BaseModel):
    name: str = Field(..., description="Name of the API key")

class APIKeyCreate(APIKeyBase):
    pass

class APIKeyResponse(APIKeyBase):
    uuid: str
    key: str
    project_id: int
    created_by: int
    status: str
    created_at: datetime
    updated_at: Optional[datetime]

    class Config:
        from_attributes = True

class APIKeyList(BaseModel):
    api_keys: List[APIKeyResponse]
    total: int

    class Config:
        from_attributes = True