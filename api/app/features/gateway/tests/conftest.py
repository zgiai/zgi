"""Test configuration and fixtures."""
import os
import pytest
from typing import Dict, Any, Generator
from fastapi.testclient import TestClient
from sqlalchemy import create_engine
from sqlalchemy.orm import Session, sessionmaker
from app.core.database import Base, get_db
from app.main import app
from app.features.api_keys.models import APIKey, APIKeyStatus
from app.features.projects.models import Project
from app.features.users.models import User

# Test database URL
SQLALCHEMY_DATABASE_URL = "sqlite:///./test.db"

# Create test engine
engine = create_engine(
    SQLALCHEMY_DATABASE_URL, connect_args={"check_same_thread": False}
)
TestingSessionLocal = sessionmaker(autocommit=False, autoflush=False, bind=engine)

@pytest.fixture(scope="session")
def test_db():
    """Create test database."""
    Base.metadata.create_all(bind=engine)
    yield
    Base.metadata.drop_all(bind=engine)

@pytest.fixture
def db(test_db) -> Generator[Session, None, None]:
    """Get database session."""
    connection = engine.connect()
    transaction = connection.begin()
    session = TestingSessionLocal(bind=connection)
    
    yield session
    
    session.close()
    transaction.rollback()
    connection.close()

@pytest.fixture
def client(db) -> Generator[TestClient, None, None]:
    """Get test client."""
    def override_get_db():
        try:
            yield db
        finally:
            pass
            
    app.dependency_overrides[get_db] = override_get_db
    yield TestClient(app)
    del app.dependency_overrides[get_db]

@pytest.fixture
def test_user(db) -> User:
    """Create test user."""
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
def test_project(db, test_user) -> Project:
    """Create test project."""
    project = Project(
        name="Test Project",
        organization_id=1,
        created_by=test_user.id,
        quota=1000000  # 1M tokens
    )
    db.add(project)
    db.commit()
    db.refresh(project)
    return project

@pytest.fixture
def test_api_key(db, test_project, test_user) -> APIKey:
    """Create test API key."""
    api_key = APIKey(
        name="Test API Key",
        key="zgi_test123456789",
        project_id=test_project.id,
        created_by=test_user.id,
        status=APIKeyStatus.ACTIVE
    )
    db.add(api_key)
    db.commit()
    db.refresh(api_key)
    return api_key

@pytest.fixture
def test_headers(test_api_key) -> Dict[str, Any]:
    """Get test headers."""
    return {
        "Authorization": f"Bearer {test_api_key.key}",
        "Content-Type": "application/json"
    }

@pytest.fixture(autouse=True)
def setup_test_env():
    """Setup test environment variables."""
    os.environ["ANTHROPIC_API_KEY"] = "test-anthropic-key"
    os.environ["OPENAI_API_KEY"] = "test-openai-key"
    yield
    del os.environ["ANTHROPIC_API_KEY"]
    del os.environ["OPENAI_API_KEY"]
