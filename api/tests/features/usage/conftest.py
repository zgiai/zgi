import pytest
from datetime import datetime
from sqlalchemy.orm import Session

from app.models import Application
from app.models.usage import ResourceUsage

@pytest.fixture
def test_application(test_db: Session, test_user):
    """创建测试应用"""
    app = Application(
        name="Test App",
        description="Test application for usage tracking",
        type="conversational",
        access_level="private",
        created_by=test_user.id
    )
    test_db.add(app)
    test_db.commit()
    test_db.refresh(app)
    return app

@pytest.fixture
def test_usage(test_db: Session, test_application):
    """创建测试使用记录"""
    usage = ResourceUsage(
        application_id=test_application.id,
        resource_type="token",
        quantity=100.0,
        model="gpt-4",
        timestamp=datetime.utcnow()
    )
    test_db.add(usage)
    test_db.commit()
    test_db.refresh(usage)
    return usage
