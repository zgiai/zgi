import pytest
from fastapi import HTTPException
from app.features.auth.service import AuthService
from app.features.users.models import User

def test_register_user(db_session):
    """Test user registration"""
    auth_service = AuthService(db_session)
    user = auth_service.register_user(
        email="test@example.com",
        username="testuser",
        password="testpass123"
    )
    assert user.email == "test@example.com"
    assert user.username == "testuser"
    assert user.is_active is True
    assert user.is_verified is True

def test_register_duplicate_email(db_session):
    """Test registration with duplicate email"""
    auth_service = AuthService(db_session)
    auth_service.register_user(
        email="test@example.com",
        username="testuser1",
        password="testpass123"
    )
    
    with pytest.raises(HTTPException) as exc_info:
        auth_service.register_user(
            email="test@example.com",
            username="testuser2",
            password="testpass123"
        )
    assert exc_info.value.status_code == 400
    assert "Email already registered" in str(exc_info.value.detail)

def test_register_duplicate_username(db_session):
    """Test registration with duplicate username"""
    auth_service = AuthService(db_session)
    auth_service.register_user(
        email="test1@example.com",
        username="testuser",
        password="testpass123"
    )
    
    with pytest.raises(HTTPException) as exc_info:
        auth_service.register_user(
            email="test2@example.com",
            username="testuser",
            password="testpass123"
        )
    assert exc_info.value.status_code == 400
    assert "Username already taken" in str(exc_info.value.detail)

def test_authenticate_user(db_session):
    """Test user authentication"""
    auth_service = AuthService(db_session)
    auth_service.register_user(
        email="test@example.com",
        username="testuser",
        password="testpass123"
    )
    
    result = auth_service.authenticate_user(
        email="test@example.com",
        password="testpass123"
    )
    assert "access_token" in result
    assert result["token_type"] == "bearer"
    assert result["user"]["email"] == "test@example.com"
    assert result["user"]["username"] == "testuser"

def test_authenticate_invalid_credentials(db_session):
    """Test authentication with invalid credentials"""
    auth_service = AuthService(db_session)
    auth_service.register_user(
        email="test@example.com",
        username="testuser",
        password="testpass123"
    )
    
    with pytest.raises(HTTPException) as exc_info:
        auth_service.authenticate_user(
            email="test@example.com",
            password="wrongpass"
        )
    assert exc_info.value.status_code == 401
    assert "Incorrect email or password" in str(exc_info.value.detail)

def test_get_current_user(db_session):
    """Test getting current user from token"""
    auth_service = AuthService(db_session)
    user = auth_service.register_user(
        email="test@example.com",
        username="testuser",
        password="testpass123"
    )
    
    token = auth_service.create_access_token(user.id)
    current_user = auth_service.get_current_user(token)
    assert current_user.id == user.id
    assert current_user.email == user.email
    assert current_user.username == user.username

def test_get_current_user_invalid_token(db_session):
    """Test getting current user with invalid token"""
    auth_service = AuthService(db_session)
    
    with pytest.raises(HTTPException) as exc_info:
        auth_service.get_current_user("invalid_token")
    assert exc_info.value.status_code == 401
    assert "Could not validate credentials" in str(exc_info.value.detail)

def test_delete_user(db_session):
    """Test deleting a user"""
    auth_service = AuthService(db_session)
    # Create a test user
    user = auth_service.register_user(
        email="test@example.com",
        username="testuser",
        password="testpass123"
    )
    
    # Delete the user
    deleted_user = auth_service.delete_user(user.id)
    assert deleted_user.id == user.id
    
    # Verify user is deleted
    assert db_session.query(User).filter(User.id == user.id).first() is None

def test_delete_nonexistent_user(db_session):
    """Test deleting a nonexistent user"""
    auth_service = AuthService(db_session)
    
    with pytest.raises(HTTPException) as exc_info:
        auth_service.delete_user(999)  # Non-existent user ID
    
    assert exc_info.value.status_code == 404
    assert "User not found" in str(exc_info.value.detail)

def test_delete_user_as_regular_user(db_session):
    """Test deleting a user without admin privileges"""
    auth_service = AuthService(db_session)
    # Create a regular user
    regular_user = auth_service.register_user(
        email="regular@example.com",
        username="regular",
        password="testpass123"
    )
    
    # Create another user to be deleted
    target_user = auth_service.register_user(
        email="target@example.com",
        username="target",
        password="testpass123"
    )
    
    with pytest.raises(HTTPException) as exc_info:
        auth_service.delete_user(target_user.id, current_user=regular_user)
    
    assert exc_info.value.status_code == 403
    assert "Insufficient permissions" in str(exc_info.value.detail)
