import os
import sys
import pytest
import pytest_asyncio
from fastapi.testclient import TestClient
from sqlalchemy import create_engine
from sqlalchemy.orm import sessionmaker
from sqlalchemy.pool import StaticPool
from sqlalchemy.ext.asyncio import create_async_engine, AsyncSession
from contextlib import contextmanager
import threading

# Add project root directory to Python path
sys.path.append(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from app.main import app
from app.core.database import Base
from app.core.database import get_db

def get_test_db():
    """Get a database connection"""
    # Import all models to ensure they are registered with Base.metadata
    from app.models import User, Team, TeamMember, TeamInvitation, APIKey, Application

    # Create a new engine
    engine = create_engine(
        "sqlite:///:memory:",
        connect_args={"check_same_thread": False},
        poolclass=StaticPool,
        echo=True  # Enable SQL logging for debugging
    )

    # Create all tables
    Base.metadata.create_all(bind=engine)

    # Create a new session factory
    TestingSessionLocal = sessionmaker(
        autocommit=False,
        autoflush=False,
        bind=engine,
        expire_on_commit=False  # Prevent detached instance errors
    )

    # Create a new session
    return TestingSessionLocal()

@pytest.fixture(scope="function")
def test_db():
    """Create a test database session"""
    db = get_test_db()
    try:
        yield db
    finally:
        db.rollback()
        db.close()

@pytest.fixture
def test_client(test_db):
    """Create a test client"""
    from app.main import app
    with TestClient(app) as client:
        yield client

@pytest.fixture(scope="function")
def client(test_db):
    """Create a test client with database dependency override"""
    def override_get_db():
        try:
            yield test_db
        finally:
            test_db.rollback()
            test_db.close()
    
    app.dependency_overrides[get_db] = override_get_db
    with TestClient(app) as test_client:
        yield test_client
    app.dependency_overrides.clear()

@pytest.fixture
def test_user(test_db, test_client):
    """Create a test user and return user info with access token"""
    from app.models import User
    from app.core.security import get_password_hash

    # Create test user
    user = User(
        email="test@example.com",
        username="testuser",
        hashed_password=get_password_hash("testpassword"),
        is_active=True,
        is_superadmin=True
    )
    test_db.add(user)
    test_db.commit()
    test_db.refresh(user)

    # Get access token
    response = test_client.post(
        "/v1/console/auth/login",
        json={
            "email": user.email,
            "password": "testpassword"
        }
    )
    token_data = response.json()

    test_db.refresh(user)  # Refresh user to get updated data
    return {
        "id": user.id,
        "email": user.email,
        "username": user.username,
        "access_token": token_data["access_token"]
    }

@pytest.fixture
def test_user_headers(test_user):
    """Return headers with test user token"""
    return {"Authorization": f"Bearer {test_user['access_token']}"}

@pytest.fixture
def test_application(test_db, test_user):
    """Create a test application"""
    from app.models import Application
    
    application = Application(
        name="Test Application",
        description="Test application for unit tests",
        owner_id=test_user["id"],
        is_active=True
    )
    test_db.add(application)
    test_db.commit()
    test_db.refresh(application)
    return application

@pytest.fixture
def test_prompt_template(test_db, test_application, test_user):
    """Create a test prompt template"""
    from app.models import PromptTemplate
    
    template = PromptTemplate(
        name="Test Template",
        description="Test template for unit tests",
        content="This is a {{test}} template",
        version="1.0",
        is_active=True,
        application_id=test_application.id,
        created_by=test_user["id"]
    )
    test_db.add(template)
    test_db.commit()
    test_db.refresh(template)
    return template

@pytest.fixture
def test_prompt_scenario(test_db, test_prompt_template, test_user):
    """Create a test prompt scenario"""
    from app.models import PromptScenario
    
    scenario = PromptScenario(
        name="Test Scenario",
        description="Test scenario for unit tests",
        content={"test": "value"},
        template_id=test_prompt_template.id,
        created_by=test_user["id"]
    )
    test_db.add(scenario)
    test_db.commit()
    test_db.refresh(scenario)
    return scenario
