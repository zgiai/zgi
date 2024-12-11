from datetime import datetime
from typing import Optional, List
from pydantic import BaseModel, Field

class PromptTemplateBase(BaseModel):
    name: str = Field(..., min_length=1, max_length=100)
    description: Optional[str] = Field(None, max_length=500)
    content: str = Field(..., min_length=1)
    version: str = Field(..., min_length=1, max_length=50)

class PromptTemplateCreate(PromptTemplateBase):
    application_id: int

class PromptTemplateUpdate(BaseModel):
    name: Optional[str] = Field(None, min_length=1, max_length=100)
    description: Optional[str] = Field(None, max_length=500)
    content: Optional[str] = Field(None, min_length=1)
    version: Optional[str] = Field(None, min_length=1, max_length=50)
    is_active: Optional[bool] = None

class PromptTemplateResponse(PromptTemplateBase):
    id: int
    is_active: bool
    application_id: int
    created_by: int
    created_at: datetime
    updated_at: datetime

    class Config:
        from_attributes = True

class PromptScenarioBase(BaseModel):
    name: str = Field(..., min_length=1, max_length=100)
    description: Optional[str] = Field(None, max_length=500)
    content: str = Field(..., min_length=1)
    template_id: int

class PromptScenarioCreate(PromptScenarioBase):
    pass

class PromptScenarioUpdate(BaseModel):
    name: Optional[str] = Field(None, min_length=1, max_length=100)
    description: Optional[str] = Field(None, max_length=500)
    content: Optional[str] = Field(None, min_length=1)

class PromptScenarioResponse(PromptScenarioBase):
    id: int
    created_by: int
    created_at: datetime
    updated_at: datetime

    class Config:
        from_attributes = True

class PromptPreviewRequest(BaseModel):
    template_id: int
    scenario_id: Optional[int] = None
    variables: Optional[dict] = None

class PromptPreviewResponse(BaseModel):
    rendered_prompt: str
