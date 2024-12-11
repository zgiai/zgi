from datetime import datetime
from typing import List, Optional
from pydantic import BaseModel

class TimeRange(BaseModel):
    start_date: datetime
    end_date: datetime

class UsageMetrics(BaseModel):
    api_calls: int
    total_tokens: int
    total_documents: int
    storage_used: float  # in MB
    vector_count: int
    timestamp: datetime

class SystemMetrics(BaseModel):
    cpu_usage: float
    memory_usage: float
    disk_usage: float
    api_latency: float
    error_rate: float
    timestamp: datetime

class UserActivityLog(BaseModel):
    user_id: int
    action: str
    resource_type: str
    resource_id: str
    status: str
    timestamp: datetime
    metadata: Optional[dict]

class AnalyticsReport(BaseModel):
    time_range: TimeRange
    total_users: int
    active_users: int
    total_api_calls: int
    average_response_time: float
    error_rate: float
    usage_by_endpoint: dict
    top_users: List[dict]
