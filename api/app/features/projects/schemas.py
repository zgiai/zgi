from datetime import datetime
from typing import Optional, List
from pydantic import BaseModel, Field

from .models import ProjectStatus

class ProjectBase(BaseModel):
    name: str = Field(..., min_length=1, max_length=255)
    description: Optional[str] = Field(None, max_length=1000)
    status: ProjectStatus = Field(default=ProjectStatus.ACTIVE)

class ProjectCreate(ProjectBase):
    organization_id: int

class ProjectUpdate(ProjectBase):
    name: Optional[str] = Field(None, min_length=1, max_length=255)
    status: Optional[ProjectStatus] = None

class ProjectResponse(ProjectBase):
    id: int
    # uuid: str
    organization_id: int
    created_by: Optional[int] = None
    created_at: datetime
    updated_at: datetime

    class Config:
        from_attributes = True
        json_encoders = {
            datetime: lambda v: v.isoformat(),
            bytes: lambda v: v.decode()
        }

class ProjectList(BaseModel):
    projects: List[ProjectResponse]
    total: int

    class Config:
        from_attributes = True
