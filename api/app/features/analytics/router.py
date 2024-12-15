from fastapi import APIRouter, Depends, HTTPException
from sqlalchemy.orm import Session

from app.core.deps import get_db, get_current_user
from app.core.cache import get_cache
from app.features.analytics.schemas import (
    TimeRange,
    UsageMetrics,
    SystemMetrics,
    UserActivityLog,
    AnalyticsReport,
)
from app.features.analytics.service import AnalyticsService

router = APIRouter()


@router.post("/usage-metrics", response_model=UsageMetrics)
async def get_usage_metrics(
    time_range: TimeRange,
    current_user=Depends(get_current_user),
    db: Session = Depends(get_db),
    cache=Depends(get_cache),
):
    """Get usage metrics for a specific time range."""
    if not current_user.is_superuser:
        raise HTTPException(status_code=403, detail="Not authorized")

    service = AnalyticsService(db, cache)
    return await service.get_usage_metrics(time_range)


@router.get("/system-metrics", response_model=SystemMetrics)
async def get_system_metrics(
    current_user=Depends(get_current_user),
    db: Session = Depends(get_db),
    cache=Depends(get_cache),
):
    """Get current system metrics."""
    if not current_user.is_superuser:
        raise HTTPException(status_code=403, detail="Not authorized")

    service = AnalyticsService(db, cache)
    return await service.get_system_metrics()


@router.post("/activity", status_code=201)
async def log_activity(
    activity: UserActivityLog,
    current_user=Depends(get_current_user),
    db: Session = Depends(get_db),
    cache=Depends(get_cache),
):
    """Log user activity."""
    service = AnalyticsService(db, cache)
    await service.log_user_activity(activity)
    return {"status": "success"}


@router.post("/report", response_model=AnalyticsReport)
async def generate_report(
    time_range: TimeRange,
    current_user=Depends(get_current_user),
    db: Session = Depends(get_db),
    cache=Depends(get_cache),
):
    """Generate analytics report for a specific time range."""
    if not current_user.is_superuser:
        raise HTTPException(status_code=403, detail="Not authorized")

    service = AnalyticsService(db, cache)
    return await service.generate_analytics_report(time_range)
