from datetime import datetime
from typing import List, Optional

from pydantic import BaseModel

class IPWhitelistBase(BaseModel):
    """Base IP whitelist schema"""
    allowed_ips: List[str]

class IPWhitelistCreate(IPWhitelistBase):
    """Create IP whitelist schema"""
    api_key_id: int

class IPWhitelistUpdate(BaseModel):
    """Update IP whitelist schema"""
    allowed_ips: List[str]
    is_active: Optional[bool] = None

class IPWhitelistResponse(IPWhitelistBase):
    """IP whitelist response schema"""
    id: int
    api_key_id: int
    is_active: bool
    created_at: datetime
    updated_at: Optional[datetime] = None

    class Config:
        from_attributes = True
