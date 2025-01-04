from typing import Optional, Dict, Any
from pydantic import BaseModel, EmailStr, constr

class UserBase(BaseModel):
    """用户基础信息"""
    email: EmailStr
    username: str
    full_name: Optional[str] = None
    is_active: bool = True
    is_verified: bool = False
    is_admin: bool = False
    is_superuser: bool = False

class UserCreate(UserBase):
    """创建用户"""
    password: constr(min_length=8)

class UserUpdate(BaseModel):
    """更新用户信息"""
    full_name: Optional[str] = None
    password: Optional[constr(min_length=8)] = None

class UserPreferences(BaseModel):
    """用户偏好设置"""
    theme: Optional[str] = None
    language: Optional[str] = None
    notifications_enabled: Optional[bool] = None

class UserInDB(UserBase):
    """数据库中的用户"""
    id: int
    preferences: Optional[Dict[str, Any]] = None

    class Config:
        from_attributes = True

class UserResponse(UserBase):
    """用户响应"""
    id: int
    preferences: Optional[Dict[str, Any]] = None

    class Config:
        from_attributes = True

class Token(BaseModel):
    """认证令牌"""
    access_token: str
    token_type: str = "bearer"

class TokenData(BaseModel):
    """令牌数据"""
    email: Optional[str] = None

class EmailVerificationRequest(BaseModel):
    """邮箱验证请求"""
    token: str

class PasswordResetRequest(BaseModel):
    """密码重置请求"""
    email: EmailStr

class PasswordResetConfirm(BaseModel):
    """密码重置确认"""
    token: str
    new_password: constr(min_length=8)

class UserListResponse(BaseModel):
    """用户列表响应"""
    items: list[UserResponse]
    total: int
    page: int
    size: int
