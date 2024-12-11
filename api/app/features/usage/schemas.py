from datetime import datetime
from typing import Optional, List
from pydantic import BaseModel

class ResourceUsageBase(BaseModel):
    resource_type: str
    quantity: float
    endpoint: Optional[str] = None
    model: Optional[str] = None
    timestamp: datetime

class ResourceUsageCreate(ResourceUsageBase):
    application_id: int

class ResourceUsageResponse(ResourceUsageBase):
    id: int
    application_id: int

    class Config:
        from_attributes = True

class ResourceUsageStats(BaseModel):
    total_tokens: float
    total_api_calls: int
    token_usage_by_model: dict[str, float]
    api_calls_by_endpoint: dict[str, int]

class TimeRange(BaseModel):
    start_time: datetime
    end_time: datetime
