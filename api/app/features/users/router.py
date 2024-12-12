from fastapi import APIRouter, Depends, HTTPException, Query
from sqlalchemy.orm import Session

from app.core.deps import get_db, get_current_user
from app.core.cache import get_cache
from app.core.email import get_email_service
from app.core.security import verify_token
from app.features.users.schemas import (
    UserProfile,
    UserProfileUpdate,
    UserPreferences,
    UserListResponse,
    PasswordResetRequest,
    PasswordResetConfirm,
    EmailVerificationRequest,
    PasswordResetVerify
)
from app.features.users.service import UserService

def get_user_service(
    db: Session = Depends(get_db),
    cache=Depends(get_cache),
    email_service=Depends(get_email_service),
) -> UserService:
    return UserService(db, cache, email_service)

router = APIRouter(prefix="/v1", tags=["users"])


@router.get("/users/me", response_model=UserProfile)
async def get_current_user_profile(
    current_user=Depends(get_current_user),
    db: Session = Depends(get_db),
    cache=Depends(get_cache),
    email_service=Depends(get_email_service),
):
    """获取当前用户信息"""
    service = UserService(db, cache, email_service)
    user = await service.get_user_profile(current_user.id)
    if not user:
        raise HTTPException(status_code=404, detail="用户不存在")
    return user


@router.put("/users/me", response_model=UserProfile)
async def update_user_profile(
    update_data: UserProfileUpdate,
    current_user=Depends(get_current_user),
    db: Session = Depends(get_db),
    cache=Depends(get_cache),
    email_service=Depends(get_email_service),
):
    """更新当前用户信息"""
    service = UserService(db, cache, email_service)
    return await service.update_profile(current_user.id, update_data)


@router.put("/users/me/preferences", response_model=UserProfile)
async def update_user_preferences(
    preferences: UserPreferences,
    current_user=Depends(get_current_user),
    db: Session = Depends(get_db),
    cache=Depends(get_cache),
    email_service=Depends(get_email_service),
):
    """更新用户偏好设置"""
    service = UserService(db, cache, email_service)
    return await service.update_preferences(current_user.id, preferences)


@router.post("/users/reset-password/request")
async def request_password_reset(
    data: PasswordResetRequest,
    db: Session = Depends(get_db),
    cache=Depends(get_cache),
    email_service=Depends(get_email_service),
):
    """请求密码重置"""
    service = UserService(db, cache, email_service)
    success = await service.request_password_reset(data.email)
    if not success:
        raise HTTPException(status_code=404, detail="用户不存在")
    return {"message": "密码重置邮件已发送"}


@router.post("/users/reset-password/confirm")
async def confirm_password_reset(
    reset_data: PasswordResetConfirm,
    db: Session = Depends(get_db),
    cache=Depends(get_cache),
    email_service=Depends(get_email_service),
):
    """确认密码重置"""
    service = UserService(db, cache, email_service)
    success = await service.reset_password(
        reset_data.email, reset_data.token, reset_data.new_password
    )
    if not success:
        raise HTTPException(status_code=400, detail="无效的重置令牌")
    return {"message": "密码重置成功"}


@router.post("/password-reset/request", response_model=dict)
async def request_password_reset(
    request: PasswordResetRequest,
    user_service: UserService = Depends(get_user_service)
):
    """
    请求密码重置
    
    - **email**: 用户邮箱
    """
    result = await user_service.request_password_reset(request.email)
    return {
        "message": "如果该邮箱存在，我们已经发送了重置验证码",
        "success": result
    }


@router.post("/password-reset/verify", response_model=dict)
async def verify_reset_code(
    verify: PasswordResetVerify,
    user_service: UserService = Depends(get_user_service)
):
    """
    验证重置码
    
    - **email**: 用户邮箱
    - **reset_code**: 重置验证码
    """
    result = await user_service.verify_reset_code(verify.email, verify.reset_code)
    return {
        "message": "验证码正确",
        "success": result
    }


@router.post("/password-reset/confirm", response_model=dict)
async def confirm_password_reset(
    confirm: PasswordResetConfirm,
    user_service: UserService = Depends(get_user_service)
):
    """
    确认密码重置
    
    - **email**: 用户邮箱
    - **reset_code**: 重置验证码
    - **new_password**: 新密码
    - **confirm_password**: 确认新密码
    """
    result = await user_service.reset_password(
        confirm.email,
        confirm.reset_code,
        confirm.new_password
    )
    return {
        "message": "密码已成功重置",
        "success": result
    }


@router.post("/users/verify-email/request")
async def request_email_verification(
    current_user=Depends(get_current_user),
    db: Session = Depends(get_db),
    cache=Depends(get_cache),
    email_service=Depends(get_email_service),
):
    """请求邮箱验证"""
    service = UserService(db, cache, email_service)
    success = await service.send_verification_email(current_user.id)
    if not success:
        raise HTTPException(
            status_code=400, detail="用户已验证或不存在"
        )
    return {"message": "验证邮件已发送"}


@router.post("/users/verify-email/confirm")
async def confirm_email_verification(
    verification_data: EmailVerificationRequest,
    current_user=Depends(get_current_user),
    db: Session = Depends(get_db),
    cache=Depends(get_cache),
    email_service=Depends(get_email_service),
):
    """确认邮箱验证"""
    service = UserService(db, cache, email_service)
    success = await service.verify_email(
        current_user.email, verification_data.token
    )
    if not success:
        raise HTTPException(status_code=400, detail="无效的验证令牌")
    return {"message": "邮箱验证成功"}


@router.get("/console/users", response_model=UserListResponse)
async def list_users(
    skip: int = Query(0, ge=0),
    limit: int = Query(10, ge=1, le=100),
    current_user=Depends(get_current_user),
    db: Session = Depends(get_db),
    cache=Depends(get_cache),
    email_service=Depends(get_email_service),
):
    """获取用户列表（仅管理员）"""
    if not current_user.is_admin:
        raise HTTPException(status_code=403, detail="没有权限")
    
    service = UserService(db, cache, email_service)
    users, total = await service.list_users(skip, limit)
    return UserListResponse(
        total=total,
        items=users,
        page=skip // limit + 1,
        size=limit,
    )


@router.post("/console/users/{user_id}/deactivate")
async def deactivate_user(
    user_id: int,
    current_user=Depends(get_current_user),
    db: Session = Depends(get_db),
    cache=Depends(get_cache),
    email_service=Depends(get_email_service),
):
    """停用用户（仅管理员）"""
    if not current_user.is_admin:
        raise HTTPException(status_code=403, detail="没有权限")
    
    service = UserService(db, cache, email_service)
    success = await service.deactivate_user(user_id)
    if not success:
        raise HTTPException(status_code=404, detail="用户不存在")
    return {"message": "用户已停用"}
