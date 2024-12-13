import pytest
from sqlalchemy import text
from sqlalchemy.orm import Session
from fastapi.testclient import TestClient
from jose import jwt

from app.core.database import Base, engine
from app.core.config import settings
from app.main import app
from app.features.users.models import User
from app.features.organizations.models import Organization
from app.features.projects.models import Project
from app.features.api_keys.models import APIKey

@pytest.fixture(scope="module")
def client() -> TestClient:
    """Create a test client"""
    return TestClient(app)

@pytest.fixture
def normal_user_token_headers(client: TestClient, user: User) -> dict:
    """Return headers with normal user token"""
    access_token = jwt.encode(
        {"sub": str(user.id), "exp": 1735689600},  # exp: 2025-01-01
        settings.SECRET_KEY,
        algorithm=settings.ALGORITHM
    )
    return {"Authorization": f"Bearer {access_token}"}

@pytest.fixture(scope="function")
def db():
    # Drop tables in reverse order of dependencies
    tables_to_drop = [
        "api_keys",
        "projects",
        "organization_members",
        "organizations",
        "users"
    ]

    # Drop all tables
    with engine.connect() as connection:
        connection.execute(text("SET FOREIGN_KEY_CHECKS=0"))
        for table in tables_to_drop:
            try:
                connection.execute(text(f"DROP TABLE IF EXISTS {table}"))
                connection.commit()
            except Exception:
                pass
        connection.execute(text("SET FOREIGN_KEY_CHECKS=1"))
        connection.commit()

    # Create all tables
    Base.metadata.create_all(bind=engine)

    # Create test session
    connection = engine.connect()
    transaction = connection.begin()
    session = Session(bind=connection)

    yield session

    # Rollback transaction and close session
    session.close()
    transaction.rollback()
    connection.close()

@pytest.fixture
def organization(db: Session) -> Organization:
    """Create a test organization"""
    org = Organization(
        name="Test Organization",
        uuid="test-org-uuid",
        is_active=True
    )
    db.add(org)
    db.commit()
    return org

@pytest.fixture
def user(db: Session, organization: Organization) -> User:
    """Create a test user"""
    user = User(
        email="test@example.com",
        username="testuser",
        full_name="Test User",
        hashed_password="hashed_password",
        is_active=True,
        is_verified=True
    )
    db.add(user)
    db.commit()
    return user

@pytest.fixture
def project(db: Session, organization: Organization, user: User) -> Project:
    """Create a test project"""
    project = Project(
        name="Test Project",
        uuid="test-project-uuid",
        organization_id=organization.id,
        created_by=user.id
    )
    db.add(project)
    db.commit()
    return project

@pytest.fixture
def api_key(db: Session, project: Project, user: User) -> APIKey:
    """Create a test API key"""
    key = APIKey(
        name="Test API Key",
        key="zgi_test_key",
        project_id=project.id,
        created_by=user.id
    )
    db.add(key)
    db.commit()
    return key
