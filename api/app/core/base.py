from typing import Any, Dict, Callable, Generic, TypeVar, Union
from functools import wraps

from pydantic import BaseModel

# 创建泛型变量
DataT = TypeVar('DataT')


class UnifiedResponseModel(Generic[DataT], BaseModel):
    """common response"""
    status_code: int
    status_message: str
    data: DataT = None


def resp_200(data: Union[list, dict, str, Any] = None,
             message: str = 'SUCCESS') -> UnifiedResponseModel:
    """success response"""
    return UnifiedResponseModel(status_code=200, status_message=message, data=data)
    # return data


def resp_500(code: int = 500,
             data: Union[list, dict, str, Any] = None,
             message: str = 'BAD REQUEST') -> UnifiedResponseModel:
    """fail response"""
    return UnifiedResponseModel(status_code=code, status_message=message, data=data)
