import pytest
from datetime import datetime
from fastapi.testclient import TestClient
from sqlalchemy.orm import Session
from app.features.users.models import User
from app.core.security import get_password_hash

# Fixtures
@pytest.fixture
def test_user(db: Session):
    """创建测试用户"""
    user = User(
        email="test@example.com",
        username="testuser",
        full_name="Test User",
        hashed_password=get_password_hash("testpassword123"),
        is_active=True,
        is_verified=False,
        is_admin=False,
        is_superuser=False,
        created_at=datetime.utcnow(),
        updated_at=datetime.utcnow()
    )
    db.add(user)
    db.commit()
    db.refresh(user)
    yield user
    # 清理
    db.query(User).filter(User.email == "test@example.com").delete()
    db.commit()

@pytest.fixture
def admin_user(db: Session):
    """创建管理员用户"""
    admin = User(
        email="admin@example.com",
        username="adminuser",
        full_name="Admin User",
        hashed_password=get_password_hash("adminpassword123"),
        is_active=True,
        is_verified=True,
        is_admin=True,
        is_superuser=True,
        created_at=datetime.utcnow(),
        updated_at=datetime.utcnow()
    )
    db.add(admin)
    db.commit()
    db.refresh(admin)
    yield admin
    # 清理
    db.query(User).filter(User.email == "admin@example.com").delete()
    db.commit()

@pytest.fixture
def mock_cache():
    """模拟缓存服务"""
    cache_data = {}
    
    async def mock_get(key):
        return cache_data.get(key)
    
    async def mock_set(key, value, expire=None):
        cache_data[key] = value
    
    async def mock_delete(key):
        if key in cache_data:
            del cache_data[key]
    
    return type("MockCache", (), {
        "get": mock_get,
        "set": mock_set,
        "delete": mock_delete
    })()

@pytest.fixture
def mock_email_service():
    """模拟邮件服务"""
    async def mock_send_verification_email(email: str, token: str):
        return True
        
    async def mock_send_password_reset_email(email: str, token: str):
        return True
        
    return type("MockEmailService", (), {
        "send_verification_email": mock_send_verification_email,
        "send_password_reset_email": mock_send_password_reset_email
    })()

@pytest.fixture
def user_token_header(client: TestClient, test_user: User):
    """获取用户认证头"""
    response = client.post(
        "/v1/auth/login",
        json={
            "email": test_user.email,
            "password": "testpassword123"
        }
    )
    token = response.json()["access_token"]
    return {"Authorization": f"Bearer {token}"}

@pytest.fixture
def admin_token_header(client: TestClient, admin_user: User):
    """获取管理员认证头"""
    response = client.post(
        "/v1/auth/login",
        json={
            "email": admin_user.email,
            "password": "adminpassword123"
        }
    )
    token = response.json()["access_token"]
    return {"Authorization": f"Bearer {token}"}

# User Profile Tests
class TestUserProfile:
    def test_get_current_user_profile(self, client: TestClient, test_user: User, user_token_header):
        """测试获取当前用户配置文件"""
        response = client.get("/v1/users/me", headers=user_token_header)
        assert response.status_code == 200
        data = response.json()
        assert data["email"] == test_user.email
        assert data["is_active"] is True

    def test_update_user_profile(self, client: TestClient, test_user: User, user_token_header):
        """测试更新用户配置文件"""
        update_data = {
            "full_name": "New Name",
            "password": "newpassword123"
        }
        response = client.put("/v1/users/me", json=update_data, headers=user_token_header)
        assert response.status_code == 200
        data = response.json()
        assert data["full_name"] == "New Name"

    def test_update_invalid_profile(self, client: TestClient, user_token_header):
        """测试无效的配置文件更新"""
        response = client.put("/v1/users/me", json={}, headers=user_token_header)
        assert response.status_code == 422

