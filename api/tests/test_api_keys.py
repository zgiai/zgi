import pytest
from datetime import datetime, timedelta
from fastapi import status

from app.features.auth.client.service import AuthClientService
from app.models.api_keys import APIKey
from app.features.users.models import User


def test_create_api_key(test_db):
    """Test creating an API key"""
    # Create a test user
    test_user = User(
        email="test@example.com",
        username="testuser",
        hashed_password="testpass",
        is_active=True
    )
    test_db.add(test_user)
    test_db.commit()
    test_db.refresh(test_user)

    # Create auth service
    auth_service = AuthClientService(test_db)

    # Create API key
    api_key = auth_service.create_api_key(
        user_id=test_user.id,
        name="Test API Key"
    )

    assert api_key.key.startswith("zgi_")
    assert api_key.user_id == test_user.id
    assert api_key.name == "Test API Key"
    assert api_key.is_active is True
    assert api_key.expires_at is None


def test_validate_api_key(test_db):
    """Test validating an API key"""
    # Create a test user
    test_user = User(
        email="test@example.com",
        username="testuser",
        hashed_password="testpass",
        is_active=True
    )
    test_db.add(test_user)
    test_db.commit()
    test_db.refresh(test_user)

    # Create auth service
    auth_service = AuthClientService(test_db)

    # Create API key
    api_key = auth_service.create_api_key(
        user_id=test_user.id,
        name="Test API Key"
    )

    # Validate API key
    validated_key = auth_service.validate_api_key(api_key.key)
    assert validated_key.id == api_key.id

    # Test invalid API key
    with pytest.raises(Exception) as exc:
        auth_service.validate_api_key("invalid_key")
    assert exc.value.status_code == status.HTTP_401_UNAUTHORIZED


def test_expired_api_key(test_db):
    """Test expired API key validation"""
    # Create a test user
    test_user = User(
        email="test@example.com",
        username="testuser",
        hashed_password="testpass",
        is_active=True
    )
    test_db.add(test_user)
    test_db.commit()
    test_db.refresh(test_user)

    # Create auth service
    auth_service = AuthClientService(test_db)

    # Create expired API key
    expired_time = datetime.utcnow() - timedelta(days=1)
    api_key = auth_service.create_api_key(
        user_id=test_user.id,
        name="Test API Key",
        expires_at=expired_time
    )

    # Test expired API key
    with pytest.raises(Exception) as exc:
        auth_service.validate_api_key(api_key.key)
    assert exc.value.status_code == status.HTTP_401_UNAUTHORIZED
    assert "expired" in str(exc.value.detail).lower()


def test_create_access_token_for_api_key(test_db):
    """Test creating an access token from an API key"""
    # Create a test user
    test_user = User(
        email="test@example.com",
        username="testuser",
        hashed_password="testpass",
        is_active=True
    )
    test_db.add(test_user)
    test_db.commit()
    test_db.refresh(test_user)

    # Create auth service
    auth_service = AuthClientService(test_db)

    # Create API key
    api_key = auth_service.create_api_key(
        user_id=test_user.id,
        name="Test API Key"
    )

    # Create access token from API key
    token_response = auth_service.create_access_token_for_api_key(api_key.key)
    assert "access_token" in token_response
    assert token_response["token_type"] == "bearer"

    # Verify the token works for authentication
    user = auth_service.get_current_user(token_response["access_token"])
    assert user.id == test_user.id
