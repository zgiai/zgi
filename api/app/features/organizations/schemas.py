from pydantic import BaseModel, Field, ConfigDict
from typing import Optional
from datetime import datetime
from app.core.database import Base

class OrganizationBase(BaseModel):
    name: str = Field(..., min_length=1, max_length=255)
    description: Optional[str] = Field(None, max_length=1000)

class OrganizationCreate(OrganizationBase):
    pass

class OrganizationUpdate(BaseModel):
    name: Optional[str] = Field(None, min_length=1, max_length=255)
    description: Optional[str] = Field(None, max_length=1000)
    is_active: Optional[bool] = True

    model_config = ConfigDict(
        from_attributes=True,
        json_encoders={
            datetime: lambda v: v.isoformat() if v else None,
            bytes: lambda v: v.decode() if v else None
        }
    )

class Organization(OrganizationBase):
    id: int
    uuid: str
    created_by: Optional[int]
    is_active: bool = True
    created_at: datetime
    updated_at: datetime

    model_config = ConfigDict(
        from_attributes=True,
        json_encoders={
            datetime: lambda v: v.isoformat() if v else None,
            bytes: lambda v: v.decode() if v else None
        }
    )
