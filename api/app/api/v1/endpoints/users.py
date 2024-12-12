from typing import Any
from fastapi import APIRouter, Depends, HTTPException, status
from sqlalchemy.orm import Session
from app.api import deps
from app.features.users.models import User
from app.schemas.user import (
    UserResponse,
    UserUpdate,
    UserPreferences,
    EmailVerificationRequest,
    PasswordResetRequest,
    PasswordResetConfirm,
    UserListResponse
)
from app.features.users.service import UserService
from app.core.email import EmailService
from app.core.cache import Cache

router = APIRouter()

@router.get("/me", response_model=UserResponse)
async def get_current_user_profile(
    current_user: User = Depends(deps.get_current_active_user)
) -> Any:
    """获取当前用户信息"""
    return current_user

@router.put("/me", response_model=UserResponse)
async def update_user_profile(
    *,
    db: Session = Depends(deps.get_db),
    current_user: User = Depends(deps.get_current_active_user),
    user_in: UserUpdate,
    cache: Cache = Depends(),
    email_service: EmailService = Depends()
) -> Any:
    """更新用户信息"""
    user_service = UserService(db, cache, email_service)
    return await user_service.update_profile(current_user.id, user_in)

@router.put("/me/preferences", response_model=UserResponse)
async def update_user_preferences(
    *,
    db: Session = Depends(deps.get_db),
    current_user: User = Depends(deps.get_current_active_user),
    preferences: UserPreferences,
    cache: Cache = Depends(),
    email_service: EmailService = Depends()
) -> Any:
    """更新用户偏好设置"""
    user_service = UserService(db, cache, email_service)
    return await user_service.update_preferences(current_user.id, preferences)

@router.post("/verify-email/request")
async def request_email_verification(
    current_user: User = Depends(deps.get_current_active_user),
    db: Session = Depends(deps.get_db),
    cache: Cache = Depends(),
    email_service: EmailService = Depends()
) -> Any:
    """请求邮箱验证"""
    user_service = UserService(db, cache, email_service)
    success = await user_service.send_verification_email(current_user.id)
    if not success:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Failed to send verification email"
        )
    return {"message": "Verification email sent"}

@router.post("/verify-email/confirm")
async def confirm_email_verification(
    *,
    db: Session = Depends(deps.get_db),
    verification: EmailVerificationRequest,
    current_user: User = Depends(deps.get_current_active_user),
    cache: Cache = Depends(),
    email_service: EmailService = Depends()
) -> Any:
    """确认邮箱验证"""
    user_service = UserService(db, cache, email_service)
    success = await user_service.verify_email(current_user.email, verification.token)
    if not success:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Invalid verification token"
        )
    return {"message": "Email verified successfully"}

@router.post("/reset-password/request")
async def request_password_reset(
    reset_request: PasswordResetRequest,
    db: Session = Depends(deps.get_db),
    cache: Cache = Depends(),
    email_service: EmailService = Depends()
) -> Any:
    """请求密码重置"""
    user_service = UserService(db, cache, email_service)
    success = await user_service.request_password_reset(reset_request.email)
    # 为了安全，无论是否成功都返回相同的消息
    return {"message": "Password reset email sent"}

@router.post("/reset-password/confirm")
async def confirm_password_reset(
    *,
    db: Session = Depends(deps.get_db),
    reset_confirm: PasswordResetConfirm,
    cache: Cache = Depends(),
    email_service: EmailService = Depends()
) -> Any:
    """确认密码重置"""
    user_service = UserService(db, cache, email_service)
    success = await user_service.reset_password(
        email=reset_confirm.email,
        token=reset_confirm.token,
        new_password=reset_confirm.new_password
    )
    if not success:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Invalid reset token"
        )
    return {"message": "Password reset successfully"}

# Admin endpoints
@router.get("", response_model=UserListResponse)
async def list_users(
    db: Session = Depends(deps.get_db),
    skip: int = 0,
    limit: int = 100,
    current_user: User = Depends(deps.get_current_admin_user),
    cache: Cache = Depends(),
    email_service: EmailService = Depends()
) -> Any:
    """获取用户列表（管理员功能）"""
    user_service = UserService(db, cache, email_service)
    users, total = await user_service.list_users(skip=skip, limit=limit)
    return {
        "items": users,
        "total": total,
        "page": skip // limit + 1,
        "size": limit
    }

@router.post("/{user_id}/deactivate", response_model=UserResponse)
async def deactivate_user(
    user_id: int,
    db: Session = Depends(deps.get_db),
    current_user: User = Depends(deps.get_current_admin_user),
    cache: Cache = Depends(),
    email_service: EmailService = Depends()
) -> Any:
    """停用用户（管理员功能）"""
    user_service = UserService(db, cache, email_service)
    success = await user_service.deactivate_user(user_id)
    if not success:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="User not found"
        )
    user = await user_service.get_user_profile(user_id)
    return user
