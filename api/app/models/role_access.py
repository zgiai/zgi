from enum import Enum
from datetime import datetime
from typing import Optional

from sqlmodel import Field, SQLModel


class AccessType(str, Enum):
    FLOW_READ = "flow_read"
    FLOW_WRITE = "flow_write"


class RoleAccess(SQLModel, table=True):
    id: Optional[int] = Field(default=None, primary_key=True)
    role_id: str = Field(index=True)
    resource_id: str = Field(index=True)
    access_type: str = Field(default=AccessType.FLOW_READ.value)
    created_at: datetime = Field(default_factory=datetime.utcnow)
    updated_at: datetime = Field(default_factory=datetime.utcnow)
