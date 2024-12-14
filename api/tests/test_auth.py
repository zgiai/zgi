import pytest
from fastapi import HTTPException
from fastapi.security import OAuth2PasswordRequestForm
from fastapi.testclient import TestClient
from sqlalchemy import create_engine, text
from sqlalchemy.orm import sessionmaker
from datetime import timedelta
import time
import jwt

from app.main import app
from app.core.database import Base
from app.db.session import get_db
from app.core.config import settings
from app.features.auth.client.service import AuthClientService
from app.features.users.models import User
from fastapi import status

# Test database configuration
SQLALCHEMY_DATABASE_URL = "mysql://root@localhost/zgi"

engine = create_engine(
    SQLALCHEMY_DATABASE_URL,
    pool_pre_ping=True,
)
TestingSessionLocal = sessionmaker(autocommit=False, autoflush=False, bind=engine)

def override_get_db():
    try:
        db = TestingSessionLocal()
        yield db
    finally:
        db.close()

app.dependency_overrides[get_db] = override_get_db

@pytest.fixture
def client():
    # Clean up data before test
    with engine.connect() as conn:
        tables = ["users", "chat_sessions", "chat_files", "applications", "team_members", "teams", "team_invitations"]
        for table in tables:
            conn.execute(text(f"DELETE FROM {table}"))
        conn.commit()
    
    yield TestClient(app)
    
    # Clean up data after test
    with engine.connect() as conn:
        for table in tables:
            conn.execute(text(f"DELETE FROM {table}"))
        conn.commit()

@pytest.fixture
def db_session():
    # Clean up data before test
    with engine.connect() as conn:
        tables = ["users", "chat_sessions", "chat_files", "applications", "team_members", "teams", "team_invitations"]
        for table in tables:
            conn.execute(text(f"DELETE FROM {table}"))
        conn.commit()
    
    db = TestingSessionLocal()
    try:
        yield db
    finally:
        db.close()
        # Clean up data after test
        with engine.connect() as conn:
            for table in tables:
                conn.execute(text(f"DELETE FROM {table}"))
            conn.commit()

def test_login_success(db_session):
    # Create a test user
    auth_service = AuthClientService(db_session)
    test_user = User(
        email="test@example.com",
        username="testuser",
        hashed_password=auth_service.get_password_hash("testpass123")
    )
    db_session.add(test_user)
    db_session.commit()

    # Test login
    form_data = OAuth2PasswordRequestForm(
        username="test@example.com",
        password="testpass123",
        scope=""
    )
    result = auth_service.login(form_data.username, form_data.password)
    
    assert result["access_token"] is not None
    assert result["token_type"] == "bearer"


def test_login_invalid_password(db_session):
    # Create a test user
    auth_service = AuthClientService(db_session)
    test_user = User(
        email="test@example.com",
        username="testuser",
        hashed_password=auth_service.get_password_hash("testpass123")
    )
    db_session.add(test_user)
    db_session.commit()

    # Test login with wrong password
    with pytest.raises(HTTPException) as exc_info:
        form_data = OAuth2PasswordRequestForm(
            username="test@example.com",
            password="wrongpass",
            scope=""
        )
        auth_service.login(form_data.username, form_data.password)
    
    assert exc_info.value.status_code == 401
    assert "Incorrect email or password" in str(exc_info.value.detail)


def test_login_nonexistent_user(db_session):
    auth_service = AuthClientService(db_session)
    
    # Test login with non-existent user
    with pytest.raises(HTTPException) as exc_info:
        form_data = OAuth2PasswordRequestForm(
            username="nonexistent@example.com",
            password="testpass123",
            scope=""
        )
        auth_service.login(form_data.username, form_data.password)
    
    assert exc_info.value.status_code == 401
    assert "Incorrect email or password" in str(exc_info.value.detail)


def test_get_current_user_success(db_session):
    # Create a test user
    auth_service = AuthClientService(db_session)
    test_user = User(
        email="test@example.com",
        username="testuser",
        hashed_password=auth_service.get_password_hash("testpass123")
    )
    db_session.add(test_user)
    db_session.commit()

    # Login to get token
    form_data = OAuth2PasswordRequestForm(
        username="test@example.com",
        password="testpass123",
        scope=""
    )
    result = auth_service.login(form_data.username, form_data.password)
    token = result["access_token"]

    # Test get current user
    user = auth_service.get_current_user(token)
    assert user.email == "test@example.com"
    assert user.username == "testuser"


def test_get_current_user_invalid_token(db_session):
    auth_service = AuthClientService(db_session)
    
    # Test with invalid token
    with pytest.raises(HTTPException) as exc_info:
        auth_service.get_current_user("invalid_token")
    
    assert exc_info.value.status_code == 401
    assert "Could not validate credentials" in str(exc_info.value.detail)


def test_login_inactive_user(db_session):
    # Create an inactive test user
    auth_service = AuthClientService(db_session)
    test_user = User(
        email="inactive@example.com",
        username="inactiveuser",
        hashed_password=auth_service.get_password_hash("testpass123"),
        is_active=False
    )
    db_session.add(test_user)
    db_session.commit()

    # Test login with inactive user
    with pytest.raises(HTTPException) as exc_info:
        auth_service.login("inactive@example.com", "testpass123")
    
    assert exc_info.value.status_code == 401
    assert "Inactive user" in str(exc_info.value.detail)


