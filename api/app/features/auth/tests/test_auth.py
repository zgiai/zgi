import pytest
from fastapi.testclient import TestClient
from sqlalchemy.orm import Session
import jwt
from datetime import datetime, timedelta

from app.core.config import settings
from app.features.users.models import User
from app.features.auth.service import AuthService
from app.main import app

# Test data
TEST_USER_EMAIL = "test@example.com"
TEST_USER_USERNAME = "testuser"
TEST_USER_PASSWORD = "testpassword"

@pytest.fixture
def auth_service(db_session: Session):
    return AuthService(db_session)

@pytest.fixture
def test_user(db_session: Session):
    """Create a test user"""
    auth_service = AuthService(db_session)
    user = auth_service.register_user(
        email=TEST_USER_EMAIL,
        username=TEST_USER_USERNAME,
        password=TEST_USER_PASSWORD
    )
    db_session.commit()
    return user

@pytest.fixture
def test_superuser(db_session: Session):
    """Create a test superuser"""
    auth_service = AuthService(db_session)
    user = auth_service.register_user(
        email="admin@example.com",
        username="admin",
        password="password"
    )
    user.is_superuser = True
    db_session.commit()
    return user

@pytest.fixture
def superuser_token(test_superuser, auth_service):
    """Create a token for superuser"""
    return auth_service.create_access_token(test_superuser.id)

@pytest.fixture
def superuser_headers(superuser_token):
    """Headers with superuser token"""
    return {"Authorization": f"Bearer {superuser_token}"}

def test_register_user(client: TestClient, db_session: Session):
    """Test user registration"""
    response = client.post(
        "/v1/register",
        json={
            "email": "newuser@example.com",
            "username": "newuser",
            "password": "newpassword"
        }
    )
    assert response.status_code == 201
    data = response.json()
    assert data["email"] == "newuser@example.com"
    assert data["username"] == "newuser"
    
    # Verify user exists in database
    user = db_session.query(User).filter(User.email == "newuser@example.com").first()
    assert user is not None
    assert user.username == "newuser"

def test_register_duplicate_email(client: TestClient, test_user, db_session: Session):
    """Test registration with duplicate email"""
    response = client.post(
        "/v1/register",
        json={
            "email": TEST_USER_EMAIL,
            "username": "another_user",
            "password": "password123"
        }
    )
    assert response.status_code == 400
    assert "Email already registered" in response.json()["detail"]

def test_register_duplicate_username(client: TestClient, test_user, db_session: Session):
    """Test registration with duplicate username"""
    response = client.post(
        "/v1/register",
        json={
            "email": "another@example.com",
            "username": TEST_USER_USERNAME,
            "password": "password123"
        }
    )
    assert response.status_code == 400
    assert "Username already taken" in response.json()["detail"]

def test_login_json(client: TestClient, test_user, db_session: Session):
    """Test JSON format login"""
    response = client.post(
        "/v1/login",
        json={
            "email": TEST_USER_EMAIL,
            "password": TEST_USER_PASSWORD
        }
    )
    assert response.status_code == 200
    data = response.json()
    assert "access_token" in data
    assert data["token_type"] == "bearer"
    assert "user" in data
    assert data["user"]["email"] == TEST_USER_EMAIL

def test_login_form(client: TestClient, test_user, db_session: Session):
    """Test form login (OAuth2)"""
    response = client.post(
        "/v1/token",
        data={
            "username": TEST_USER_EMAIL,
            "password": TEST_USER_PASSWORD,
            "grant_type": "password"
        }
    )
    assert response.status_code == 200
    data = response.json()
    assert "access_token" in data
    assert data["token_type"] == "bearer"

def test_login_invalid_credentials(client: TestClient, test_user, db_session: Session):
    """Test login with invalid credentials"""
    response = client.post(
        "/v1/login",
        json={
            "email": TEST_USER_EMAIL,
            "password": "wrongpassword"
        }
    )
    assert response.status_code == 401
    assert "Incorrect email or password" in response.json()["detail"]

