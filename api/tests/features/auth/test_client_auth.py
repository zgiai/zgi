import pytest
from fastapi import status
from app.features.auth.client.schemas import UserLogin
from app.features.auth.client.models import APIKey

def test_client_login_success(client, test_db):
    """Test successful client login"""
    # Create API key for client
    api_key = APIKey(
        name="Test Client",
        key="test-client-key",
        is_active=True
    )
    test_db.add(api_key)
    test_db.commit()
    test_db.refresh(api_key)

    # Attempt login
    response = client.post(
        "/v1/client/auth/login",
        headers={"X-API-Key": api_key.key}
    )
    assert response.status_code == status.HTTP_200_OK
    assert "access_token" in response.json()
    assert "token_type" in response.json()
    assert response.json()["token_type"] == "bearer"

def test_client_login_invalid_credentials(client):
    """Test client login with invalid API key"""
    response = client.post(
        "/v1/client/auth/login",
        headers={"X-API-Key": "invalid-key"}
    )
    assert response.status_code == status.HTTP_401_UNAUTHORIZED

def test_client_protected_endpoint_without_token(client):
    """Test accessing protected endpoint without token"""
    response = client.get("/v1/client/auth/protected-resource")
    assert response.status_code == status.HTTP_401_UNAUTHORIZED

def test_client_protected_endpoint_with_token(client, test_db):
    """Test accessing protected endpoint with valid token"""
    # Create API key for client
    api_key = APIKey(
        name="Test Client",
        key="test-client-key",
        is_active=True
    )
    test_db.add(api_key)
    test_db.commit()
    test_db.refresh(api_key)

    # Get access token
    response = client.post(
        "/v1/client/auth/login",
        headers={"X-API-Key": api_key.key}
    )
    token = response.json()["access_token"]

    # Access protected endpoint
    response = client.get(
        "/v1/client/auth/protected-resource",
        headers={"Authorization": f"Bearer {token}"}
    )
    assert response.status_code == status.HTTP_200_OK
    assert "message" in response.json()
    assert "api_key_id" in response.json()

def test_client_login_rate_limit(client, test_db):
    """Test client login rate limiting"""
    # Create API key for client
    api_key = APIKey(
        name="Test Client",
        key="test-client-key",
        is_active=True
    )
    test_db.add(api_key)
    test_db.commit()
    test_db.refresh(api_key)

    # Make multiple login attempts
    for _ in range(5):
        response = client.post(
            "/v1/client/auth/login",
            headers={"X-API-Key": api_key.key}
        )
        assert response.status_code == status.HTTP_200_OK

    # Next attempt should be rate limited
    response = client.post(
        "/v1/client/auth/login",
        headers={"X-API-Key": api_key.key}
    )
    assert response.status_code == status.HTTP_429_TOO_MANY_REQUESTS
