from pydantic import BaseModel
from typing import Optional, Dict


class AssistantBase(BaseModel):
    name: str
    logo: str
    desc: Optional[str] = None
    system_prompt: Optional[str] = None
    prompt: Optional[str] = None
    guide_word: Optional[str] = None
    guide_question: Optional[Dict] = None
    model_name: str
    temperature: float
    max_token: int
    status: int
    user_id: int
    is_delete: int


class AssistantCreate(AssistantBase):
    id: str


class AssistantUpdate(AssistantBase):
    pass


class AssistantInDBBase(AssistantBase):
    id: str

    class Config:
        orm_mode = True


class Assistant(AssistantInDBBase):
    pass


class AssistantInDB(AssistantInDBBase):
    pass
