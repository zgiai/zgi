from pydantic import BaseModel, ConfigDict, Field
from typing import Dict, Optional, Any, List
from datetime import datetime


class LLMProviderCreate(BaseModel):
    """LLMProvider creation schema"""
    name: str
    provider: str
    description: Optional[str] = None
    api_base: Optional[str] = None
    api_key: Optional[str] = None


class LLMProviderUpdate(BaseModel):
    """LLMProvider update schema"""
    name: Optional[str] = None
    provider: Optional[str] = None
    description: Optional[str] = None
    api_base: Optional[str] = None
    api_key: Optional[str] = None


class LLMProviderResponse(BaseModel):
    """LLMProvider response schema"""
    id: int
    name: str
    provider: str
    description: Optional[str] = None
    api_base: Optional[str] = None
    api_key: Optional[str] = None
    user_id: int
    create_time: datetime
    update_time: datetime

    model_config = ConfigDict(
        from_attributes=True,
        json_encoders={
            datetime: lambda v: v.isoformat() if v else None,
            bytes: lambda v: v.decode() if v else None
        }
    )


class LLMProviderListResponse(BaseModel):
    """LLMProvider list response schema"""
    total: int
    page_size: int
    page_num: int
    data: List[LLMProviderResponse]

    model_config = ConfigDict(
        from_attributes=True
    )


class LLMModelCreate(BaseModel):
    """LLMModel creation schema"""
    provider_id: int
    name: str
    description: Optional[str] = None
    model_name: str
    model_type: str


class LLMModelUpdate(BaseModel):
    """LLMModel update schema"""
    name: Optional[str] = None
    description: Optional[str] = None
    model_name: Optional[str] = None
    model_type: Optional[str] = None
    status: Optional[int] = None


class LLMModelResponse(BaseModel):
    """LLMModel response schema"""
    id: int
    name: str
    description: Optional[str] = None
    model_name: str
    model_type: str
    status: int
    user_id: int
    create_time: datetime
    update_time: datetime

    model_config = ConfigDict(
        from_attributes=True,
        json_encoders={
            datetime: lambda v: v.isoformat() if v else None,
            bytes: lambda v: v.decode() if v else None
        }
    )


class LLMModelListResponse(BaseModel):
    """LLMModel list response schema"""
    total: int
    page_size: int
    page_num: int
    data: List[LLMModelResponse]

    class Config:
        from_attributes = True


class LLMConfigCreate(BaseModel):
    """LLMConfig creation schema"""
    config_key: str = Field(..., description="Unique key for the configuration")
    description: Optional[str] = Field(None, description="Description of the configuration")
    config: Dict[str, Any] = Field(..., description="Configuration data")


class LLMConfigUpdate(BaseModel):
    """LLMConfig update schema"""
    config_key: Optional[str] = Field(None, description="Unique key for the configuration")
    description: Optional[str] = Field(None, description="Description of the configuration")
    config: Optional[Dict[str, Any]] = Field(None, description="Configuration data")


class LLMConfigResponse(BaseModel):
    """LLMConfig response schema"""
    id: int = Field(..., description="ID of the LLMConfig")
    config_key: str = Field(..., description="Unique key for the configuration")
    llm_model_id: int = Field(..., description="ID of the LLMModel")
    description: Optional[str] = Field(None, description="Description of the configuration")
    create_time: str = Field(..., description="Creation time of the LLMConfig")
    update_time: str = Field(..., description="Last update time of the LLMConfig")

    model_config = ConfigDict(
        from_attributes=True,
        json_encoders={
            datetime: lambda v: v.isoformat() if v else None,
            bytes: lambda v: v.decode() if v else None
        }
    )


class LLMConfigListResponse(BaseModel):
    """LLMConfig list response schema"""
    total: int = Field(..., description="Total number of LLMConfigs")
    page_size: int = Field(..., description="Number of LLMConfigs per page")
    page_num: int = Field(..., description="Current page number")
    data: List[LLMConfigResponse] = Field(..., description="List of LLMConfigs")

    class Config:
        from_attributes = True
