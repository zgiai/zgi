from datetime import datetime
from typing import List, Optional
from pydantic import BaseModel, HttpUrl, Field

class WebhookConfig(BaseModel):
    url: HttpUrl
    events: List[str]
    is_active: bool
    secret_key: str
    created_at: datetime
    updated_at: datetime

class IntegrationConfig(BaseModel):
    name: str
    type: str
    config: dict
    is_active: bool
    created_at: datetime
    updated_at: datetime

class WebhookEvent(BaseModel):
    event_type: str
    payload: dict
    timestamp: datetime
    status: str
    retry_count: int = 0

class IntegrationStatus(BaseModel):
    integration_id: str
    status: str
    last_sync: datetime
    error_message: Optional[str]
    metrics: dict
