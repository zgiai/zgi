import pytest
from datetime import datetime
from unittest.mock import AsyncMock, MagicMock, create_autospec
from sqlalchemy.orm import Session
from fastapi import HTTPException

from app.features.users.service import UserService
from app.features.users.schemas import UserProfileUpdate, UserPreferences
from app.core.cache import Cache
from app.core.email import EmailService

@pytest.fixture
def db_session():
    return MagicMock(spec=Session)

@pytest.fixture
def cache():
    return AsyncMock(spec=Cache)

@pytest.fixture
def email_service():
    return AsyncMock(spec=EmailService)

@pytest.fixture
def user_service(db_session, cache, email_service):
    return UserService(db_session, cache, email_service)

@pytest.fixture
def mock_user():
    # 创建一个模拟用户对象
    user = MagicMock()
    user.id = 1
    user.email = "test@example.com"
    user.username = "testuser"
    user.full_name = "Test User"
    user.hashed_password = "hashed_password"
    user.is_active = True
    user.is_verified = True
    user.is_admin = False
    user.avatar_url = None
    user.bio = None
    user.phone = None
    user.location = None
    user.company = None
    user.website = None
    user.preferences = {}
    user.last_login = None
    user.login_count = 0
    user.created_at = datetime.utcnow()
    user.updated_at = datetime.utcnow()
    return user

@pytest.mark.asyncio
async def test_get_user_profile(user_service, mock_user, db_session, cache):
    # 设置模拟数据
    db_session.query.return_value.filter.return_value.first.return_value = mock_user
    cache.get.return_value = None
    
    # 测试获取用户信息
    user = await user_service.get_user_profile(1)
    
    assert user.id == 1
    assert user.email == "test@example.com"
    assert user.username == "testuser"
    assert user.login_count >= 0
    assert hasattr(user, 'last_login')
    
    # 验证缓存调用
    cache.set.assert_called_once()

@pytest.mark.asyncio
async def test_update_profile(user_service, mock_user, db_session, cache):
    # 设置模拟数据
    db_session.query.return_value.filter.return_value.first.return_value = mock_user
    cache.get.return_value = None
    
    update_data = UserProfileUpdate(
        full_name="Updated Name",
        bio="Test bio",
        location="Test City",
        website="example.com"
    )
    
    # 测试更新用户资料
    updated_user = await user_service.update_profile(1, update_data)
    
    # 验证更新是否正确
    assert updated_user.full_name == "Updated Name"
    assert updated_user.bio == "Test bio"
    assert updated_user.location == "Test City"
    assert updated_user.website == "https://example.com"
    
    # 验证缓存被清除
    cache.delete.assert_called_once_with("user_profile:1")

@pytest.mark.asyncio
async def test_update_preferences(user_service, mock_user, db_session, cache):
    # 设置模拟数据
    db_session.query.return_value.filter.return_value.first.return_value = mock_user
    cache.get.return_value = None
    
    preferences = UserPreferences(
        theme="dark",
        language="zh",
        timezone="Asia/Shanghai",
        notifications_enabled=True,
        email_notifications=True
    )
    
    # 测试更新用户偏好设置
    updated_user = await user_service.update_preferences(1, preferences)
    
    # 验证更新是否正确
    assert updated_user.preferences.get("theme") == "dark"
    assert updated_user.preferences.get("language") == "zh"
    assert updated_user.preferences.get("timezone") == "Asia/Shanghai"
    
    # 验证缓存被清除
    cache.delete.assert_called_once_with("user_profile:1")

@pytest.mark.asyncio
async def test_get_user_profile_from_cache(user_service, mock_user, cache):
    # 设置缓存中有数据
    cache.get.return_value = mock_user
    
    # 测试从缓存获取用户信息
    user = await user_service.get_user_profile(1)
    
    assert user.id == 1
    assert user.email == "test@example.com"
    # 验证没有访问数据库
    user_service.db.query.assert_not_called()

@pytest.mark.asyncio
async def test_update_profile_invalid_user(user_service, db_session, cache):
    # 设置模拟数据 - 用户不存在
    db_session.query.return_value.filter.return_value.first.return_value = None
    cache.get.return_value = None
    
    update_data = UserProfileUpdate(full_name="Updated Name")
    
    # 测试更新不存在的用户
    with pytest.raises(HTTPException) as exc:
        await user_service.update_profile(1, update_data)
    
    assert exc.value.status_code == 404
    assert exc.value.detail == "用户不存在"

