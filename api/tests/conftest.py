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

from app.core.database import Base, get_db
from app.core.config import settings
from app.features.users.models import User
from app.models.team import Team, TeamMember, TeamInvitation
from app.models.application import Application

def get_test_db():
    """Get a database connection"""
    # Create a new engine using MySQL
    engine = create_engine(
        settings.SQLALCHEMY_DATABASE_URL,
        echo=True  # Enable SQL logging for debugging
    )

    # Create all tables
    Base.metadata.drop_all(bind=engine)  # 清除所有表
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
    
    from app.main import app
    app.dependency_overrides[get_db] = override_get_db
    with TestClient(app) as test_client:
        yield test_client
    app.dependency_overrides.clear()

@pytest.fixture(scope="function")
def db():
    """Create a test database session"""
    db = get_test_db()
    try:
        yield db
    finally:
        db.close()

@pytest.fixture
def test_user(test_db, test_client):
    """Create a test user and return user info with access token"""
    from app.core.security import get_password_hash

    # Create test user
    user = User(
        email="test@example.com",
        username="testuser",
        hashed_password=get_password_hash("testpassword"),
        is_active=True,
        is_admin=True,
        is_superuser=True
    )
    test_db.add(user)
    test_db.commit()
    test_db.refresh(user)
    yield user
    test_db.delete(user)
    test_db.commit()

@pytest.fixture
def test_user_headers(test_user):
    """Return headers with test user token"""
    return {"Authorization": f"Bearer {test_user.id}"}

@pytest.fixture
def test_application(test_db, test_user):
    """Create a test application"""
    app = Application(
        name="Test App",
        description="Test application",
        owner_id=test_user.id
    )
    test_db.add(app)
    test_db.commit()
    test_db.refresh(app)
    yield app
    test_db.delete(app)
    test_db.commit()

@pytest.fixture
def test_prompt_template(test_db, test_application, test_user):
    """Create a test prompt template"""
    from app.models.prompt_template import PromptTemplate
    
    template = PromptTemplate(
        name="Test Template",
        description="Test template",
        content="Test content",
        application_id=test_application.id,
        owner_id=test_user.id
    )
    test_db.add(template)
    test_db.commit()
    test_db.refresh(template)
    yield template
    test_db.delete(template)
    test_db.commit()

@pytest.fixture
def test_prompt_scenario(test_db, test_prompt_template, test_user):
    """Create a test prompt scenario"""
    from app.models.prompt_scenario import PromptScenario
    
    scenario = PromptScenario(
        name="Test Scenario",
        description="Test scenario",
        prompt_template_id=test_prompt_template.id,
        owner_id=test_user.id
    )
    test_db.add(scenario)
    test_db.commit()
    test_db.refresh(scenario)
    yield scenario
    test_db.delete(scenario)
    test_db.commit()
