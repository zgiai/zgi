import os
import sys
import pytest
from fastapi.testclient import TestClient
from sqlalchemy import create_engine
from sqlalchemy.orm import sessionmaker
from sqlalchemy.pool import StaticPool

# Add project root directory to Python path
sys.path.append(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from app.core.database import Base, get_db
from app.core.config import settings
from app.features.users.models import User
from app.core.security import get_password_hash

def get_test_db():
    """Get a database connection"""
    # Create a new engine using MySQL with PyMySQL
    test_db_url = settings.SQLALCHEMY_DATABASE_URL.replace("+aiomysql", "+pymysql")
    
    engine = create_engine(
        test_db_url,
        echo=True  # Enable SQL logging for debugging
    )

    # Drop tables in correct order to handle foreign key constraints
    tables = [
        "knowledge_document_chunks",  # Drop child tables first
        "knowledge_documents",
        "knowledge_bases",
        "api_key_mappings",
        "api_keys",
        "user_quotas",
        "usage_logs",
        "chat_files",
        "chat_sessions",
        "applications",
        "organization_member_roles",
        "organization_members",
        "projects",
        "roles",
        "organizations",
        "users"
    ]
    
    connection = engine.connect()
    for table in tables:
        try:
            connection.execute(f"DROP TABLE IF EXISTS {table}")
        except:
            pass  # Ignore errors if table doesn't exist
    connection.commit()
    connection.close()

    # Create all tables
    Base.metadata.create_all(bind=engine)

    # Create a new session factory
    TestingSessionLocal = sessionmaker(
        autocommit=False,
        autoflush=False,
        bind=engine,
        expire_on_commit=False
    )

    # Create a new session
    return TestingSessionLocal()

@pytest.fixture(scope="function")
def db():
    """Create a test database session"""
    db = get_test_db()
    try:
        yield db
    finally:
        db.close()

@pytest.fixture(scope="function")
def client(db):
    """Create a test client with database dependency override"""
    def override_get_db():
        try:
            yield db
        finally:
            db.rollback()
    
    from app.main import app
    app.dependency_overrides[get_db] = override_get_db
    with TestClient(app) as test_client:
        yield test_client
    app.dependency_overrides.clear()

@pytest.fixture
def test_user(db):
    """Create a test user and return user info with access token"""
    user = User(
        email="test@example.com",
        username="testuser",
        hashed_password=get_password_hash("testpassword"),
        is_active=True,
        is_admin=True,
        is_superuser=True
    )
    db.add(user)
    db.commit()
    db.refresh(user)
    yield user
    db.delete(user)
    db.commit()

@pytest.fixture
def test_user_headers(test_user):
    """Return headers with test user token"""
    return {"Authorization": f"Bearer {test_user.id}"}
