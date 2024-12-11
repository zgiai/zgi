from fastapi import APIRouter, Depends, HTTPException, Query
from sqlalchemy.orm import Session

from app.core.deps import get_db, get_current_user
from app.core.cache import get_cache
from app.core.email import get_email_service
from app.features.users.schemas import (
    UserProfile,
    UserProfileUpdate,
    UserPreferences,
    PasswordReset,
    EmailVerification,
)
from app.features.users.service import UserService
from app.core.security import verify_token

router = APIRouter()


@router.get("/me", response_model=UserProfile)
async def get_current_user_profile(
    current_user=Depends(get_current_user),
    db: Session = Depends(get_db),
    cache=Depends(get_cache),
    email_service=Depends(get_email_service),
):
    """Get current user's profile."""
    service = UserService(db, cache, email_service)
    user = await service.get_user_profile(current_user.id)
    if not user:
        raise HTTPException(status_code=404, detail="User not found")
    return user


@router.put("/me", response_model=UserProfile)
async def update_user_profile(
    update_data: UserProfileUpdate,
    current_user=Depends(get_current_user),
    db: Session = Depends(get_db),
    cache=Depends(get_cache),
    email_service=Depends(get_email_service),
):
    """Update current user's profile."""
    service = UserService(db, cache, email_service)
    return await service.update_profile(current_user.id, update_data)


@router.put("/me/preferences", response_model=UserProfile)
async def update_user_preferences(
    preferences: UserPreferences,
    current_user=Depends(get_current_user),
    db: Session = Depends(get_db),
    cache=Depends(get_cache),
    email_service=Depends(get_email_service),
):
    """Update user preferences."""
    service = UserService(db, cache, email_service)
    return await service.update_preferences(current_user.id, preferences)


@router.post("/password-reset/request")
async def request_password_reset(
    email: str,
    db: Session = Depends(get_db),
    cache=Depends(get_cache),
    email_service=Depends(get_email_service),
):
    """Request a password reset."""
    service = UserService(db, cache, email_service)
    success = await service.request_password_reset(email)
    if not success:
        raise HTTPException(status_code=404, detail="User not found")
    return {"message": "Password reset email sent"}


@router.post("/password-reset/confirm")
async def confirm_password_reset(
    reset_data: PasswordReset,
    db: Session = Depends(get_db),
    cache=Depends(get_cache),
    email_service=Depends(get_email_service),
):
    """Confirm password reset."""
    service = UserService(db, cache, email_service)
    success = await service.reset_password(
        reset_data.email, reset_data.token, reset_data.new_password
    )
    if not success:
        raise HTTPException(status_code=400, detail="Invalid reset token")
    return {"message": "Password reset successful"}


@router.post("/verify-email/request")
async def request_email_verification(
    current_user=Depends(get_current_user),
    db: Session = Depends(get_db),
    cache=Depends(get_cache),
    email_service=Depends(get_email_service),
):
    """Request email verification."""
    service = UserService(db, cache, email_service)
    success = await service.send_verification_email(current_user.id)
    if not success:
        raise HTTPException(
            status_code=400, detail="User already verified or not found"
        )
    return {"message": "Verification email sent"}


@router.post("/verify-email/confirm")
async def confirm_email_verification(
    verification_data: EmailVerification,
    db: Session = Depends(get_db),
    cache=Depends(get_cache),
    email_service=Depends(get_email_service),
):
    """Confirm email verification."""
    service = UserService(db, cache, email_service)
    success = await service.verify_email(
        verification_data.email, verification_data.token
    )
    if not success:
        raise HTTPException(status_code=400, detail="Invalid verification token")
    return {"message": "Email verification successful"}


@router.get("/list", response_model=dict)
async def list_users(
    skip: int = Query(0, ge=0),
    limit: int = Query(10, ge=1, le=100),
    current_user=Depends(get_current_user),
    db: Session = Depends(get_db),
    cache=Depends(get_cache),
    email_service=Depends(get_email_service),
):
    """List users (admin only)."""
    if not current_user.is_admin:
        raise HTTPException(status_code=403, detail="Not authorized")
    
    service = UserService(db, cache, email_service)
    users, total = await service.list_users(skip, limit)
    return {
        "total": total,
        "items": users,
        "skip": skip,
        "limit": limit,
    }


@router.post("/{user_id}/deactivate")
async def deactivate_user(
    user_id: int,
    current_user=Depends(get_current_user),
    db: Session = Depends(get_db),
    cache=Depends(get_cache),
    email_service=Depends(get_email_service),
):
    """Deactivate a user (admin only)."""
    if not current_user.is_admin:
        raise HTTPException(status_code=403, detail="Not authorized")
    
    service = UserService(db, cache, email_service)
    success = await service.deactivate_user(user_id)
    if not success:
        raise HTTPException(status_code=404, detail="User not found")
    return {"message": "User deactivated successfully"}
