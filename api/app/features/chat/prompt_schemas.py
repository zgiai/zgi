from datetime import datetime
from typing import Optional, Dict, Any, List
from pydantic import BaseModel, Field

class PromptBase(BaseModel):
    title: str = Field(..., min_length=1, max_length=255)
    content: str = Field(..., min_length=1)
    scenario: str = Field(..., min_length=1, max_length=100)
    description: Optional[str] = None
    metadata: Dict[str, Any] = Field(default_factory=dict)

class PromptCreate(PromptBase):
    pass

class PromptUpdate(BaseModel):
    title: Optional[str] = Field(None, min_length=1, max_length=255)
    content: Optional[str] = Field(None, min_length=1)
    scenario: Optional[str] = Field(None, min_length=1, max_length=100)
    description: Optional[str] = None
    metadata: Optional[Dict[str, Any]] = None

class PromptResponse(PromptBase):
    id: int
    user_id: int
    is_template: bool
    usage_count: int
    created_at: datetime
    updated_at: datetime

    class Config:
        from_attributes = True

class PromptPreviewRequest(BaseModel):
    prompt_id: Optional[int] = None
    content: Optional[str] = None
    variables: Dict[str, str] = Field(default_factory=dict)

class PromptPreviewResponse(BaseModel):
    preview: str
    tokens_used: int
    model_used: str

class PromptListParams(BaseModel):
    scenario: Optional[str] = None
    include_templates: bool = True
    search: Optional[str] = None
    page: int = Field(default=1, ge=1)
    page_size: int = Field(default=20, ge=1, le=100)

class PromptListResponse(BaseModel):
    items: List[PromptResponse]
    total: int
    page: int
    page_size: int
