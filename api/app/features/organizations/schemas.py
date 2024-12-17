from pydantic import BaseModel, Field, ConfigDict
from typing import Optional, List
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

class OrganizationResponse(OrganizationBase):
    id: int
    # uuid: str
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

class OrganizationList(BaseModel):
    organizations: List[OrganizationResponse]
    total: int

    class Config:
        from_attributes = True

class RoleBase(BaseModel):
    name: str = Field(..., min_length=1, max_length=255)
    description: Optional[str] = Field(None, max_length=1000)

class RoleCreate(RoleBase):
    organization_id: int

class RoleUpdate(RoleBase):
    pass

class RoleResponseBase(BaseModel):
    id: int
    name: str

    model_config = ConfigDict(
        from_attributes=True,
        json_encoders={
            datetime: lambda v: v.isoformat() if v else None,
            bytes: lambda v: v.decode() if v else None
        }
    )

class RoleResponse(RoleResponseBase):
    organization_id: int
    created_by: Optional[int]
    is_active: bool
    created_at: datetime
    updated_at: datetime

class RoleList(BaseModel):
    roles: List[RoleResponse]
    total: int

    class Config:
        from_attributes = True

class MemberBase(BaseModel):
    organization_id: int
    user_id: int

class MemberResponse(MemberBase):
    id: int
    username: str
    is_admin: bool
    roles: List[RoleResponseBase]
    created_at: datetime
    updated_at: datetime

    model_config = ConfigDict(
        from_attributes=True,
        json_encoders={
            datetime: lambda v: v.isoformat() if v else None,
            bytes: lambda v: v.decode() if v else None
        }
    )

class MemberList(BaseModel):
    members: List[MemberResponse]
    total: int

    class Config:
        from_attributes = True