# User Preferences Tests
class TestUserPreferences:
    def test_update_preferences(self, client: TestClient, test_user: User, user_token_header):
        """测试更新用户偏好设置"""
        preferences = {
            "theme": "dark",
            "language": "zh-CN",
            "notifications_enabled": True
        }
        response = client.put("/v1/users/me/preferences", json=preferences, headers=user_token_header)
        assert response.status_code == 200
        data = response.json()
        assert data["preferences"]["theme"] == "dark"

    def test_invalid_preferences(self, client: TestClient, user_token_header):
        """测试无效的偏好设置"""
        response = client.put("/v1/users/me/preferences", json={}, headers=user_token_header)
        assert response.status_code == 422

# Email Verification Tests
class TestEmailVerification:
    def test_request_verification(self, client: TestClient, test_user: User, user_token_header, mock_email_service):
        """测试请求邮箱验证"""
        response = client.post("/v1/users/verify-email/request", headers=user_token_header)
        assert response.status_code == 200

    def test_confirm_verification(self, client: TestClient, test_user: User, user_token_header, mock_cache, mock_email_service):
        """测试确认邮箱验证"""
        token = "test-verification-token"
        mock_cache.set(f"email_verification:{test_user.email}", token)
        response = client.post("/v1/users/verify-email/confirm", json={"token": token}, headers=user_token_header)
        assert response.status_code == 200

    def test_invalid_verification_token(self, client: TestClient, test_user: User, user_token_header):
        """测试无效的验证token"""
        response = client.post("/v1/users/verify-email/confirm", json={"token": "invalid-token"}, headers=user_token_header)
        assert response.status_code == 400

# Password Reset Tests
class TestPasswordReset:
    def test_request_password_reset(self, client: TestClient, test_user: User, mock_email_service):
        """测试请求密码重置"""
        response = client.post("/v1/users/reset-password/request", json={"email": test_user.email})
        assert response.status_code == 200

    def test_confirm_password_reset(self, client: TestClient, test_user: User, mock_cache, mock_email_service):
        """测试确认密码重置"""
        token = "test-reset-token"
        mock_cache.set(f"password_reset:{test_user.email}", token)
        response = client.post("/v1/users/reset-password/confirm", 
            json={
                "email": test_user.email,
                "token": token,
                "new_password": "newpassword123"
            })
        assert response.status_code == 200

    def test_invalid_reset_token(self, client: TestClient, test_user: User):
        """测试无效的重置token"""
        response = client.post("/v1/users/reset-password/confirm",
            json={
                "email": test_user.email,
                "token": "invalid-token",
                "new_password": "newpassword123"
            })
        assert response.status_code == 400

# User Management Tests (Admin Functions)
class TestUserManagement:
    def test_list_users(self, client: TestClient, admin_token_header):
        """测试获取用户列表（管理员）"""
        response = client.get("/v1/console/users", headers=admin_token_header)
        assert response.status_code == 200
        data = response.json()
        assert "total" in data
        assert "items" in data
        assert "page" in data
        assert "size" in data

    def test_list_users_unauthorized(self, client: TestClient, user_token_header):
        """测试未授权获取用户列表"""
        response = client.get("/v1/console/users", headers=user_token_header)
        assert response.status_code == 403

    def test_deactivate_user(self, client: TestClient, test_user: User, admin_token_header):
        """测试停用用户（管理员）"""
        response = client.post(f"/v1/console/users/{test_user.id}/deactivate", headers=admin_token_header)
        assert response.status_code == 200

    def test_deactivate_user_unauthorized(self, client: TestClient, test_user: User, user_token_header):
        """测试未授权停用用户"""
        response = client.post(f"/v1/console/users/{test_user.id}/deactivate", headers=user_token_header)
        assert response.status_code == 403

# Error Handling Tests
class TestErrorHandling:
    def test_user_not_found(self, client: TestClient, admin_token_header):
        """测试访问不存在的用户"""
        response = client.post("/v1/console/users/999/deactivate", headers=admin_token_header)
        assert response.status_code == 404

    def test_invalid_email_format(self, client: TestClient):
        """测试无效的邮箱格式"""
        response = client.post("/v1/users/reset-password/request", json={"email": "invalid-email"})
        assert response.status_code == 422

    def test_weak_password(self, client: TestClient, test_user: User, user_token_header):
        """测试弱密码"""
        response = client.put("/v1/users/me", 
            json={"password": "weak"}, 
            headers=user_token_header)
        assert response.status_code == 422