@pytest.mark.asyncio
async def test_update_preferences_validation(user_service, mock_user, db_session, cache):
    # 设置模拟数据
    db_session.query.return_value.filter.return_value.first.return_value = mock_user
    cache.get.return_value = None
    
    # 测试无效的时区
    preferences = UserPreferences(timezone="Invalid/Timezone")
    
    with pytest.raises(HTTPException) as exc:
        await user_service.update_preferences(1, preferences)
    
    assert exc.value.status_code == 400
    assert "无效的时区" in exc.value.detail

@pytest.mark.asyncio
async def test_request_password_reset(user_service, mock_user, db_session, cache, email_service):
    # 设置模拟数据
    db_session.query.return_value.filter.return_value.first.return_value = mock_user
    cache.set.return_value = None
    
    # 测试请求密码重置
    result = await user_service.request_password_reset("test@example.com")
    
    assert result is True
    # 验证缓存设置
    cache.set.assert_called_once()
    # 验证邮件发送
    email_service.send_email.assert_called_once()
    assert "密码重置验证码" in email_service.send_email.call_args[1]['subject']

@pytest.mark.asyncio
async def test_request_password_reset_nonexistent_user(user_service, db_session, cache, email_service):
    # 设置模拟数据 - 用户不存在
    db_session.query.return_value.filter.return_value.first.return_value = None
    
    # 测试请求密码重置
    result = await user_service.request_password_reset("nonexistent@example.com")
    
    assert result is True  # 应该返回True以避免泄露用户信息
    # 验证没有设置缓存
    cache.set.assert_not_called()
    # 验证没有发送邮件
    email_service.send_email.assert_not_called()

@pytest.mark.asyncio
async def test_verify_reset_code(user_service, cache):
    # 设置模拟数据
    test_email = "test@example.com"
    test_code = "123456"
    cache.get.return_value = test_code
    
    # 测试验证重置码
    result = await user_service.verify_reset_code(test_email, test_code)
    
    assert result is True
    cache.get.assert_called_once_with("password_reset:test@example.com")

@pytest.mark.asyncio
async def test_verify_reset_code_invalid(user_service, cache):
    # 设置模拟数据
    test_email = "test@example.com"
    cache.get.return_value = "123456"
    
    # 测试验证无效的重置码
    with pytest.raises(HTTPException) as exc:
        await user_service.verify_reset_code(test_email, "654321")
    
    assert exc.value.status_code == 400
    assert "验证码无效或已过期" in exc.value.detail

@pytest.mark.asyncio
async def test_verify_reset_code_expired(user_service, cache):
    # 设置模拟数据
    test_email = "test@example.com"
    cache.get.return_value = None  # 模拟过期的验证码
    
    # 测试验证过期的重置码
    with pytest.raises(HTTPException) as exc:
        await user_service.verify_reset_code(test_email, "123456")
    
    assert exc.value.status_code == 400
    assert "验证码无效或已过期" in exc.value.detail

@pytest.mark.asyncio
async def test_reset_password(user_service, mock_user, db_session, cache, email_service):
    # 设置模拟数据
    test_email = "test@example.com"
    test_code = "123456"
    cache.get.return_value = test_code
    db_session.query.return_value.filter.return_value.first.return_value = mock_user
    
    # 测试重置密码
    result = await user_service.reset_password(test_email, test_code, "NewPassword123!")
    
    assert result is True
    # 验证密码已更新
    assert mock_user.hashed_password != "hashed_password"
    # 验证缓存已清除
    cache.delete.assert_called_once_with(f"password_reset:{test_email}")
    # 验证发送了通知邮件
    email_service.send_email.assert_called_once()
    assert "密码已重置" in email_service.send_email.call_args[1]['subject']

@pytest.mark.asyncio
async def test_reset_password_invalid_code(user_service, mock_user, db_session, cache):
    # 设置模拟数据
    test_email = "test@example.com"
    cache.get.return_value = "123456"
    
    # 测试使用无效验证码重置密码
    with pytest.raises(HTTPException) as exc:
        await user_service.reset_password(test_email, "654321", "NewPassword123!")
    
    assert exc.value.status_code == 400
    assert "验证码无效或已过期" in exc.value.detail

@pytest.mark.asyncio
async def test_reset_password_nonexistent_user(user_service, db_session, cache):
    # 设置模拟数据
    test_email = "nonexistent@example.com"
    test_code = "123456"
    cache.get.return_value = test_code
    db_session.query.return_value.filter.return_value.first.return_value = None
    
    # 测试重置不存在用户的密码
    with pytest.raises(HTTPException) as exc:
        await user_service.reset_password(test_email, test_code, "NewPassword123!")
    
    assert exc.value.status_code == 404
    assert "用户不存在" in exc.value.detail
