from typing import Any, Dict, List, Optional, TypeVar, Generic
from pydantic import BaseModel


class FlowCompareReq(BaseModel):
    flow_id: str
    version_ids: List[int]


class FlowListRead(BaseModel):
    total: int
    items: List[Dict[str, Any]]


class FlowVersionCreate(BaseModel):
    name: str
    description: Optional[str] = ""


class StreamData(BaseModel):
    data: Any


T = TypeVar('T')

class UnifiedResponseModel(BaseModel, Generic[T]):
    code: int = 200
    msg: str = "success"
    data: Optional[T] = None


def resp_200(data: Any = None) -> Dict:
    return {
        "code": 200,
        "msg": "success",
        "data": data
    }
