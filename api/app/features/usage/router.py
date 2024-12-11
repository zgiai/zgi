from typing import List, Optional
from fastapi import APIRouter, Depends, Query
from sqlalchemy.orm import Session

from app.core.database import get_db
from app.core.security import get_current_user
from app.models import User
from app.features.usage import schemas, service

router = APIRouter(prefix="/v1/console/usage", tags=["usage"])

@router.post("/record", response_model=schemas.ResourceUsageResponse)
def record_usage(
    data: schemas.ResourceUsageCreate,
    db: Session = Depends(get_db),
    current_user: User = Depends(get_current_user)
):
    """Record a new resource usage entry"""
    usage_service = service.UsageService(db)
    return usage_service.record_usage(data)

@router.get("/applications/{application_id}/stats", response_model=schemas.ResourceUsageStats)
def get_usage_stats(
    application_id: int,
    time_range: Optional[schemas.TimeRange] = None,
    db: Session = Depends(get_db),
    current_user: User = Depends(get_current_user)
):
    """Get aggregated usage statistics for an application"""
    usage_service = service.UsageService(db)
    start_time = time_range.start_time if time_range else None
    end_time = time_range.end_time if time_range else None
    return usage_service.get_usage_stats(application_id, start_time, end_time)

@router.get("/applications/{application_id}/records", response_model=List[schemas.ResourceUsageResponse])
def list_usage(
    application_id: int,
    time_range: Optional[schemas.TimeRange] = None,
    resource_type: Optional[str] = Query(None, regex="^(token|api_call)$"),
    db: Session = Depends(get_db),
    current_user: User = Depends(get_current_user)
):
    """List all usage records for an application with optional filters"""
    usage_service = service.UsageService(db)
    start_time = time_range.start_time if time_range else None
    end_time = time_range.end_time if time_range else None
    return usage_service.list_usage(application_id, start_time, end_time, resource_type)
