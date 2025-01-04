from datetime import datetime
from enum import Enum
from typing import Dict, List, Optional
from uuid import UUID, uuid4

from pydantic import BaseModel, Field
from sqlmodel import Field, SQLModel, JSON, Column


class FlowType(str, Enum):
    FLOW = "flow"
    AGENT = "agent"


class FlowBase(SQLModel):
    name: str = Field(index=True)
    description: Optional[str] = Field(default="")
    data: Dict = Field(default={}, sa_column=Column(JSON))
    style: Dict = Field(default={}, sa_column=Column(JSON))
    user_id: Optional[str] = Field(default=None, foreign_key="user.id")
    status: int = Field(default=1)  # 1: draft, 2: online
    type: str = Field(default=FlowType.FLOW.value)
    created_at: datetime = Field(default_factory=datetime.utcnow)
    updated_at: datetime = Field(default_factory=datetime.utcnow)


class Flow(FlowBase, table=True):
    id: UUID = Field(default_factory=uuid4, primary_key=True)


class FlowCreate(FlowBase):
    pass


class FlowUpdate(FlowBase):
    name: Optional[str] = None
    description: Optional[str] = None
    data: Optional[Dict] = None
    style: Optional[Dict] = None
    status: Optional[int] = None


class FlowRead(FlowBase):
    id: UUID
    version_id: Optional[int] = None


class FlowReadWithStyle(FlowRead):
    style: Dict = {}


class FlowDao:
    @staticmethod
    def create_flow(flow: Flow, flow_type: str = FlowType.FLOW.value) -> Flow:
        """Create a new flow"""
        flow.type = flow_type
        return flow