def test_get_current_user(client: TestClient, test_user, auth_service, db_session: Session):
    """Test getting current user info"""
    token = auth_service.create_access_token(test_user.id)
    headers = {"Authorization": f"Bearer {token}"}
    
    response = client.get("/v1/me", headers=headers)
    assert response.status_code == 200
    data = response.json()
    assert data["email"] == TEST_USER_EMAIL
    assert data["username"] == TEST_USER_USERNAME

def test_get_current_user_invalid_token(client: TestClient, db_session: Session):
    """Test getting current user with invalid token"""
    headers = {"Authorization": "Bearer invalid_token"}
    response = client.get("/v1/me", headers=headers)
    assert response.status_code == 401

def test_list_users_as_admin(client: TestClient, test_user, superuser_headers, db_session: Session):
    """Test listing users as admin"""
    response = client.get("/v1/users", headers=superuser_headers)
    assert response.status_code == 200
    users = response.json()
    assert len(users) > 0
    assert any(user["email"] == TEST_USER_EMAIL for user in users)

def test_list_users_as_regular_user(client: TestClient, test_user, auth_service, db_session: Session):
    """Test listing users as regular user (should fail)"""
    token = auth_service.create_access_token(test_user.id)
    headers = {"Authorization": f"Bearer {token}"}
    
    response = client.get("/v1/users", headers=headers)
    assert response.status_code == 403

def test_get_user_as_admin(client: TestClient, test_user, superuser_headers, db_session: Session):
    """Test getting specific user as admin"""
    response = client.get(f"/v1/users/{test_user.id}", headers=superuser_headers)
    assert response.status_code == 200
    data = response.json()
    assert data["email"] == TEST_USER_EMAIL
    assert data["username"] == TEST_USER_USERNAME

def test_delete_user_as_admin(client: TestClient, test_user, superuser_headers, db_session: Session):
    """Test deleting a user as admin"""
    response = client.delete(f"/v1/users/{test_user.id}", headers=superuser_headers)
    assert response.status_code == 200
    assert response.json()["id"] == test_user.id
    
    # Verify user is deleted
    assert db_session.query(User).filter(User.id == test_user.id).first() is None

def test_delete_nonexistent_user_as_admin(client: TestClient, superuser_headers, db_session: Session):
    """Test deleting a nonexistent user as admin"""
    response = client.delete("/v1/users/999", headers=superuser_headers)
    assert response.status_code == 404
    assert "User not found" in response.json()["detail"]

def test_delete_user_as_regular_user(client: TestClient, test_user, auth_service, db_session: Session):
    """Test deleting a user as a regular user (should fail)"""
    # Create another user to be deleted
    target_user = auth_service.register_user(
        email="target@example.com",
        username="target",
        password="testpass123"
    )
    
    # Get token for regular user
    token = auth_service.create_access_token(test_user.id)
    headers = {"Authorization": f"Bearer {token}"}
    
    response = client.delete(f"/v1/users/{target_user.id}", headers=headers)
    assert response.status_code == 403
    assert "Insufficient permissions" in response.json()["detail"]
    
    # Verify target user still exists
    assert db_session.query(User).filter(User.id == target_user.id).first() is not None

def test_delete_user_without_auth(client: TestClient, test_user, db_session: Session):
    """Test deleting a user without authentication"""
    response = client.delete(f"/v1/users/{test_user.id}")
    assert response.status_code == 401
    assert "Not authenticated" in response.json()["detail"]

def test_token_expiration(client: TestClient, test_user, db_session: Session):
    """Test token expiration"""
    # Create an expired token
    expired_payload = {
        "sub": str(test_user.id),
        "exp": datetime.utcnow() - timedelta(minutes=1)
    }
    expired_token = jwt.encode(expired_payload, settings.SECRET_KEY, algorithm=settings.ALGORITHM)
    headers = {"Authorization": f"Bearer {expired_token}"}
    
    response = client.get("/v1/me", headers=headers)
    assert response.status_code == 401
