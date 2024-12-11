import pytest
from datetime import datetime, timedelta
from unittest.mock import Mock, patch
from sqlalchemy.orm import Session

from app.features.billing import service, schemas
from app.models.billing import ModelPricing, BillingAlert, BillingRecord
from app.models.usage import ResourceUsage

def test_create_billing_alert(test_db: Session, test_application):
    """Test creating billing alert"""
    billing_service = service.BillingService(test_db)
    
    alert_data = schemas.BillingAlertCreate(
        application_id=test_application.id,
        alert_type="daily_limit",
        threshold_amount=100.0,
        notification_email="test@example.com"
    )
    
    alert = billing_service.create_billing_alert(alert_data)
    
    assert alert.application_id == test_application.id
    assert alert.alert_type == "daily_limit"
    assert alert.threshold_amount == 100.0
    assert alert.is_active == True

def test_calculate_daily_costs(test_db: Session, test_application):
    """Test daily cost calculation"""
    billing_service = service.BillingService(test_db)
    
    # Create test pricing
    pricing = ModelPricing(
        model_name="gpt-4",
        price_per_1k_tokens=0.01
    )
    test_db.add(pricing)
    test_db.commit()
    
    # Create test usage records
    now = datetime.utcnow()
    usage1 = ResourceUsage(
        application_id=test_application.id,
        resource_type="token",
        quantity=1000.0,
        model="gpt-4",
        timestamp=now
    )
    usage2 = ResourceUsage(
        application_id=test_application.id,
        resource_type="token",
        quantity=2000.0,
        model="gpt-4",
        timestamp=now
    )
    test_db.add(usage1)
    test_db.add(usage2)
    test_db.commit()
    
    model_costs, total_cost = billing_service.calculate_daily_costs(
        test_application.id,
        now
    )
    
    assert model_costs["gpt-4"] == 0.03  # (3000 tokens / 1000) * $0.01
    assert total_cost == 0.03

@patch('app.core.notifications.BillingNotifier.send_daily_limit_alert')
def test_daily_limit_alert(mock_send_alert, test_db: Session, test_application):
    """Test daily limit alert triggering"""
    billing_service = service.BillingService(test_db)
    
    # Create alert
    alert = BillingAlert(
        application_id=test_application.id,
        alert_type="daily_limit",
        threshold_amount=0.02,
        notification_email="test@example.com"
    )
    test_db.add(alert)
    
    # Create pricing
    pricing = ModelPricing(
        model_name="gpt-4",
        price_per_1k_tokens=0.01
    )
    test_db.add(pricing)
    test_db.commit()
    
    # Create usage that exceeds threshold
    usage = ResourceUsage(
        application_id=test_application.id,
        resource_type="token",
        quantity=3000.0,  # Will cost $0.03
        model="gpt-4",
        timestamp=datetime.utcnow()
    )
    test_db.add(usage)
    test_db.commit()
    
    # Record daily billing
    billing_service.record_daily_billing(
        test_application.id,
        datetime.utcnow()
    )
    
    # Check if alert was triggered
    mock_send_alert.assert_called_once()
    alert = test_db.query(BillingAlert).first()
    assert alert.last_triggered_at is not None

def test_generate_billing_report(test_db: Session, test_application):
    """Test billing report generation"""
    billing_service = service.BillingService(test_db)
    
    # Create test records
    now = datetime.utcnow()
    record1 = BillingRecord(
        application_id=test_application.id,
        date=now - timedelta(days=1),
        model_costs={"gpt-4": 0.03},
        total_cost=0.03
    )
    record2 = BillingRecord(
        application_id=test_application.id,
        date=now,
        model_costs={"gpt-4": 0.02},
        total_cost=0.02
    )
    test_db.add(record1)
    test_db.add(record2)
    test_db.commit()
    
    # Test CSV report
    csv_report = billing_service.generate_billing_report(
        test_application.id,
        now - timedelta(days=2),
        now,
        "csv"
    )
    assert isinstance(csv_report, bytes)
    assert b"Date,Model,Cost (USD)" in csv_report
    
    # Test PDF report
    pdf_report = billing_service.generate_billing_report(
        test_application.id,
        now - timedelta(days=2),
        now,
        "pdf"
    )
    assert isinstance(pdf_report, bytes)
    assert pdf_report.startswith(b"%PDF")  # PDF magic number

def test_billing_trend(test_db: Session, test_application):
    """Test billing trend analysis"""
    billing_service = service.BillingService(test_db)
    
    # Create test records
    now = datetime.utcnow()
    records = []
    for i in range(5):
        record = BillingRecord(
            application_id=test_application.id,
            date=now - timedelta(days=i),
            model_costs={"gpt-4": 0.01 * (i + 1)},
            total_cost=0.01 * (i + 1)
        )
        records.append(record)
    
    test_db.add_all(records)
    test_db.commit()
    
    trend = billing_service.get_billing_trend(
        test_application.id,
        now - timedelta(days=5),
        now
    )
    
    assert len(trend.dates) == 5
    assert len(trend.costs) == 5
    assert "gpt-4" in trend.model_breakdown
    assert trend.costs == sorted(trend.costs)  # Costs should be in ascending order
