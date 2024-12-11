from datetime import datetime
from typing import Optional
from pydantic import BaseModel, EmailStr, Field

class UserProfile(BaseModel):
    email: EmailStr
    full_name: str
    is_active: bool
    is_verified: bool
    created_at: datetime
    updated_at: datetime

    class Config:
        orm_mode = True

class UserProfileUpdate(BaseModel):
    full_name: Optional[str] = None
    password: Optional[str] = None
    preferences: Optional[dict] = None

class PasswordReset(BaseModel):
    email: EmailStr
    token: str
    new_password: str = Field(..., min_length=8)

class EmailVerification(BaseModel):
    email: EmailStr
    token: str

class UserPreferences(BaseModel):
    theme: Optional[str] = "light"
    language: Optional[str] = "en"
    notifications_enabled: Optional[bool] = True
    email_notifications: Optional[bool] = True