def test_login_superuser(db_session):
    # Create a superuser
    auth_service = AuthClientService(db_session)
    test_user = User(
        email="admin@example.com",
        username="admin",
        hashed_password=auth_service.get_password_hash("admin123"),
        is_superuser=True
    )
    db_session.add(test_user)
    db_session.commit()

    # Test login
    result = auth_service.login("admin@example.com", "admin123")
    
    assert result["access_token"] is not None
    assert result["token_type"] == "bearer"

    # Verify the user is actually a superuser
    current_user = auth_service.get_current_user(result["access_token"])
    assert current_user.is_superuser is True


def test_token_expiration(db_session):
    # Create a test user
    auth_service = AuthClientService(db_session)
    test_user = User(
        email="test@example.com",
        username="testuser",
        hashed_password=auth_service.get_password_hash("testpass123")
    )
    db_session.add(test_user)
    db_session.commit()

    # Create a token with a very short expiration time (1 second)
    token = auth_service.create_access_token(
        data={"sub": test_user.email},
        expires_delta=timedelta(seconds=1)
    )

    # Wait for token to expire
    time.sleep(1.1)  # Wait slightly longer than expiration

    # Try to verify the expired token
    with pytest.raises(HTTPException) as exc_info:
        auth_service.verify_token(token)
    
    assert exc_info.value.status_code == status.HTTP_401_UNAUTHORIZED
    assert "Token has expired" in str(exc_info.value.detail)


def test_password_validation(db_session):
    auth_service = AuthClientService(db_session)
    
    # Test various invalid passwords
    invalid_passwords = [
        "",  # Empty password
        "short",  # Too short
        "no_numbers",  # No numbers
        "12345678",  # Only numbers
    ]
    
    for password in invalid_passwords:
        with pytest.raises(HTTPException) as exc_info:
            auth_service.validate_password(password)
        assert exc_info.value.status_code == 400

    # Test valid password
    valid_password = "ValidPass123"
    # Should not raise any exception
    auth_service.validate_password(valid_password)


def test_refresh_token(db_session):
    # Create a test user
    auth_service = AuthClientService(db_session)
    test_user = User(
        email="test@example.com",
        username="testuser",
        hashed_password=auth_service.get_password_hash("testpass123")
    )
    db_session.add(test_user)
    db_session.commit()

    # Login to get initial token
    result = auth_service.login("test@example.com", "testpass123")
    old_token = result["access_token"]

    # Refresh token
    new_token = auth_service.refresh_token(old_token)
    assert new_token != old_token

    # Verify new token works
    user = auth_service.get_current_user(new_token)
    assert user.email == "test@example.com"


def test_register_user(db_session):
    auth_service = AuthClientService(db_session)
    
    # Register a new user
    user = auth_service.register_user(
        email="newuser@example.com",
        username="newuser",
        password="Password123"
    )
    
    assert user.email == "newuser@example.com"
    assert user.username == "newuser"
    assert user.is_active is True
    assert user.is_superuser is False

    # Verify we can login with the new user
    result = auth_service.login("newuser@example.com", "Password123")
    assert result["access_token"] is not None


def test_register_duplicate_email(db_session):
    auth_service = AuthClientService(db_session)
    
    # Register first user
    auth_service.register_user(
        email="test@example.com",
        username="testuser1",
        password="Password123"
    )
    
    # Try to register second user with same email
    with pytest.raises(HTTPException) as exc_info:
        auth_service.register_user(
            email="test@example.com",
            username="testuser2",
            password="Password123"
        )
    
    assert exc_info.value.status_code == 400
    assert "Email already registered" in str(exc_info.value.detail)


def test_register_duplicate_username(db_session):
    auth_service = AuthClientService(db_session)
    
    # Register first user
    auth_service.register_user(
        email="test1@example.com",
        username="testuser",
        password="Password123"
    )
    
    # Try to register second user with same username
    with pytest.raises(HTTPException) as exc_info:
        auth_service.register_user(
            email="test2@example.com",
            username="testuser",
            password="Password123"
        )
    
    assert exc_info.value.status_code == 400
    assert "Username already taken" in str(exc_info.value.detail)


def test_change_password(db_session):
    auth_service = AuthClientService(db_session)
    
    # Create a test user
    test_user = User(
        email="test@example.com",
        username="testuser",
        hashed_password=auth_service.get_password_hash("oldpass123")
    )
    db_session.add(test_user)
    db_session.commit()

    # Change password
    auth_service.change_password(
        user=test_user,
        old_password="oldpass123",
        new_password="newpass123"
    )

    # Try to login with old password
    with pytest.raises(HTTPException) as exc_info:
        auth_service.login("test@example.com", "oldpass123")
    assert exc_info.value.status_code == 401

    # Login with new password should work
    result = auth_service.login("test@example.com", "newpass123")
    assert result["access_token"] is not None


def test_change_password_wrong_old_password(db_session):
    auth_service = AuthClientService(db_session)
    
    # Create a test user
    test_user = User(
        email="test@example.com",
        username="testuser",
        hashed_password=auth_service.get_password_hash("testpass123")
    )
    db_session.add(test_user)
    db_session.commit()

    # Try to change password with wrong old password
    with pytest.raises(HTTPException) as exc_info:
        auth_service.change_password(
            user=test_user,
            old_password="wrongpass",
            new_password="newpass123"
        )
    
    assert exc_info.value.status_code == 401
    assert "Incorrect password" in str(exc_info.value.detail)
