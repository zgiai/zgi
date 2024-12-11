from datetime import datetime
from typing import List, Optional
from pydantic import BaseModel, Field

class AuditLog(BaseModel):
    id: str
    timestamp: datetime
    user_id: int
    action: str
    resource_type: str
    resource_id: str
    ip_address: str
    user_agent: str
    status: str
    details: dict

class AccessLog(BaseModel):
    timestamp: datetime
    user_id: int
    endpoint: str
    method: str
    ip_address: str
    status_code: int
    response_time: float
    request_size: int
    response_size: int

class ComplianceReport(BaseModel):
    report_id: str
    report_type: str
    time_range: dict
    generated_at: datetime
    generated_by: int
    findings: List[dict]
    recommendations: List[str]
    compliance_score: float

class RetentionPolicy(BaseModel):
    resource_type: str
    retention_period: int = Field(..., description="Retention period in days")
    auto_delete: bool = True
    exceptions: List[str] = []
    last_cleanup: datetime
    next_cleanup: datetime

class SecurityAlert(BaseModel):
    alert_id: str
    severity: str
    category: str
    description: str
    timestamp: datetime
    affected_resources: List[str]
    remediation_steps: List[str]
    status: str
