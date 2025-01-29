from pydantic import BaseModel
from typing import Optional, Dict, List


class AssistantBase(BaseModel):
    name: str
    prompt: str
    logo: Optional[str] = None
    desc: Optional[str] = None
    system_prompt: Optional[str] = None
    guide_word: Optional[str] = ''
    guide_question: Optional[Dict] = None
    model_id: Optional[int] = 0
    temperature: Optional[float] = 0.5
    max_token: Optional[int] = 0
    status: Optional[int] = 0
    user_id: Optional[int] = None
    is_delete: Optional[int] = 0


class AssistantCreate(AssistantBase):
    knowledge_list: Optional[List[str]] = []
    tool_list: Optional[List[str]] = []


class AssistantUpdate(AssistantBase):
    knowledge_list: Optional[List[str]] = []
    tool_list: Optional[List[str]] = []


class AssistantInDBBase(AssistantBase):
    id: str
    knowledges: Optional[str] = None
    tools: Optional[str] = None

    class Config:
        orm_mode = True
        from_attributes = True

    @property
    def knowledge_list(self) -> List[str]:
        if self.knowledges is None or self.knowledges == '':
            return []
        return self.knowledges.split(',')

    @property
    def tool_list(self) -> List[str]:
        if self.tools is None or self.tools == '':
            return []
        return self.tools.split(',')

    def dict(self, *args, **kwargs):
        data = super().dict(*args, **kwargs)
        data['knowledge_list'] = self.knowledge_list
        data['tool_list'] = self.tool_list
        data.pop('knowledges', None)
        data.pop('tools', None)
        return data


class Assistant(AssistantInDBBase):
    pass


class AssistantInDB(AssistantInDBBase):
    pass
