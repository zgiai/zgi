from pydantic import BaseModel, Field
from typing import Optional
from datetime import datetime

class OrganizationBase(BaseModel):
    name: str = Field(..., min_length=1, max_length=255)
    description: Optional[str] = Field(None, max_length=1000)

class OrganizationCreate(OrganizationBase):
    pass

class OrganizationUpdate(OrganizationBase):
    name: Optional[str] = Field(None, min_length=1, max_length=255)
    is_active: Optional[bool] = None

class Organization(OrganizationBase):
    id: str
    created_by: Optional[int]
    is_active: bool
    created_at: datetime
    updated_at: datetime

    class Config:
        orm_mode = True
