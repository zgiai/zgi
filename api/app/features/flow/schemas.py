from pydantic import BaseModel
from typing import Optional, List, Dict

class FlowBase(BaseModel):
    name: str
    description: Optional[str] = None
    logo: Optional[str] = None
    status: Optional[int] = None
    flow_type: Optional[int] = None
    guide_word: Optional[str] = None
    data: Optional[Dict] = None

class FlowCreate(FlowBase):
    pass

class FlowUpdate(FlowBase):
    pass

class FlowInDBBase(FlowBase):
    id: str
    user_id: Optional[int] = None

    class Config:
        orm_mode = True
        from_attributes = True

class Flow(FlowInDBBase):
    pass

class FlowVersionBase(BaseModel):
    name: str
    description: Optional[str] = None
    data: Optional[Dict] = None
    flow_type: Optional[int] = None
    is_current: Optional[int] = None
    is_delete: Optional[int] = None

class FlowVersionCreate(FlowVersionBase):
    pass

class FlowVersionUpdate(FlowVersionBase):
    pass

class FlowVersionInDBBase(FlowVersionBase):
    id: int
    flow_id: str
    user_id: Optional[int] = None
    original_version_id: Optional[int] = None

    class Config:
        orm_mode = True
        from_attributes = True

class FlowVersion(FlowVersionInDBBase):
    pass
