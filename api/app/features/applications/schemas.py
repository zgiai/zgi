from datetime import datetime
from typing import Optional, List
from enum import Enum
from pydantic import BaseModel

class ApplicationType(str, Enum):
    CONVERSATIONAL = "conversational"  # 对话式应用
    GENERATIVE = "generative"         # 生成式应用
    FUNCTION = "function"             # 函数式应用
    WORKFLOW = "workflow"             # 工作流应用

class AccessLevel(str, Enum):
    PRIVATE = "private"      # 仅创建者可访问
    TEAM = "team"           # 团队内可访问
    PUBLIC = "public"       # 公开访问

class ApplicationBase(BaseModel):
    name: str
    description: Optional[str] = None
    type: ApplicationType
    access_level: AccessLevel = AccessLevel.PRIVATE
    team_id: Optional[int] = None  # 如果是团队应用，关联到团队

class ApplicationCreate(ApplicationBase):
    pass

class ApplicationUpdate(BaseModel):
    name: Optional[str] = None
    description: Optional[str] = None
    type: Optional[ApplicationType] = None
    access_level: Optional[AccessLevel] = None
    team_id: Optional[int] = None

class Application(ApplicationBase):
    id: int
    created_by: int
    created_at: datetime
    updated_at: datetime
    is_active: bool = True
    
    class Config:
        orm_mode = True

class ApplicationDetail(Application):
    # 这里可以添加更多详细信息，比如使用统计、配置等
    total_requests: int = 0
    average_latency: float = 0.0
    
    class Config:
        orm_mode = True
