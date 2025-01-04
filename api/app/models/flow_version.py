from datetime import datetime
from typing import Dict, Optional
from uuid import UUID

from sqlmodel import Field, SQLModel, JSON, Column


class FlowVersionBase(SQLModel):
    flow_id: UUID = Field(index=True)
    name: str
    description: Optional[str] = Field(default="")
    data: Dict = Field(default={}, sa_column=Column(JSON))
    is_current: bool = Field(default=False)
    created_at: datetime = Field(default_factory=datetime.utcnow)
    updated_at: datetime = Field(default_factory=datetime.utcnow)


class FlowVersion(FlowVersionBase, table=True):
    id: Optional[int] = Field(default=None, primary_key=True)


class FlowVersionDao:
    @staticmethod
    def get_version_by_flow(flow_id: str) -> Optional[FlowVersion]:
        """Get current version by flow id"""
        # TODO: Implement this
        return None
