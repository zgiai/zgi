from datetime import datetime
from typing import Optional, List
from pydantic import BaseModel

class APIKeyBase(BaseModel):
    name: str
    team_id: Optional[int] = None

class APIKeyCreate(APIKeyBase):
    pass

class APIKeyUpdate(BaseModel):
    name: Optional[str] = None
    is_active: Optional[bool] = None

class APIKey(APIKeyBase):
    id: int
    key: str
    user_id: int
    expires_at: Optional[datetime]
    created_at: datetime
    last_used_at: Optional[datetime]
    is_active: bool

    class Config:
        orm_mode = True

class APIKeyResponse(APIKey):
    pass

class APIKeyListResponse(BaseModel):
    items: List[APIKey]
    total: int
    
    class Config:
        orm_mode = True
