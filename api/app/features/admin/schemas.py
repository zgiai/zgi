from datetime import datetime
from typing import List, Optional
from pydantic import BaseModel, Field

class SystemConfig(BaseModel):
    maintenance_mode: bool = False
    max_upload_size: int = Field(default=10485760)  # 10MB
    allowed_file_types: List[str] = ["pdf", "txt", "doc", "docx"]
    vector_dimensions: int = 1536
    max_tokens_per_request: int = 4096
    rate_limit_per_minute: int = 60

class SystemStatus(BaseModel):
    status: str
    version: str
    uptime: float
    active_users: int
    total_documents: int
    total_vectors: int
    storage_usage: float
    memory_usage: float
    cpu_usage: float

class AdminUserManagement(BaseModel):
    user_id: int
    email: str
    status: str
    role: str
    last_login: Optional[datetime]
    created_at: datetime
    teams: List[str]
    usage_stats: dict

class SystemLogs(BaseModel):
    level: str
    timestamp: datetime
    service: str
    message: str
    metadata: dict

class MaintenanceWindow(BaseModel):
    start_time: datetime
    end_time: datetime
    description: str
    affected_services: List[str]
    notification_sent: bool
