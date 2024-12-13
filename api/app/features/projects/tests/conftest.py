import pytest
from fastapi.testclient import TestClient
from sqlalchemy.orm import Session

from app.main import app
from app.core.database import SessionLocal
from app.features.projects.models import Project
from app.features.organizations.models import Organization
from app.features.users.models import User
from app.core.security.auth import create_access_token

@pytest.fixture
def client():
    return TestClient(app)

@pytest.fixture
def db():
    db = SessionLocal()
    try:
        yield db
    finally:
        db.close()

@pytest.fixture
def test_user(db: Session):
    user = User(
        email="test@example.com",
        hashed_password="test_password",
        is_active=True
    )
    db.add(user)
    db.commit()
    db.refresh(user)
    return user

@pytest.fixture
def test_user_token(test_user: User):
    return create_access_token(test_user.id)

@pytest.fixture
def test_user_headers(test_user_token: str):
    return {"Authorization": f"Bearer {test_user_token}"}

@pytest.fixture
def test_org(db: Session, test_user: User):
    org = Organization(
        name="Test Organization",
        description="Test Description",
        created_by=test_user.id
    )
    db.add(org)
    db.commit()
    db.refresh(org)
    return org

@pytest.fixture
def test_project(db: Session, test_org: Organization, test_user: User):
    project = Project(
        name="Test Project",
        description="Test Project Description",
        organization_id=test_org.id,
        created_by=test_user.id
    )
    db.add(project)
    db.commit()
    db.refresh(project)
    return project
