import pytest
from fastapi import status
from app.features.auth.console.schemas import UserRegister, UserLogin

def test_user_registration_success(client, test_db):
    """Test successful user registration"""
    user_data = UserRegister(
        email="newuser@example.com",  # Use a different email
        password="Test123!@#",
        username="newuser"  # Use a different username
    )
    
    response = client.post("/v1/console/auth/register", json=user_data.model_dump())
    assert response.status_code == status.HTTP_201_CREATED
    assert "message" in response.json()
    assert "user" in response.json()
    assert response.json()["user"]["email"] == user_data.email

def test_user_registration_duplicate_email(client, test_user):
    """Test registration with existing email"""
    user_data = UserRegister(
        email=test_user["email"],  # Use email from existing test user
        password="Test123!@#",
        username="newuser"  # Use a different username
    )
    
    response = client.post("/v1/console/auth/register", json=user_data.model_dump())
    assert response.status_code == status.HTTP_400_BAD_REQUEST

def test_user_login_success(client, test_user):
    """Test successful user login"""
    login_data = UserLogin(
        email=test_user["email"],
        password="testpassword"  # Use the password from test_user fixture
    )
    
    response = client.post("/v1/console/auth/login", json=login_data.model_dump())
    assert response.status_code == status.HTTP_200_OK
    assert "access_token" in response.json()
    assert "token_type" in response.json()
    assert response.json()["token_type"] == "bearer"

def test_user_login_invalid_credentials(client):
    """Test login with invalid credentials"""
    login_data = UserLogin(
        email="wrong@example.com",
        password="wrongpassword"
    )
    
    response = client.post("/v1/console/auth/login", json=login_data.model_dump())
    assert response.status_code == status.HTTP_401_UNAUTHORIZED

def test_console_protected_endpoint_without_token(client):
    """Test accessing protected endpoint without token"""
    response = client.get("/v1/console/auth/protected-resource")
    assert response.status_code == status.HTTP_401_UNAUTHORIZED

def test_console_protected_endpoint_with_token(client, test_user_headers):
    """Test accessing protected endpoint with valid token"""
    response = client.get(
        "/v1/console/auth/protected-resource",
        headers=test_user_headers
    )
    assert response.status_code == status.HTTP_200_OK

def test_password_validation(client):
    """Test password validation rules"""
    user_data = UserRegister(
        email="test@example.com",
        password="weak",  # Too short and simple
        username="testuser"
    )
    
    response = client.post("/v1/console/auth/register", json=user_data.model_dump())
    assert response.status_code == status.HTTP_400_BAD_REQUEST

def test_email_format_validation(client):
    """Test email format validation"""
    try:
        user_data = UserRegister(
            email="invalid-email",  # Invalid email format
            password="Test123!@#",
            username="testuser"
        )
        assert False, "Should raise ValidationError"
    except Exception as e:
        assert "value is not a valid email address" in str(e)
