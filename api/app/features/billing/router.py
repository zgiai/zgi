from typing import List
from fastapi import APIRouter, Depends, Query, Response
from sqlalchemy.orm import Session
from datetime import datetime, timedelta

from app.core.database import get_db
from app.core.security import get_current_user
from app.models import User
from app.features.billing import schemas, service

router = APIRouter(prefix="/v1/console/billing", tags=["billing"])

@router.post("/alerts", response_model=schemas.BillingAlertResponse)
def create_billing_alert(
    data: schemas.BillingAlertCreate,
    db: Session = Depends(get_db),
    current_user: User = Depends(get_current_user)
):
    """Create a new billing alert"""
    billing_service = service.BillingService(db)
    return billing_service.create_billing_alert(data)

@router.put("/alerts/{alert_id}", response_model=schemas.BillingAlertResponse)
def update_billing_alert(
    alert_id: int,
    data: schemas.BillingAlertUpdate,
    db: Session = Depends(get_db),
    current_user: User = Depends(get_current_user)
):
    """Update billing alert settings"""
    billing_service = service.BillingService(db)
    return billing_service.update_billing_alert(alert_id, data)

@router.get("/applications/{application_id}/trend", response_model=schemas.BillingTrendResponse)
def get_billing_trend(
    application_id: int,
    start_date: datetime = Query(...),
    end_date: datetime = Query(...),
    db: Session = Depends(get_db),
    current_user: User = Depends(get_current_user)
):
    """Get billing trend data"""
    billing_service = service.BillingService(db)
    return billing_service.get_billing_trend(application_id, start_date, end_date)

@router.get("/applications/{application_id}/report")
def generate_billing_report(
    application_id: int,
    start_date: datetime = Query(...),
    end_date: datetime = Query(...),
    format: str = Query(..., regex="^(csv|pdf)$"),
    db: Session = Depends(get_db),
    current_user: User = Depends(get_current_user)
):
    """Generate billing report"""
    billing_service = service.BillingService(db)
    report_content = billing_service.generate_billing_report(
        application_id, start_date, end_date, format
    )
    
    media_type = "text/csv" if format == "csv" else "application/pdf"
    filename = f"billing_report_{start_date.date()}_{end_date.date()}.{format}"
    
    return Response(
        content=report_content,
        media_type=media_type,
        headers={"Content-Disposition": f"attachment; filename={filename}"}
    )

@router.get("/applications/{application_id}/daily-costs")
def get_daily_costs(
    application_id: int,
    date: datetime = Query(default_factory=datetime.utcnow),
    db: Session = Depends(get_db),
    current_user: User = Depends(get_current_user)
):
    """Get daily costs breakdown"""
    billing_service = service.BillingService(db)
    model_costs, total_cost = billing_service.calculate_daily_costs(application_id, date)
    return {
        "date": date.date(),
        "model_costs": model_costs,
        "total_cost": total_cost
    }
