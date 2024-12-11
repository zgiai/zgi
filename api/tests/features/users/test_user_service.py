import pytest
from datetime import datetime
from unittest.mock import Mock, AsyncMock

from app.features.users.service import UserService
from app.features.users.schemas import UserProfileUpdate, UserPreferences
from app.models.user import User


@pytest.fixture
def mock_db():
    return Mock()


@pytest.fixture
def mock_cache():
    cache = AsyncMock()
    cache.get.return_value = None
    return cache


@pytest.fixture
def mock_email_service():
    return AsyncMock()


@pytest.fixture
def user_service(mock_db, mock_cache, mock_email_service):
    return UserService(mock_db, mock_cache, mock_email_service)


@pytest.fixture
def test_user():
    return User(
        id=1,
        email="test@example.com",
        full_name="Test User",
        is_active=True,
        is_verified=True,
        created_at=datetime.utcnow(),
    )


@pytest.mark.asyncio
async def test_get_user_profile(user_service, test_user, mock_db, mock_cache):
    # Setup
    mock_db.query.return_value.filter.return_value.first.return_value = test_user

    # Test
    user = await user_service.get_user_profile(1)

    # Verify
    assert user == test_user
    mock_cache.get.assert_called_once_with("user_profile:1")
    mock_cache.set.assert_called_once_with("user_profile:1", test_user, expire=3600)


@pytest.mark.asyncio
async def test_update_profile(user_service, test_user, mock_db):
    # Setup
    mock_db.query.return_value.filter.return_value.first.return_value = test_user
    update_data = UserProfileUpdate(full_name="Updated Name")

    # Test
    updated_user = await user_service.update_profile(1, update_data)

    # Verify
    assert updated_user.full_name == "Updated Name"
    mock_db.commit.assert_called_once()


@pytest.mark.asyncio
async def test_update_preferences(user_service, test_user, mock_db):
    # Setup
    mock_db.query.return_value.filter.return_value.first.return_value = test_user
    preferences = UserPreferences(theme="dark", language="es")

    # Test
    updated_user = await user_service.update_preferences(1, preferences)

    # Verify
    assert updated_user.preferences == preferences.dict()
    mock_db.commit.assert_called_once()


@pytest.mark.asyncio
async def test_request_password_reset(user_service, test_user, mock_db, mock_email_service):
    # Setup
    mock_db.query.return_value.filter.return_value.first.return_value = test_user

    # Test
    success = await user_service.request_password_reset("test@example.com")

    # Verify
    assert success is True
    mock_email_service.send_password_reset_email.assert_called_once()


@pytest.mark.asyncio
async def test_reset_password(user_service, test_user, mock_db, mock_cache):
    # Setup
    mock_db.query.return_value.filter.return_value.first.return_value = test_user
    mock_cache.get.return_value = "valid_token"

    # Test
    success = await user_service.reset_password(
        "test@example.com", "valid_token", "new_password"
    )

    # Verify
    assert success is True
    mock_db.commit.assert_called_once()
    mock_cache.delete.assert_called_once_with("password_reset:test@example.com")


@pytest.mark.asyncio
async def test_verify_email(user_service, test_user, mock_db, mock_cache):
    # Setup
    mock_db.query.return_value.filter.return_value.first.return_value = test_user
    mock_cache.get.return_value = "valid_token"

    # Test
    success = await user_service.verify_email("test@example.com", "valid_token")

    # Verify
    assert success is True
    assert test_user.is_verified is True
    mock_db.commit.assert_called_once()
    mock_cache.delete.assert_called_once_with("email_verification:test@example.com")
