from datetime import datetime
from typing import Optional, List
from pydantic import BaseModel, EmailStr, Field, constr, validator

class Token(BaseModel):
    """认证令牌"""
    access_token: str
    token_type: str = "bearer"

class UserLogin(BaseModel):
    """用户登录"""
    email: EmailStr
    password: constr(min_length=8)

class UserBase(BaseModel):
    """用户基础信息"""
    email: EmailStr
    username: str
    full_name: Optional[str] = None

class UserCreate(UserBase):
    """创建用户"""
    password: constr(min_length=8)

class UserProfile(UserBase):
    """用户信息"""
    id: int
    is_active: bool = True
    is_verified: bool = False
    is_admin: bool = False
    avatar_url: Optional[str] = None
    bio: Optional[str] = None
    phone: Optional[str] = None
    location: Optional[str] = None
    company: Optional[str] = None
    website: Optional[str] = None
    preferences: Optional[dict] = None
    last_login: Optional[datetime] = None
    login_count: Optional[int] = 0
    created_at: datetime
    updated_at: datetime

    class Config:
        from_attributes = True

class UserProfileUpdate(BaseModel):
    """更新用户信息"""
    full_name: Optional[str] = None
    avatar_url: Optional[str] = None
    bio: Optional[str] = Field(None, max_length=500)
    phone: Optional[str] = Field(None, pattern=r'^\+?1?\d{9,15}$')
    location: Optional[str] = Field(None, max_length=100)
    company: Optional[str] = Field(None, max_length=100)
    website: Optional[str] = Field(None, max_length=200)
    password: Optional[constr(min_length=8, max_length=64)] = None

class UserPreferences(BaseModel):
    """用户偏好设置"""
    theme: Optional[str] = Field("light", pattern="^(light|dark|system)$")
    language: Optional[str] = Field("en", pattern="^(en|zh|es|fr|de|ja|ko)$")
    timezone: Optional[str] = Field("UTC", max_length=50)
    date_format: Optional[str] = Field("YYYY-MM-DD", max_length=20)
    time_format: Optional[str] = Field("HH:mm:ss", max_length=20)
    notifications_enabled: Optional[bool] = True
    email_notifications: Optional[bool] = True
    desktop_notifications: Optional[bool] = True
    weekly_digest: Optional[bool] = True
    two_factor_auth: Optional[bool] = False

class EmailVerificationRequest(BaseModel):
    """邮箱验证请求"""
    token: str

class PasswordResetRequest(BaseModel):
    """密码重置请求"""
    email: EmailStr

class PasswordResetVerify(BaseModel):
    """密码重置验证"""
    email: EmailStr
    reset_code: str = Field(..., min_length=6, max_length=6)

class PasswordResetConfirm(BaseModel):
    """密码重置确认"""
    email: EmailStr
    reset_code: str = Field(..., min_length=6, max_length=6)
    new_password: constr(min_length=8, max_length=64) = Field(
        ...,
        description="密码至少8个字符，必须包含字母和数字"
    )
    confirm_password: str = Field(..., description="确认密码")

    @validator('new_password')
    def validate_password(cls, v):
        if not any(c.isalpha() for c in v):
            raise ValueError('密码必须包含至少一个字母')
        if not any(c.isdigit() for c in v):
            raise ValueError('密码必须包含至少一个数字')
        if not any(c in '@$!%*#?&' for c in v):
            raise ValueError('密码必须包含至少一个特殊字符 (@$!%*#?&)')
        return v

    @validator('confirm_password')
    def passwords_match(cls, v, values, **kwargs):
        if 'new_password' in values and v != values['new_password']:
            raise ValueError('两次输入的密码不匹配')
        return v

class UserListResponse(BaseModel):
    """用户列表响应"""
    items: List[UserProfile]
    total: int
    page: int
    size: int
