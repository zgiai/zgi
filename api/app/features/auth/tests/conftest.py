import pytest
from sqlalchemy import create_engine
from sqlalchemy.orm import sessionmaker
from fastapi.testclient import TestClient
from app.core.database import Base, get_db
from app.main import app
from app.core.config import settings
from app.features.users.models import User
from app.features.auth.service import AuthService

# Test database URL
TEST_DATABASE_URL = "sqlite:///./test.db"

@pytest.fixture(scope="session")
def engine():
    """Create a test database engine"""
    engine = create_engine(TEST_DATABASE_URL, connect_args={"check_same_thread": False})
    Base.metadata.create_all(bind=engine)
    yield engine
    Base.metadata.drop_all(bind=engine)

@pytest.fixture(scope="function")
def db_session(engine):
    """Create a test database session"""
    Session = sessionmaker(autocommit=False, autoflush=False, bind=engine)
    session = Session()
    
    # Clear all tables before each test
    for table in reversed(Base.metadata.sorted_tables):
        session.execute(table.delete())
    session.commit()
    
    try:
        yield session
    finally:
        session.rollback()
        session.close()

@pytest.fixture(scope="function")
def client(db_session):
    """Create a test client"""
    app.dependency_overrides[get_db] = lambda: db_session
    yield TestClient(app)
    app.dependency_overrides.clear()

@pytest.fixture
def auth_service(db_session):
    """Create an auth service instance"""
    return AuthService(db_session)

@pytest.fixture
def test_user(db_session, auth_service):
    """Create a test user"""
    user = auth_service.register_user(
        email="test@example.com",
        username="testuser",
        password="testpass123"
    )
    return user

@pytest.fixture
def superuser(db_session):
    """Create a superuser"""
    user = User(
        email="admin@example.com",
        username="admin",
        hashed_password="$2b$12$LQv3c1yqBWVHxkd0LHAkCOYz6TtxMQJqhN8/LewKyNiGpJ4vz6em2",  # "adminpass123"
        is_superuser=True,
        is_active=True,
        is_verified=True
    )
    db_session.add(user)
    db_session.commit()
    db_session.refresh(user)
    return user

@pytest.fixture
def superuser_headers(superuser, auth_service, db_session):
    """Create headers with superuser token"""
    auth_service.db = db_session  # Ensure db session is set
    token = auth_service.create_access_token(superuser.id)
    return {"Authorization": f"Bearer {token}"}
