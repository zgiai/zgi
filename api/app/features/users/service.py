from datetime import datetime, timedelta
from typing import Optional, Tuple, List
from sqlalchemy.orm import Session
from fastapi import HTTPException

from app.core.security import get_password_hash, verify_password
from app.core.security.auth import create_access_token
from app.models.user import User
from app.features.users.schemas import UserProfileUpdate, UserPreferences
from app.core.email import EmailService
from app.core.cache import Cache


class UserService:
    def __init__(self, db: Session, cache: Cache, email_service: EmailService):
        self.db = db
        self.cache = cache
        self.email_service = email_service

    async def get_user_profile(self, user_id: int) -> Optional[User]:
        # Try to get from cache first
        cache_key = f"user_profile:{user_id}"
        if user := await self.cache.get(cache_key):
            return user

        user = self.db.query(User).filter(User.id == user_id).first()
        if user:
            await self.cache.set(cache_key, user, expire=3600)  # Cache for 1 hour
        return user

    async def update_profile(self, user_id: int, update_data: UserProfileUpdate) -> User:
        user = await self.get_user_profile(user_id)
        if not user:
            raise HTTPException(status_code=404, detail="User not found")

        update_dict = update_data.dict(exclude_unset=True)
        if "password" in update_dict:
            update_dict["hashed_password"] = get_password_hash(update_dict.pop("password"))

        for field, value in update_dict.items():
            setattr(user, field, value)

        self.db.commit()
        self.db.refresh(user)
        
        # Invalidate cache
        await self.cache.delete(f"user_profile:{user_id}")
        return user

    async def update_preferences(self, user_id: int, preferences: UserPreferences) -> User:
        user = await self.get_user_profile(user_id)
        if not user:
            raise HTTPException(status_code=404, detail="User not found")

        user.preferences = preferences.dict()
        self.db.commit()
        self.db.refresh(user)
        
        # Invalidate cache
        await self.cache.delete(f"user_profile:{user_id}")
        return user

    async def request_password_reset(self, email: str) -> bool:
        user = self.db.query(User).filter(User.email == email).first()
        if not user:
            return False

        # Generate reset token
        reset_token = create_access_token(
            data={"sub": user.email, "type": "password_reset"},
            expires_delta=timedelta(hours=1)
        )

        # Store token in cache
        await self.cache.set(
            f"password_reset:{user.email}",
            reset_token,
            expire=3600  # 1 hour
        )

        # Send reset email
        await self.email_service.send_password_reset_email(
            email=user.email,
            token=reset_token
        )
        return True

    async def reset_password(self, email: str, token: str, new_password: str) -> bool:
        # Verify token from cache
        cached_token = await self.cache.get(f"password_reset:{email}")
        if not cached_token or cached_token != token:
            return False

        user = self.db.query(User).filter(User.email == email).first()
        if not user:
            return False

        user.hashed_password = get_password_hash(new_password)
        self.db.commit()

        # Clear reset token
        await self.cache.delete(f"password_reset:{email}")
        return True

    async def verify_email(self, email: str, token: str) -> bool:
        # Verify token from cache
        cached_token = await self.cache.get(f"email_verification:{email}")
        if not cached_token or cached_token != token:
            return False

        user = self.db.query(User).filter(User.email == email).first()
        if not user:
            return False

        user.is_verified = True
        self.db.commit()

        # Clear verification token
        await self.cache.delete(f"email_verification:{email}")
        return True

    async def send_verification_email(self, user_id: int) -> bool:
        user = await self.get_user_profile(user_id)
        if not user or user.is_verified:
            return False

        # Generate verification token
        verification_token = create_access_token(
            data={"sub": user.email, "type": "email_verification"},
            expires_delta=timedelta(hours=24)
        )

        # Store token in cache
        await self.cache.set(
            f"email_verification:{user.email}",
            verification_token,
            expire=86400  # 24 hours
        )

        # Send verification email
        await self.email_service.send_verification_email(
            email=user.email,
            token=verification_token
        )
        return True

    async def list_users(
        self, skip: int = 0, limit: int = 10
    ) -> Tuple[List[User], int]:
        total = self.db.query(User).count()
        users = self.db.query(User).offset(skip).limit(limit).all()
        return users, total

    async def deactivate_user(self, user_id: int) -> bool:
        user = await self.get_user_profile(user_id)
        if not user:
            return False

        user.is_active = False
        self.db.commit()
        
        # Invalidate cache
        await self.cache.delete(f"user_profile:{user_id}")
        return True
